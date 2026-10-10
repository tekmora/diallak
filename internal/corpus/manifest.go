// Package corpus loads, parses, and chunks documentation files from a filesystem.
package corpus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

const (
	// ManifestName is the expected filename of the corpus manifest file.
	ManifestName = "manifest.json"
	// MaxManifestBytes is the maximum allowed size of the manifest file (1 MiB).
	MaxManifestBytes = 1 << 20
	// MaxSources is the maximum number of source checkouts allowed in a manifest.
	MaxSources = 64
)

// Manifest lists the documentation sources that compose the corpus.
type Manifest struct {
	Version int              `json:"version"`
	Sources []ManifestSource `json:"sources"`
}

// ManifestSource specifies one source checkout directory and repository metadata.
type ManifestSource struct {
	Directory  string   `json:"directory"`
	Repository string   `json:"repository"`
	Revision   string   `json:"revision"`
	Patterns   []string `json:"paths"`
}

// LoadManifest reads and strictly validates the corpus manifest from fsys.
func LoadManifest(fsys fs.FS) (_ Manifest, err error) {
	file, err := fsys.Open(ManifestName)
	if err != nil {
		return Manifest{}, fmt.Errorf("open manifest: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close manifest: %w", closeErr))
		}
	}()

	data, err := io.ReadAll(io.LimitReader(file, int64(MaxManifestBytes+1)))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("manifest exceeds max size of %d bytes", MaxManifestBytes)
	}

	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("manifest contains trailing data")
	}

	if err := validateManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func validateManifest(m Manifest) error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if len(m.Sources) == 0 {
		return errors.New("manifest contains no sources")
	}
	if len(m.Sources) > MaxSources {
		return fmt.Errorf("manifest contains %d sources, exceeding limit of %d", len(m.Sources), MaxSources)
	}

	seenDirs := make(map[string]struct{}, len(m.Sources))
	seenRepos := make(map[string]struct{}, len(m.Sources))

	for i, s := range m.Sources {
		if !isValidSourceDirectory(s.Directory) {
			return fmt.Errorf("source %d has invalid directory %q: must be a single clean path element", i, s.Directory)
		}
		if _, ok := seenDirs[s.Directory]; ok {
			return fmt.Errorf("duplicate source directory %q", s.Directory)
		}
		seenDirs[s.Directory] = struct{}{}

		if _, ok := seenRepos[s.Repository]; ok {
			return fmt.Errorf("duplicate repository %q", s.Repository)
		}
		seenRepos[s.Repository] = struct{}{}

		srcDef := Source{
			Repository: s.Repository,
			Revision:   s.Revision,
			Patterns:   s.Patterns,
		}
		if err := validateSource(srcDef); err != nil {
			return fmt.Errorf("source %q: %w", s.Repository, err)
		}
	}
	return nil
}

func isValidSourceDirectory(dir string) bool {
	if dir == "" || dir == "." || dir == ".." || strings.HasPrefix(dir, ".") {
		return false
	}
	if strings.Contains(dir, "/") || strings.Contains(dir, `\`) {
		return false
	}
	return true
}

// LoadAll loads and chunks documents from all sources listed in manifest m.
func LoadAll(ctx context.Context, fsys fs.FS, m Manifest, lim Limits) ([]Chunk, error) {
	if err := validateLimits(lim); err != nil {
		return nil, fmt.Errorf("validate limits: %w", err)
	}
	if len(m.Sources) == 0 {
		return nil, errors.New("manifest contains no sources")
	}
	if len(m.Sources) > MaxSources {
		return nil, fmt.Errorf("manifest contains %d sources, exceeding limit of %d", len(m.Sources), MaxSources)
	}

	var allChunks []Chunk
	for _, src := range m.Sources {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		sub, err := fs.Sub(fsys, src.Directory)
		if err != nil {
			return nil, fmt.Errorf("source %q directory %q: %w", src.Repository, src.Directory, err)
		}

		sourceDef := Source{
			Repository: src.Repository,
			Revision:   src.Revision,
			Patterns:   src.Patterns,
		}
		chunks, err := Load(ctx, sub, sourceDef, lim)
		if err != nil {
			return nil, fmt.Errorf("load source %q: %w", src.Repository, err)
		}

		if len(allChunks)+len(chunks) > lim.MaxChunks {
			return nil, fmt.Errorf("source %q causes total chunks to exceed limit of %d", src.Repository, lim.MaxChunks)
		}

		for i := range chunks {
			chunks[i].ID = src.Repository + ":" + chunks[i].ID
		}
		allChunks = append(allChunks, chunks...)
	}
	return allChunks, nil
}
