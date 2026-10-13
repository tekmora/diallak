package corpus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func validManifestJSON() string {
	return `{
		"version": 1,
		"sources": [
			{
				"directory": "snatcharr",
				"repository": "lusoris/SnatchArr",
				"revision": "9e70c2e3f4",
				"paths": ["docs/*.md"]
			}
		]
	}`
}

func TestLoadManifest_Positive_Valid(t *testing.T) {
	fsys := fstest.MapFS{
		ManifestName: &fstest.MapFile{Data: []byte(validManifestJSON())},
	}

	m, err := LoadManifest(fsys)
	if err != nil {
		t.Fatalf("LoadManifest = %v, want nil", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("len(Sources) = %d, want 1", len(m.Sources))
	}
	src := m.Sources[0]
	if src.Directory != "snatcharr" || src.Repository != "lusoris/SnatchArr" || src.Revision != "9e70c2e3f4" {
		t.Errorf("unexpected source metadata: %+v", src)
	}
}

func TestLoadAll_Positive_MultiSourcePrefixesAndRevisions(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Sources: []ManifestSource{
			{
				Directory:  "repo-a",
				Repository: "tekmora/project-a",
				Revision:   "rev-aaaa-1111",
				Patterns:   []string{"README.md"},
			},
			{
				Directory:  "repo-b",
				Repository: "tekmora/project-b",
				Revision:   "rev-bbbb-2222",
				Patterns:   []string{"README.md"},
			},
		},
	}

	fsys := fstest.MapFS{
		"repo-a/README.md": &fstest.MapFile{Data: []byte("# Project A\nAlpha documentation.")},
		"repo-b/README.md": &fstest.MapFile{Data: []byte("# Project B\nBeta documentation.")},
	}

	chunks, err := LoadAll(context.Background(), fsys, manifest, DefaultLimits())
	if err != nil {
		t.Fatalf("LoadAll = %v, want nil", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}

	// Verify both sources kept, IDs differ by prefix, Revision per source correct
	chunkA := chunks[0]
	chunkB := chunks[1]

	if !strings.HasPrefix(chunkA.ID, "tekmora/project-a:") {
		t.Errorf("chunk A ID %q does not have tekmora/project-a prefix", chunkA.ID)
	}
	if !strings.HasPrefix(chunkB.ID, "tekmora/project-b:") {
		t.Errorf("chunk B ID %q does not have tekmora/project-b prefix", chunkB.ID)
	}
	if chunkA.ID == chunkB.ID {
		t.Errorf("chunk IDs collision: %q", chunkA.ID)
	}
	if chunkA.Revision != "rev-aaaa-1111" {
		t.Errorf("chunk A revision = %q, want rev-aaaa-1111", chunkA.Revision)
	}
	if chunkB.Revision != "rev-bbbb-2222" {
		t.Errorf("chunk B revision = %q, want rev-bbbb-2222", chunkB.Revision)
	}
	if chunkA.Path != "README.md" || chunkB.Path != "README.md" {
		t.Errorf("unexpected chunk paths: %q, %q", chunkA.Path, chunkB.Path)
	}
}

func TestLoadManifest_Negative_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"invalid json", `{ "version": 1, `, "decode manifest"},
		{"unknown field", `{ "version": 1, "extra": "field", "sources": [] }`, "unknown field"},
		{"trailing data", validManifestJSON() + " trailing", "trailing data"},
		{"version zero", `{ "version": 0, "sources": [{"directory": "d", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "unsupported manifest version"},
		{"version two", `{ "version": 2, "sources": [{"directory": "d", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "unsupported manifest version"},
		{"zero sources", `{ "version": 1, "sources": [] }`, "no sources"},
		{"duplicate directory", `{ "version": 1, "sources": [
			{"directory": "dir", "repository": "a/b", "revision": "r", "paths": ["*.md"]},
			{"directory": "dir", "repository": "c/d", "revision": "r", "paths": ["*.md"]}
		]}`, "duplicate source directory"},
		{"duplicate repository", `{ "version": 1, "sources": [
			{"directory": "dir1", "repository": "a/b", "revision": "r", "paths": ["*.md"]},
			{"directory": "dir2", "repository": "a/b", "revision": "r", "paths": ["*.md"]}
		]}`, "duplicate repository"},
		{"empty directory", `{ "version": 1, "sources": [{"directory": "", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"directory with slash", `{ "version": 1, "sources": [{"directory": "a/b", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"directory with backslash", `{ "version": 1, "sources": [{"directory": "a\\b", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"directory dot", `{ "version": 1, "sources": [{"directory": ".", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"directory dot dot", `{ "version": 1, "sources": [{"directory": "..", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"directory leading dot", `{ "version": 1, "sources": [{"directory": ".git", "repository": "a/b", "revision": "r", "paths": ["*.md"]}] }`, "invalid directory"},
		{"invalid repository format", `{ "version": 1, "sources": [{"directory": "dir", "repository": "invalid", "revision": "r", "paths": ["*.md"]}] }`, "invalid repository format"},
		{"invalid revision format", `{ "version": 1, "sources": [{"directory": "dir", "repository": "a/b", "revision": "rev with space", "paths": ["*.md"]}] }`, "invalid revision format"},
		{"empty paths", `{ "version": 1, "sources": [{"directory": "dir", "repository": "a/b", "revision": "r", "paths": []}] }`, "patterns slice is empty"},
		{"invalid path pattern", `{ "version": 1, "sources": [{"directory": "dir", "repository": "a/b", "revision": "r", "paths": ["["]}] }`, "invalid pattern"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				ManifestName: &fstest.MapFile{Data: []byte(tc.json)},
			}
			_, err := LoadManifest(fsys)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadManifest error = %v, want one naming %q", err, tc.want)
			}
		})
	}

	t.Run("manifest absent", func(t *testing.T) {
		fsys := fstest.MapFS{}
		_, err := LoadManifest(fsys)
		if err == nil || !strings.Contains(err.Error(), "open manifest") {
			t.Fatalf("LoadManifest absent error = %v, want open manifest error", err)
		}
	})
}

func TestLoadAll_Negative_Errors(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Sources: []ManifestSource{
			{
				Directory:  "checkout",
				Repository: "tekmora/project",
				Revision:   "rev1",
				Patterns:   []string{"*.md"},
			},
		},
	}

	t.Run("source directory missing", func(t *testing.T) {
		fsys := fstest.MapFS{}
		_, err := LoadAll(context.Background(), fsys, manifest, DefaultLimits())
		if err == nil || !strings.Contains(err.Error(), "tekmora/project") {
			t.Fatalf("LoadAll with missing directory = %v, want error naming repository", err)
		}
	})

	t.Run("no files matched", func(t *testing.T) {
		fsys := fstest.MapFS{
			"checkout/other.txt": &fstest.MapFile{Data: []byte("text")},
		}
		_, err := LoadAll(context.Background(), fsys, manifest, DefaultLimits())
		if err == nil || !strings.Contains(err.Error(), "tekmora/project") {
			t.Fatalf("LoadAll with unmatched files = %v, want error naming repository", err)
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		fsys := fstest.MapFS{
			"checkout/doc.md": &fstest.MapFile{Data: []byte("# Doc\nContent")},
		}
		_, err := LoadAll(ctx, fsys, manifest, DefaultLimits())
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("LoadAll with cancelled context = %v, want context canceled", err)
		}
	})
}

// testLoadManifestSourceCountLimits verifies manifest parsing at and beyond MaxSources limit.
func testLoadManifestSourceCountLimits(t *testing.T) {
	t.Helper()
	var sourcesAtLimit []string
	for i := 0; i < MaxSources; i++ {
		sourcesAtLimit = append(sourcesAtLimit, fmt.Sprintf(
			`{"directory": "dir%d", "repository": "repo/name%d", "revision": "rev", "paths": ["*.md"]}`, i, i))
	}
	jsonAtLimit := fmt.Sprintf(`{"version": 1, "sources": [%s]}`, strings.Join(sourcesAtLimit, ","))
	fsys := fstest.MapFS{
		ManifestName: &fstest.MapFile{Data: []byte(jsonAtLimit)},
	}
	m, err := LoadManifest(fsys)
	if err != nil {
		t.Fatalf("LoadManifest with MaxSources = %v, want nil", err)
	}
	if len(m.Sources) != MaxSources {
		t.Errorf("len(m.Sources) = %d, want %d", len(m.Sources), MaxSources)
	}

	// MaxSources + 1
	sourcesOver := append(sourcesAtLimit, fmt.Sprintf(
		`{"directory": "dir%d", "repository": "repo/name%d", "revision": "rev", "paths": ["*.md"]}`, MaxSources, MaxSources))
	jsonOver := fmt.Sprintf(`{"version": 1, "sources": [%s]}`, strings.Join(sourcesOver, ","))
	fsysOver := fstest.MapFS{
		ManifestName: &fstest.MapFile{Data: []byte(jsonOver)},
	}
	if _, err := LoadManifest(fsysOver); err == nil || !strings.Contains(err.Error(), "exceeding limit") {
		t.Fatalf("LoadManifest over MaxSources = %v, want exceeding limit error", err)
	}
}

// testLoadManifestFileSizeLimit verifies manifest parsing at and beyond MaxManifestBytes limit.
func testLoadManifestFileSizeLimit(t *testing.T) {
	t.Helper()
	// Create padded valid JSON up to MaxManifestBytes
	base := validManifestJSON()
	paddingLen := MaxManifestBytes - len(base)
	if paddingLen > 0 {
		padded := base + strings.Repeat(" ", paddingLen)
		fsys := fstest.MapFS{
			ManifestName: &fstest.MapFile{Data: []byte(padded)},
		}
		if _, err := LoadManifest(fsys); err != nil {
			t.Fatalf("LoadManifest at MaxManifestBytes = %v, want nil", err)
		}
	}

	// Over limit
	paddedOver := base + strings.Repeat(" ", MaxManifestBytes-len(base)+1)
	fsysOver := fstest.MapFS{
		ManifestName: &fstest.MapFile{Data: []byte(paddedOver)},
	}
	if _, err := LoadManifest(fsysOver); err == nil || !strings.Contains(err.Error(), "exceeds max size") {
		t.Fatalf("LoadManifest over MaxManifestBytes = %v, want exceeds max size error", err)
	}
}

func TestLoadManifest_Boundary_SizeAndSourceCount(t *testing.T) {
	t.Run("source count limits", func(t *testing.T) {
		testLoadManifestSourceCountLimits(t)
	})

	t.Run("manifest file size limit", func(t *testing.T) {
		testLoadManifestFileSizeLimit(t)
	})
}

func TestLoadAll_Boundary_ChunkLimits(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Sources: []ManifestSource{
			{
				Directory:  "dir1",
				Repository: "repo/one",
				Revision:   "rev1",
				Patterns:   []string{"*.md"},
			},
			{
				Directory:  "dir2",
				Repository: "repo/two",
				Revision:   "rev2",
				Patterns:   []string{"*.md"},
			},
		},
	}
	fsys := fstest.MapFS{
		"dir1/a.md": &fstest.MapFile{Data: []byte("# Title A\nContent A")},
		"dir2/b.md": &fstest.MapFile{Data: []byte("# Title B\nContent B")},
	}

	lim := DefaultLimits()
	lim.MaxChunks = 2
	chunks, err := LoadAll(context.Background(), fsys, manifest, lim)
	if err != nil {
		t.Fatalf("LoadAll at chunk limit = %v, want nil", err)
	}
	if len(chunks) != 2 {
		t.Errorf("len(chunks) = %d, want 2", len(chunks))
	}

	limOver := DefaultLimits()
	limOver.MaxChunks = 1
	if _, err := LoadAll(context.Background(), fsys, manifest, limOver); err == nil || !strings.Contains(err.Error(), "exceed limit") {
		t.Fatalf("LoadAll over chunk limit = %v, want exceed limit error", err)
	}
}
