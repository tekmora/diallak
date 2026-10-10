// Package corpus loads, parses, and chunks documentation files from a filesystem.
package corpus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"unicode/utf8"
)

var (
	repoRegexp = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	revRegexp  = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Source identifies a documentation source repository and file paths.
type Source struct {
	Repository string   `json:"repository"`
	Revision   string   `json:"revision"`
	Patterns   []string `json:"paths"`
}

// Chunk represents one indexed segment of a documentation document.
type Chunk struct {
	ID         string
	Repository string
	Revision   string
	Path       string
	Title      string
	Heading    string
	Text       string
}

// URL returns the canonical GitHub web URL for this chunk's document.
func (c Chunk) URL() string {
	return fmt.Sprintf("https://github.com/%s/blob/%s/%s", c.Repository, c.Revision, c.Path)
}

// Limits defines the upper bounds applied when loading a corpus from an fs.FS.
type Limits struct {
	MaxFiles      int
	MaxFileBytes  int
	MaxChunks     int
	MaxChunkBytes int
}

// DefaultLimits returns the standard bounded limits for corpus loading.
func DefaultLimits() Limits {
	return Limits{
		MaxFiles:      512,
		MaxFileBytes:  1 << 20,
		MaxChunks:     8192,
		MaxChunkBytes: 4000,
	}
}

type loadState struct {
	ctx          context.Context
	fsys         fs.FS
	src          Source
	lim          Limits
	matchedFiles int
	allChunks    []Chunk
}

// Load walks fsys in lexical order, filtering by src.Patterns, and chunks all matching files.
func Load(ctx context.Context, fsys fs.FS, src Source, lim Limits) ([]Chunk, error) {
	if err := validateSourceAndLimits(src, lim); err != nil {
		return nil, fmt.Errorf("validate source or limits: %w", err)
	}

	state := &loadState{ctx: ctx, fsys: fsys, src: src, lim: lim}
	walkErr := fs.WalkDir(fsys, ".", state.handleEntry)
	if walkErr != nil {
		return nil, fmt.Errorf("load corpus: %w", walkErr)
	}
	if state.matchedFiles == 0 {
		return nil, errors.New("no files matched source patterns")
	}
	return state.allChunks, nil
}

func (s *loadState) handleEntry(p string, d fs.DirEntry, err error) error {
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return fmt.Errorf("walk %q: %w", p, err)
	}
	if d.IsDir() {
		return nil
	}

	matched, mErr := matchAnyPattern(s.src.Patterns, p)
	if mErr != nil {
		return fmt.Errorf("match patterns for %q: %w", p, mErr)
	}
	if !matched {
		return nil
	}

	s.matchedFiles++
	if s.matchedFiles > s.lim.MaxFiles {
		return fmt.Errorf("file %q exceeds max file count limit of %d", p, s.lim.MaxFiles)
	}

	chunks, readErr := readAndChunkFile(s.fsys, p, s.src, s.lim)
	if readErr != nil {
		return readErr
	}

	s.allChunks = append(s.allChunks, chunks...)
	if len(s.allChunks) > s.lim.MaxChunks {
		return fmt.Errorf("file %q causes total chunks to exceed limit of %d", p, s.lim.MaxChunks)
	}
	return nil
}

func validateSourceAndLimits(src Source, lim Limits) error {
	if !repoRegexp.MatchString(src.Repository) {
		return fmt.Errorf("invalid repository format %q", src.Repository)
	}
	if !revRegexp.MatchString(src.Revision) {
		return fmt.Errorf("invalid revision format %q", src.Revision)
	}
	if len(src.Patterns) == 0 {
		return errors.New("source patterns slice is empty")
	}
	for _, p := range src.Patterns {
		if _, err := Match(p, "probe"); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", p, err)
		}
	}
	if lim.MaxFiles <= 0 || lim.MaxFileBytes <= 0 || lim.MaxChunks <= 0 || lim.MaxChunkBytes <= 0 {
		return errors.New("all limits must be positive")
	}
	return nil
}

func matchAnyPattern(patterns []string, p string) (bool, error) {
	for _, pat := range patterns {
		m, err := Match(pat, p)
		if err != nil {
			return false, err
		}
		if m {
			return true, nil
		}
	}
	return false, nil
}

func readAndChunkFile(fsys fs.FS, p string, src Source, lim Limits) (_ []Chunk, err error) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", p, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %q: %w", p, closeErr))
		}
	}()

	data, err := io.ReadAll(io.LimitReader(f, int64(lim.MaxFileBytes+1)))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", p, err)
	}
	if len(data) > lim.MaxFileBytes {
		return nil, fmt.Errorf("file %q exceeds max file size of %d bytes", p, lim.MaxFileBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("file %q is not valid UTF-8", p)
	}
	return parseFileChunks(src.Repository, src.Revision, p, string(data), lim.MaxChunkBytes), nil
}
