package corpus

import (
	"strings"
	"testing"
)

func TestParseFileChunks_Negative_IndentedCodeBlockOpensNoFence(t *testing.T) {
	doc := "# Title\n\n    ```\n\n# Real heading\nbody"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v (len %d), want 2 chunks", chunks, len(chunks))
	}
	if chunks[0].Heading != "Title" {
		t.Errorf("chunk[0].Heading = %q, want %q", chunks[0].Heading, "Title")
	}
	if chunks[1].Heading != "Real heading" {
		t.Errorf("chunk[1].Heading = %q, want %q", chunks[1].Heading, "Real heading")
	}
	if chunks[1].Text != "body" {
		t.Errorf("chunk[1].Text = %q, want %q", chunks[1].Text, "body")
	}
}

func TestParseFileChunks_Positive_TabAfterHashIsHeading(t *testing.T) {
	doc := "#\tTabbed\nbody"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v (len %d), want 1 chunk", chunks, len(chunks))
	}
	if chunks[0].Heading != "Tabbed" {
		t.Errorf("chunks[0].Heading = %q, want %q", chunks[0].Heading, "Tabbed")
	}
	if chunks[0].Title != "Tabbed" {
		t.Errorf("chunks[0].Title = %q, want %q", chunks[0].Title, "Tabbed")
	}
	if chunks[0].Text != "body" {
		t.Errorf("chunks[0].Text = %q, want %q", chunks[0].Text, "body")
	}
}

func TestParseFileChunks_Positive_SetextHeading(t *testing.T) {
	doc := "Setext\n======\nbody"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v (len %d), want 1 chunk", chunks, len(chunks))
	}
	if chunks[0].Heading != "Setext" {
		t.Errorf("chunks[0].Heading = %q, want %q", chunks[0].Heading, "Setext")
	}
	if chunks[0].Title != "Setext" {
		t.Errorf("chunks[0].Title = %q, want %q", chunks[0].Title, "Setext")
	}
	if chunks[0].Text != "body" {
		t.Errorf("chunks[0].Text = %q, want %q", chunks[0].Text, "body")
	}
	if strings.Contains(chunks[0].Text, "======") {
		t.Errorf("chunks[0].Text unexpectedly contains underline: %q", chunks[0].Text)
	}
}

func TestParseFileChunks_Boundary_HTMLCommentHidesHeading(t *testing.T) {
	doc := "# Title\n\n<!--\n# hidden\n-->\nafter"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v (len %d), want 1 chunk", chunks, len(chunks))
	}
	if chunks[0].Heading != "Title" {
		t.Errorf("chunks[0].Heading = %q, want %q", chunks[0].Heading, "Title")
	}
	if chunks[0].Title != "Title" {
		t.Errorf("chunks[0].Title = %q, want %q", chunks[0].Title, "Title")
	}
	if strings.Contains(chunks[0].Text, "hidden") {
		t.Errorf("chunks[0].Text unexpectedly contains hidden heading: %q", chunks[0].Text)
	}
	if strings.Contains(chunks[0].Text, "<!--") || strings.Contains(chunks[0].Text, "-->") {
		t.Errorf("chunks[0].Text unexpectedly contains comment syntax: %q", chunks[0].Text)
	}
}

func TestParseFileChunks_Boundary_CommentInHeadingLeavesHeadingText(t *testing.T) {
	chunks := parseFileChunks("o/r", "v1", "a.md", "# Foo <!-- note -->\nbody", 4000)
	if len(chunks) != 1 || chunks[0].Heading != "Foo" || chunks[0].Title != "Foo" {
		t.Fatalf("chunks = %+v, want one chunk with heading and title %q", chunks, "Foo")
	}
}

func TestParseFileChunks_Boundary_TextAfterCommentBlockKept(t *testing.T) {
	chunks := parseFileChunks("o/r", "v1", "a.md", "<!-- c --> tail\n# H\nbody", 4000)
	if len(chunks) != 2 || chunks[0].Text != "tail" || chunks[1].Heading != "H" {
		t.Fatalf("chunks = %+v, want %q kept before heading %q", chunks, "tail", "H")
	}
}

func TestParseFileChunks_Boundary_ListItemRightAfterATXHeadingIsKept(t *testing.T) {
	doc := "# Guide\n- first step\n- second step"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v (len %d), want 1 chunk", chunks, len(chunks))
	}
	if chunks[0].Heading != "Guide" {
		t.Errorf("chunks[0].Heading = %q, want %q", chunks[0].Heading, "Guide")
	}
	want := "- first step\n- second step"
	if chunks[0].Text != want {
		t.Errorf("chunks[0].Text = %q, want %q", chunks[0].Text, want)
	}
}

func TestParseFileChunks_Boundary_MultiLineSetextHeadingDropsOnlyItsUnderline(t *testing.T) {
	doc := "Two line\nheading\n-------\n- item"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v (len %d), want 1 chunk", chunks, len(chunks))
	}
	if chunks[0].Heading != "Two line heading" {
		t.Errorf("chunks[0].Heading = %q, want %q", chunks[0].Heading, "Two line heading")
	}
	if chunks[0].Text != "- item" {
		t.Errorf("chunks[0].Text = %q, want %q", chunks[0].Text, "- item")
	}
}
