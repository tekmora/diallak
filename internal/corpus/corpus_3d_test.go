package corpus

import (
	"strings"
	"testing"
)

func TestFormatURL_Positive_SubstitutesDistinctPath(t *testing.T) {
	c := Chunk{
		Repository: "owner/my-repo",
		Revision:   "v1.0",
		Path:       "docs/guide.md",
	}
	got := c.FormatURL("https://host/{repository}/{revision}/{path}")
	want := "https://host/owner/my-repo/v1.0/docs/guide.md"
	if got != want {
		t.Fatalf("FormatURL() = %q, want %q", got, want)
	}
}

func TestFormatURL_Negative_NoExtraneousTrailingSlash(t *testing.T) {
	c := Chunk{
		Repository: "owner/repo",
		Revision:   "rev",
		Path:       "README.md",
	}
	tpl := "https://example.org/{repository}/{path}"
	got := c.FormatURL(tpl)
	want := "https://example.org/owner/repo/README.md"
	if got != want || strings.HasSuffix(got, "/") {
		t.Fatalf("FormatURL() = %q, want exact %q without trailing slash", got, want)
	}
}

func TestFormatURL_Boundary_EmptyTemplateUsesDefault(t *testing.T) {
	c := Chunk{
		Repository: "owner/repo",
		Revision:   "rev",
		Path:       "doc.md",
	}
	got := c.FormatURL("")
	want := "https://github.com/owner/repo/blob/rev/doc.md"
	if got != want {
		t.Fatalf("FormatURL(\"\") = %q, want default %q", got, want)
	}
}
