package corpus

import (
	"strings"
	"testing"
)

// A hash line inside a fenced code block is code, not a heading: the file stays one chunk.
func TestParseFileChunks_Positive_FenceHidesHeading(t *testing.T) {
	cases := map[string]string{
		"plain backtick fence":       "# Title\n\n```sh\n# not a heading\n```\nafter",
		"plain tilde fence":          "# Title\n\n~~~\n# not a heading\n~~~\nafter",
		"longer fence holds shorter": "# Title\n\n````md\n```\n# not a heading\n```\n````\nafter",
		"tilde fence holds backtick": "# Title\n\n~~~md\n```\n# not a heading\n~~~\nafter",
		"backtick fence holds tilde": "# Title\n\n```md\n~~~\n# not a heading\n```\nafter",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
			if len(chunks) != 1 || chunks[0].Heading != "Title" || !strings.Contains(chunks[0].Text, "# not a heading") {
				t.Fatalf("chunks = %+v, want one chunk under Title holding the code line", chunks)
			}
		})
	}
}

func TestParseFileChunks_Boundary_ClosingFenceTakesNoText(t *testing.T) {
	// A fence line followed by text does not close the block; the hash line after it is code.
	doc := "# Title\n\n```\n``` still code\n# not a heading\n```\nafter"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "# not a heading") {
		t.Fatalf("chunks = %+v, want one chunk holding the code line", chunks)
	}
}

func TestParseFileChunks_Negative_BacktickInInfoStringOpensNoFence(t *testing.T) {
	// "```a`b" is inline code, not a fence, so the heading after it still splits the file.
	doc := "# Title\n\n```a`b\n\n# Two\n\nbody"
	chunks := parseFileChunks("o/r", "v1", "a.md", doc, 4000)
	if len(chunks) != 2 || chunks[1].Heading != "Two" {
		t.Fatalf("chunks = %+v, want a second chunk under Two", chunks)
	}
}
