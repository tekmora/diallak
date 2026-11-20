package ruleanswer

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/corpus"
)

var sponsorshipLink = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:github\.com/sponsors/|ko-fi\.com/|opencollective\.com/|liberapay\.com/|patreon\.com/|buymeacoffee\.com/)`)
var listItem = regexp.MustCompile(`^[ \t]*(?:[-+*] |[0-9]+[.)] )`)

func filterSponsorship(text string) string {
	if !sponsorshipLink.MatchString(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	return joinWithoutSponsorship(lines, sponsorshipLines(lines))
}

// sponsorshipLines modifies lines owned by filterSponsorship. For a multi-line
// non-list paragraph, it keeps filtered text in the first line and marks the
// other lines as removed. Indented code retains indented list-like lines;
// paragraphs starting with a list item still split at list items.
func sponsorshipLines(lines []string) []bool {
	removed := make([]bool, len(lines))
	start := 0

	for i, line := range lines {
		if strings.TrimSpace(line) == "" || (listItem.MatchString(line) && !codeParagraphContinues(lines, start, i)) {
			filterSponsorshipParagraph(lines, removed, start, i)
			start = i
			if strings.TrimSpace(line) == "" {
				start = i + 1
			}
		}
	}
	filterSponsorshipParagraph(lines, removed, start, len(lines))
	removeSponsorshipSubtrees(lines, removed)
	return removed
}

func codeParagraphContinues(lines []string, start, next int) bool {
	return start < next && corpus.IndentColumn(lines[start]) >= 4 &&
		!listItem.MatchString(lines[start]) && corpus.IndentColumn(lines[next]) >= 4
}

// filterSponsorshipParagraph modifies lines owned by filterSponsorship. For a
// multi-line non-list paragraph, it keeps filtered text in the first line and
// marks the other lines as removed. Indented code with a sponsorship link is
// removed whole without changing its text.
func filterSponsorshipParagraph(lines []string, removed []bool, start, end int) {
	if start == end {
		return
	}
	paragraph := strings.Join(lines[start:end], "\n")
	if !sponsorshipLink.MatchString(paragraph) {
		return
	}
	if corpus.IndentColumn(lines[start]) >= 4 {
		for i := start; i < end; i++ {
			removed[i] = true
		}
		return
	}
	if !listItem.MatchString(lines[start]) {
		lines[start] = filterSponsorshipSentences(paragraph)
	}
	removed[start] = listItem.MatchString(lines[start]) || strings.TrimSpace(lines[start]) == ""
	for i := start + 1; i < end; i++ {
		removed[i] = true
	}
}

// filterSponsorshipSentences treats a line without end punctuation as part of
// the sentence continuing on the next line, removing it with a sponsorship sentence.
// Inside a paragraph, letter-dot abbreviations with at least two pairs do not
// end a sentence.
func filterSponsorshipSentences(paragraph string) string {
	var kept strings.Builder
	start := 0
	for i, r := range paragraph {
		if !sponsorshipSentenceBoundary(paragraph, start, i, r) {
			continue
		}
		next := i + 1
		sentence := paragraph[start:next]
		if !sponsorshipLink.MatchString(sentence) {
			kept.WriteString(sentence)
		}
		start = next
	}
	if tail := paragraph[start:]; !sponsorshipLink.MatchString(tail) {
		kept.WriteString(tail)
	}
	return strings.TrimSpace(kept.String())
}

func sponsorshipSentenceBoundary(paragraph string, start, i int, r rune) bool {
	if r != '.' && r != '!' && r != '?' {
		return false
	}
	next := i + 1
	if next == len(paragraph) {
		return true
	}
	following, _ := utf8.DecodeRuneInString(paragraph[next:])
	return unicode.IsSpace(following) && (r != '.' || !letterDotAbbreviation(paragraph[start:next]))
}

func letterDotAbbreviation(sentence string) bool {
	start := len(sentence)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(sentence[:start])
		if unicode.IsSpace(r) {
			break
		}
		start -= size
	}
	word := strings.TrimLeftFunc(sentence[start:], func(r rune) bool {
		return r == '(' || r == '[' || unicode.Is(unicode.Quotation_Mark, r)
	})
	pairs := 0
	expectLetter := true
	for _, r := range word {
		if expectLetter {
			if !unicode.IsLetter(r) {
				return false
			}
		} else {
			if r != '.' {
				return false
			}
			pairs++
		}
		expectLetter = !expectLetter
	}
	return expectLetter && pairs >= 2
}

func removeSponsorshipSubtrees(lines []string, removed []bool) {
	active := false
	indent := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		column := corpus.IndentColumn(line)
		if active && column > indent {
			removed[i] = true
			continue
		}
		active = removed[i] && listItem.MatchString(line)
		if active {
			indent = column
		}
	}
}

func joinWithoutSponsorship(lines []string, removed []bool) string {
	var kept, blanks []string
	removedBlock := false
	for i, line := range lines {
		if removed[i] {
			removedBlock = true
			continue
		}
		if strings.TrimSpace(line) == "" {
			blanks = append(blanks, line)
			continue
		}
		if len(kept) > 0 {
			if removedBlock && len(blanks) > 1 {
				blanks = blanks[:1]
			}
			kept = append(kept, blanks...)
		}
		kept = append(kept, line)
		blanks = nil
		removedBlock = false
	}
	return strings.Join(kept, "\n")
}
