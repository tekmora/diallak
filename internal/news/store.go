package news

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	// KeyRetention is how long the key of a posted or dropped item is kept (ADR-0019, "What is
	// kept").
	KeyRetention = 30 * 24 * time.Hour
	// MaxItemAge is the age past which an item is never posted, so an expired key cannot bring it
	// back (ADR-0019, "What is kept").
	MaxItemAge = 7 * 24 * time.Hour
	// MaxKeys bounds the keys in the store; past it the oldest go first. 20 feeds of 200 items each
	// fill it after 25 reads that are all new. Source: chosen, not measured.
	MaxKeys = 100_000
	// MaxStateBytes bounds the state file a store reads. Source: chosen, not measured.
	MaxStateBytes = 16 << 20
	stateVersion  = 1
)

// FeedState is what the store keeps per listed feed.
type FeedState struct {
	// Validators are the ones the feed's server sent with the last read.
	Validators Validators `json:"validators"`
	// NextRead is the earliest time of the next read; zero means now.
	NextRead time.Time `json:"next_read"`
	// Primed is set by the first read, which marks every item as seen and posts nothing.
	Primed bool `json:"primed"`
}

// Store keeps what the news function remembers between reads (ADR-0019, points 7 and 8).
type Store interface {
	// Feed returns the state of feed id, the zero FeedState for a feed not read yet.
	Feed(id string) FeedState
	// Record keeps the result of a read of feed id and returns the items of res that may be posted.
	Record(id string, res Result, now time.Time) []Item
	// Forget drops the key of item itemID of feed id, so the next read returns the item again.
	Forget(id, itemID string)
	// Hold keeps items for the next digest of channel, in the order given, each with the time now.
	Hold(channel string, items []Item, now time.Time)
	// Held returns the items channel holds for its digest, in the order they were held.
	Held(channel string) []HeldItem
	// HeldChannels returns the channels that hold items, in order.
	HeldChannels() []string
	// ClearHeld forgets the items channel holds.
	ClearHeld(channel string)
	// SetNextRead sets the earliest time of the next read of feed id.
	SetNextRead(id string, at time.Time)
	// KeepFeeds forgets the state of every feed not in ids; item keys stay until they expire.
	KeepFeeds(ids []string)
	// Expire drops the item keys older than KeyRetention.
	Expire(now time.Time)
	// Save writes the state so that a crash leaves either the old or the new state.
	Save(ctx context.Context) error
}

// HeldItem is an item waiting for its channel's digest: its title and address only (ADR-0019,
// "What is kept").
type HeldItem struct {
	Title string `json:"title"`
	Link  string `json:"link"`
	// Held is when the item was held; it waits at most MaxHeldAge.
	Held time.Time `json:"held"`
}

type stateFile struct {
	Version int                   `json:"version"`
	Feeds   map[string]FeedState  `json:"feeds"`
	Seen    map[string]time.Time  `json:"seen"`
	Held    map[string][]HeldItem `json:"held,omitempty"`
}

// FileStore is a Store kept in one JSON file.
type FileStore struct {
	path  string
	mu    sync.Mutex
	state stateFile
}

var _ Store = (*FileStore)(nil)

// OpenFileStore reads the state at path. A missing file is an empty state; a file that cannot be
// read whole is an error, never an empty state, so a damaged file cannot make the bot post again.
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path, state: stateFile{
		Version: stateVersion,
		Feeds:   map[string]FeedState{},
		Seen:    map[string]time.Time{},
		Held:    map[string][]HeldItem{},
	}}
	data, err := readStateFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var read stateFile
	if err := dec.Decode(&read); err != nil {
		return nil, fmt.Errorf("decode news state: %w", err)
	}
	if read.Version != stateVersion {
		return nil, fmt.Errorf("news state version %d, want %d", read.Version, stateVersion)
	}
	maps.Copy(s.state.Feeds, read.Feeds)
	maps.Copy(s.state.Seen, read.Seen)
	maps.Copy(s.state.Held, read.Held)
	return s, nil
}

// readStateFile returns the content of path, at most MaxStateBytes.
func readStateFile(path string) (_ []byte, err error) {
	f, err := os.Open(path) // #nosec G304 -- the operator names the state file.
	if err != nil {
		return nil, fmt.Errorf("open news state: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close news state: %w", closeErr))
		}
	}()
	data, err := io.ReadAll(io.LimitReader(f, MaxStateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read news state: %w", err)
	}
	if len(data) > MaxStateBytes {
		return nil, fmt.Errorf("news state exceeds max %d bytes", MaxStateBytes)
	}
	return data, nil
}

