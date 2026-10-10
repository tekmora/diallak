package corpus

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestChunkURL_Positive_FormatsValidURL(t *testing.T) {
	c := Chunk{
		Repository: "lusoris/SnatchArr",
		Revision:   "abc1234",
		Path:       "docs/concepts.md",
	}
	want := "https://github.com/lusoris/SnatchArr/blob/abc1234/docs/concepts.md"
	if got := c.URL(); got != want {
		t.Errorf("c.URL() = %q, want %q", got, want)
	}
}

func TestDefaultLimits_Positive_HasStandardValues(t *testing.T) {
	lim := DefaultLimits()
	if lim.MaxFiles != 512 || lim.MaxFileBytes != 1<<20 || lim.MaxChunks != 8192 || lim.MaxChunkBytes != 4000 {
		t.Errorf("DefaultLimits() unexpected values: %+v", lim)
	}
}

func TestLoad_Positive_LoadsAndChunksFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md": &fstest.MapFile{
			Data: []byte("# Project\n\nIntro text.\n\n## Section 1\n\nSection 1 text.\n"),
		},
		"docs/guide.md": &fstest.MapFile{
			Data: []byte("<!-- comment -->\n# Guide\n\nGuide content.\n"),
		},
		"ignored.txt": &fstest.MapFile{
			Data: []byte("ignore me"),
		},
	}
	src := Source{
		Repository: "owner/repo",
		Revision:   "main",
		Patterns:   []string{"README.md", "docs/**/*.md"},
	}
	chunks, err := Load(context.Background(), fsys, src, DefaultLimits())
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	if chunks[0].ID != "README.md#1" || chunks[0].Title != "Project" || chunks[0].Heading != "Project" {
		t.Errorf("chunk[0] = %+v", chunks[0])
	}
	if chunks[1].ID != "README.md#2" || chunks[1].Title != "Project" || chunks[1].Heading != "Section 1" {
		t.Errorf("chunk[1] = %+v", chunks[1])
	}
	if chunks[2].ID != "docs/guide.md#1" || chunks[2].Title != "Guide" {
		t.Errorf("chunk[2] = %+v", chunks[2])
	}
}

func TestLoad_Positive_HTMLCommentsAndFences(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{
			Data: []byte("<!-- multiline\ncomment -->\n# Code Sample\n```go\n# Not a heading\n<!-- not a comment -->\n```\nAfter fence."),
		},
	}
	src := Source{
		Repository: "owner/repo",
		Revision:   "v1.0.0",
		Patterns:   []string{"test.md"},
	}
	chunks, err := Load(context.Background(), fsys, src, DefaultLimits())
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0].Text, "# Not a heading") {
		t.Errorf("expected code fence content preserved, got %q", chunks[0].Text)
	}
	if strings.Contains(chunks[0].Text, "multiline") {
		t.Errorf("expected HTML comment removed, got %q", chunks[0].Text)
	}
}

