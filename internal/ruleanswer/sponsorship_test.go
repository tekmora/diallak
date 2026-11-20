package ruleanswer

import (
	"testing"

	"github.com/tekmora/diallak/internal/corpus"
)

func TestResolve_Positive_RemovesSponsorshipParagraph(t *testing.T) {
	trigger := fixtureTrigger()
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "Ask for help in Discussions.\n\n[Sponsor us](https://github.com/sponsors/example)"}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "Ask for help in Discussions." {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
}

func TestResolve_Negative_SponsorshipOnlyDisablesTrigger(t *testing.T) {
	for _, url := range []string{"https://github.com/sponsors/example", "https://ko-fi.com/example", "https://opencollective.com/example", "https://liberapay.com/example", "https://patreon.com/example", "https://buymeacoffee.com/example"} {
		trigger := fixtureTrigger()
		chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "[Sponsor](" + url + ")"}}
		resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
		if err != nil || len(issues) != 1 || issues[0] != (Issue{trigger.ID, "sponsorship_only"}) || resolved.DisabledCount != 1 || len(resolved.Answers) != 0 {
			t.Errorf("Resolve(%s) = (%v, %v, %v)", url, resolved, issues, err)
		}
	}
}

func TestResolve_Boundary_SponsorshipListItem(t *testing.T) {
	trigger := fixtureTrigger()
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "- Ask for help in Discussions.\n- [Sponsor](https://ko-fi.com/example)"}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "- Ask for help in Discussions." {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
}

func TestResolve_Positive_IndentedCodePreserved(t *testing.T) {
	input := "    code line\n    second line"
	if got := filterSponsorship(input); got != input {
		t.Errorf("got %q", got)
	}
}

func TestFilterSponsorship_Positive_ListUnchanged(t *testing.T) {
	input := "\n- first\n- second\n\n"
	if got := filterSponsorship(input); got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestFilterSponsorship_Positive_RemoveMiddleItem(t *testing.T) {
	input := "- first\n- [Sponsor](https://ko-fi.com/example)\n- last"
	want := "- first\n- last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorPreservesParent(t *testing.T) {
	input := "- parent\n  - ordinary\n  - [Sponsor](https://ko-fi.com/example)"
	want := "- parent\n  - ordinary"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorBetweenItems(t *testing.T) {
	input := "- parent\n\t1. first\n\t2. [Sponsor](https://ko-fi.com/example)\n\t3. last"
	want := "- parent\n\t1. first\n\t3. last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorContinuation(t *testing.T) {
	input := "- parent\n  - [Sponsor](https://ko-fi.com/example)\n    continuation\n  - ordinary"
	want := "- parent\n  - ordinary"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Positive_RemovesSponsorSubtree(t *testing.T) {
	input := "- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits"
	if got := filterSponsorship(input); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFilterSponsorship_Positive_RemovesSubtreeBetweenItems(t *testing.T) {
	input := "- ordinary\n- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits\n- last"
	want := "- ordinary\n- last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Boundary_SubtreeBeforeParagraph(t *testing.T) {
	input := "- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits\n\nparagraph"
	if got := filterSponsorship(input); got != "paragraph" {
		t.Errorf("got %q, want paragraph", got)
	}
}

func TestResolve_Positive_SponsorshipSentences(t *testing.T) {
	trigger := fixtureTrigger()
	for _, text := range []string{
		"Ask in Discussions for help. You can also [sponsor us](https://github.com/sponsors/example).",
		"You can also [sponsor us](https://github.com/sponsors/example). Ask in Discussions for help.",
		"Ask in Discussions for help. You can also\n[sponsor us](https://github.com/sponsors/example).",
		"Ask in Discussions for help. Sponsor at https://ko-fi.com/example e.g. monthly.",
	} {
		chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: text}}
		resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
		if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "Ask in Discussions for help." {
			t.Errorf("Resolve(%q) = (%v, %v, %v)", text, resolved, issues, err)
		}
	}
}

func TestResolve_Positive_SponsorshipLowercaseSentence(t *testing.T) {
	trigger := fixtureTrigger()
	text := "Sponsor https://ko-fi.com/example. iOS users can ask in Discussions."
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: text}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "iOS users can ask in Discussions." {
		t.Errorf("Resolve(%q) = (%v, %v, %v)", text, resolved, issues, err)
	}
}

func TestFilterSponsorship_Negative_IndentedCodeUnchanged(t *testing.T) {
	text := "Keep this paragraph.\n\n    example. Sponsor https://ko-fi.com/example\n    more code\n\n    safe code\n    more safe code"
	want := "Keep this paragraph.\n\n    safe code\n    more safe code"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_IndentedCodeListLikeLines(t *testing.T) {
	text := "Keep this paragraph.\n\n    keep\n    - example\n    https://ko-fi.com/example\n\n    safe code\n    - safe list-like code\n    more safe code"
	want := "Keep this paragraph.\n\n    safe code\n    - safe list-like code\n    more safe code"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}

func TestFilterSponsorshipSentences_Boundary_NextRune(t *testing.T) {
	for _, suffix := range []string{"Next", "iOS users can ask in Discussions.", "1 next", "[next]", "(next)", "\"Next\"", "'Next'"} {
		text := "Sponsor https://ko-fi.com/example. " + suffix
		if got := filterSponsorshipSentences(text); got != suffix {
			t.Errorf("filterSponsorshipSentences(%q) = %q; want %q", text, got, suffix)
		}
	}
	for _, punctuation := range []string{"!", "?"} {
		text := "Sponsor https://ko-fi.com/example" + punctuation + " iOS users can ask."
		if got := filterSponsorshipSentences(text); got != "iOS users can ask." {
			t.Errorf("filterSponsorshipSentences(%q) = %q; want %q", text, got, "iOS users can ask.")
		}
	}
	if got := filterSponsorshipSentences("Sponsor https://ko-fi.com/example."); got != "" {
		t.Errorf("sponsorship at end = %q", got)
	}
}

func TestFilterSponsorshipSentences_Boundary_Abbreviations(t *testing.T) {
	for _, tc := range []struct{ word, want string }{
		{"i.e.", ""}, {"U.S.", ""}, {"(e.g.", ""},
		{"a.", "keep"}, {"J.", "keep"}, {"example.", "keep"},
	} {
		text := "Sponsor https://ko-fi.com/example " + tc.word + " keep"
		if got := filterSponsorshipSentences(text); got != tc.want {
			t.Errorf("filterSponsorshipSentences(%q) = %q; want %q", text, got, tc.want)
		}
	}
}

func TestFilterSponsorship_Boundary_IndentedList(t *testing.T) {
	text := "Keep.\n\n    - first\n    - Sponsor https://ko-fi.com/example\n    - last"
	want := "Keep.\n\n    - first\n    - last"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}