// ItemKey is the stable key of an item: the SHA-256 of the feed id and the item's own id. The
// item's text is not part of it, so an item whose title changed keeps its key.
func ItemKey(feedID, itemID string) string {
	sum := sha256.Sum256([]byte(feedID + "\x00" + itemID))
	return hex.EncodeToString(sum[:])
}

// Feed implements Store.
func (s *FileStore) Feed(id string) FeedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Feeds[id]
}

// Record implements Store. Every item of res is marked as seen. The first read of a feed returns
// nothing; later reads return the items not seen before that are at most MaxItemAge old. An item
// without a date counts as new.
func (s *FileStore) Record(id string, res Result, now time.Time) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state.Feeds[id]
	st.Validators = res.Validators
	primed := st.Primed
	st.Primed = true
	s.state.Feeds[id] = st

	var fresh []Item
	for i := 0; i < len(res.Items) && i < MaxItems; i++ {
		it := res.Items[i]
		key := ItemKey(id, it.ID)
		if _, seen := s.state.Seen[key]; seen {
			continue
		}
		s.state.Seen[key] = now
		if primed && (it.Published.IsZero() || now.Sub(it.Published) <= MaxItemAge) {
			fresh = append(fresh, it)
		}
	}
	s.evictOldest()
	return fresh
}

// Forget implements Store.
func (s *FileStore) Forget(id, itemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Seen, ItemKey(id, itemID))
}

// Hold implements Store. Past MaxHeldPerChannel the oldest items of the channel go first.
func (s *FileStore) Hold(channel string, items []Item, now time.Time) {
	if len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.state.Held[channel]
	for i := 0; i < len(items) && i < MaxItems; i++ {
		held = append(held, HeldItem{Title: items[i].Title, Link: items[i].Link, Held: now})
	}
	if over := len(held) - MaxHeldPerChannel; over > 0 {
		held = slices.Clone(held[over:])
	}
	s.state.Held[channel] = held
}

// Held implements Store.
func (s *FileStore) Held(channel string) []HeldItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.state.Held[channel])
}

// HeldChannels implements Store.
func (s *FileStore) HeldChannels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Sorted(maps.Keys(s.state.Held))
}

// ClearHeld implements Store.
func (s *FileStore) ClearHeld(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Held, channel)
}

// SetNextRead implements Store.
func (s *FileStore) SetNextRead(id string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state.Feeds[id]
	st.NextRead = at
	s.state.Feeds[id] = st
}

// KeepFeeds implements Store.
func (s *FileStore) KeepFeeds(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.DeleteFunc(s.state.Feeds, func(id string, _ FeedState) bool { return !slices.Contains(ids, id) })
}

// Expire implements Store.
func (s *FileStore) Expire(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.DeleteFunc(s.state.Seen, func(_ string, at time.Time) bool { return now.Sub(at) > KeyRetention })
}

// evictOldest drops the oldest keys past MaxKeys. The caller holds s.mu.
func (s *FileStore) evictOldest() {
	over := len(s.state.Seen) - MaxKeys
	if over <= 0 {
		return
	}
	keys := slices.Collect(maps.Keys(s.state.Seen))
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(s.state.Seen[a].Compare(s.state.Seen[b]), cmp.Compare(a, b))
	})
	for _, k := range keys[:over] {
		delete(s.state.Seen, k)
	}
}

// Save implements Store: it writes a new file next to the old one, flushes it to disk and renames
// it over the old one in one step.
func (s *FileStore) Save(ctx context.Context) error {
	s.mu.Lock()
	data, err := json.Marshal(s.state)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("encode news state: %w", err)
	}
	if len(data) > MaxStateBytes {
		return fmt.Errorf("news state of %d bytes exceeds max %d", len(data), MaxStateBytes)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save news state: %w", err)
	}
	return writeAtomic(s.path, data)
}

func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".news-state-*")
	if err != nil {
		return fmt.Errorf("create news state: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, removeIfExists(tmp.Name()))
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write news state: %w", err), tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(fmt.Errorf("flush news state: %w", err), tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close news state: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace news state: %w", err)
	}
	return syncDir(dir)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove temporary news state: %w", err)
	}
	return nil
}

// syncDir flushes the directory entry of the rename, so the new file survives a power loss.
func syncDir(dir string) error {
	d, err := os.Open(dir) // #nosec G304 -- the directory of the state file the operator names.
	if err != nil {
		return fmt.Errorf("open news state directory: %w", err)
	}
	if err := d.Sync(); err != nil {
		return errors.Join(fmt.Errorf("flush news state directory: %w", err), d.Close())
	}
	if err := d.Close(); err != nil {
		return fmt.Errorf("close news state directory: %w", err)
	}
	return nil
}