func TestLoad_Negative_InvalidSourceAndLimits(t *testing.T) {
	validFS := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title\nBody")}}
	cases := []struct {
		name string
		src  Source
		lim  Limits
		want string
	}{
		{"invalid repo", Source{Repository: "bad", Revision: "main", Patterns: []string{"*.md"}}, DefaultLimits(), "invalid repository format"},
		{"invalid rev", Source{Repository: "a/b", Revision: "rev with space", Patterns: []string{"*.md"}}, DefaultLimits(), "invalid revision format"},
		{"empty patterns", Source{Repository: "a/b", Revision: "main", Patterns: nil}, DefaultLimits(), "source patterns slice is empty"},
		{"invalid pattern", Source{Repository: "a/b", Revision: "main", Patterns: []string{"/bad"}}, DefaultLimits(), "invalid pattern"},
		{"zero max files", Source{Repository: "a/b", Revision: "main", Patterns: []string{"*.md"}}, Limits{0, 100, 10, 100}, "limits must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(context.Background(), validFS, tc.src, tc.lim)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestLoad_Negative_FileErrorsAndContext(t *testing.T) {
	src := Source{Repository: "owner/repo", Revision: "v1", Patterns: []string{"*.md"}}

	// No matched files
	fsEmpty := fstest.MapFS{"test.txt": &fstest.MapFile{Data: []byte("text")}}
	if _, err := Load(context.Background(), fsEmpty, src, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "no files matched") {
		t.Errorf("Load with zero matches error = %v, want no files matched", err)
	}

	// Invalid UTF-8
	fsBadUTF8 := fstest.MapFS{"test.md": &fstest.MapFile{Data: []byte{0xff, 0xfe, 0xfd}}}
	if _, err := Load(context.Background(), fsBadUTF8, src, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("Load bad UTF-8 error = %v, want not valid UTF-8", err)
	}

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fsValid := fstest.MapFS{"test.md": &fstest.MapFile{Data: []byte("# T\nB")}}
	if _, err := Load(ctx, fsValid, src, DefaultLimits()); err == nil {
		t.Errorf("Load with canceled ctx = nil, want context error")
	}
}

func TestLoad_Boundary_LimitsAndSizes(t *testing.T) {
	src := Source{Repository: "owner/repo", Revision: "v1", Patterns: []string{"*.md"}}

	// File exceeds MaxFileBytes
	fsOversized := fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("# Title\n" + strings.Repeat("x", 20))}}
	limSmallFile := Limits{MaxFiles: 10, MaxFileBytes: 10, MaxChunks: 10, MaxChunkBytes: 10}
	if _, err := Load(context.Background(), fsOversized, src, limSmallFile); err == nil || !strings.Contains(err.Error(), "exceeds max file size") {
		t.Errorf("Load oversized file error = %v, want exceeds max file size", err)
	}

	// MaxFiles exceeded
	fsManyFiles := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# A\nText")},
		"b.md": &fstest.MapFile{Data: []byte("# B\nText")},
	}
	limOneFile := Limits{MaxFiles: 1, MaxFileBytes: 1000, MaxChunks: 10, MaxChunkBytes: 1000}
	if _, err := Load(context.Background(), fsManyFiles, src, limOneFile); err == nil || !strings.Contains(err.Error(), "exceeds max file count") {
		t.Errorf("Load too many files error = %v, want exceeds max file count", err)
	}

	// MaxChunks exceeded
	fsMultiChunk := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# S1\nText 1\n## S2\nText 2\n## S3\nText 3\n")},
	}
	limFewChunks := Limits{MaxFiles: 5, MaxFileBytes: 1000, MaxChunks: 2, MaxChunkBytes: 1000}
	if _, err := Load(context.Background(), fsMultiChunk, src, limFewChunks); err == nil || !strings.Contains(err.Error(), "causes total chunks to exceed") {
		t.Errorf("Load too many chunks error = %v, want causes total chunks to exceed", err)
	}

	// Chunk split at MaxChunkBytes
	fsSplit := fstest.MapFS{
		"split.md": &fstest.MapFile{Data: []byte("# Title\n\nPara 1\n\nPara 2\n\nPara 3\n")},
	}
	limSmallChunk := Limits{MaxFiles: 5, MaxFileBytes: 1000, MaxChunks: 10, MaxChunkBytes: 15}
	chunks, err := Load(context.Background(), fsSplit, src, limSmallChunk)
	if err != nil {
		t.Fatalf("Load split chunk = %v, want nil", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected chunk splitting, got %d chunks", len(chunks))
	}
}

func TestLoad_Negative_RefusesMatchedNonRegularFile(t *testing.T) {
	src := Source{Repository: "owner/repo", Revision: "v1", Patterns: []string{"docs/**/*.md"}}
	// A checkout can carry a symbolic link whose target lies outside the corpus directory. The
	// loader must not read through it, so a matched entry that is no regular file is an error.
	fsys := fstest.MapFS{
		"docs/real.md": &fstest.MapFile{Data: []byte("# Real\nBody")},
		"docs/link.md": &fstest.MapFile{Data: []byte("real.md"), Mode: fs.ModeSymlink},
	}
	_, err := Load(context.Background(), fsys, src, DefaultLimits())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") || !strings.Contains(err.Error(), "docs/link.md") {
		t.Fatalf("Load err = %v, want an error naming docs/link.md as not a regular file", err)
	}
}

func TestLoad_Boundary_MaxChunkBytesHoldsOneRune(t *testing.T) {
	src := Source{Repository: "owner/repo", Revision: "v1", Patterns: []string{"*.md"}}
	fsys := fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("# T\nBody")}}
	// One rune takes up to four bytes; a smaller chunk limit cannot cut on a rune boundary.
	if _, err := Load(context.Background(), fsys, src, Limits{MaxFiles: 1, MaxFileBytes: 100, MaxChunks: 10, MaxChunkBytes: 3}); err == nil || !strings.Contains(err.Error(), "MaxChunkBytes") {
		t.Fatalf("Load with MaxChunkBytes 3 err = %v, want an error naming MaxChunkBytes", err)
	}
	if _, err := Load(context.Background(), fsys, src, Limits{MaxFiles: 1, MaxFileBytes: 100, MaxChunks: 10, MaxChunkBytes: 4}); err != nil {
		t.Fatalf("Load with MaxChunkBytes 4 err = %v, want nil", err)
	}
}
