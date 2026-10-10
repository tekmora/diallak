package retrieval

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var stopwords = []string{
	"about", "also", "am", "an", "and", "are", "as", "at",
	"be", "because", "been", "being", "but", "by", "can", "could",
	"did", "do", "does", "for", "from", "had", "has", "have",
	"having", "he", "her", "him", "his", "how", "if", "in",
	"into", "is", "it", "its", "me", "my", "no", "not",
	"of", "on", "or", "our", "out", "over", "so", "some",
	"such", "than", "that", "the", "their", "them", "then", "there",
	"these", "they", "this", "those", "through", "to", "too", "under",
	"up", "was", "we", "were", "what", "when", "where", "which",
	"who", "why", "will", "with", "would", "you", "your",
}

type stemRule struct {
	suffix      string
	replacement string
	minStem     int
	cond        func(word, stemPart string) bool
}

var rules = []stemRule{
	{suffix: "ies", replacement: "y", minStem: 4, cond: nil},
	{suffix: "es", replacement: "", minStem: 4, cond: isESPrecededBySibilant},
	{suffix: "ing", replacement: "", minStem: 4, cond: nil},
	{suffix: "ed", replacement: "", minStem: 4, cond: nil},
	{suffix: "s", replacement: "", minStem: 4, cond: isNotSSorUSorIS},
}

func isESPrecededBySibilant(_, stemPart string) bool {
	return strings.HasSuffix(stemPart, "ch") || strings.HasSuffix(stemPart, "sh") ||
		strings.HasSuffix(stemPart, "ss") || strings.HasSuffix(stemPart, "x") ||
		strings.HasSuffix(stemPart, "z")
}

func isNotSSorUSorIS(word, _ string) bool {
	return !strings.HasSuffix(word, "ss") && !strings.HasSuffix(word, "us") && !strings.HasSuffix(word, "is")
}

// Tokenize converts text to lowercase tokens, split on non-letters/non-digits,
// dropping tokens shorter than 2 runes, dropping function stopwords, and applying
// a conservative suffix stemmer.
func Tokenize(text string) []string {
	var tokens []string
	var cur []rune
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, unicode.ToLower(r))
		} else if len(cur) > 0 {
			if tok := processToken(cur); tok != "" {
				tokens = append(tokens, tok)
			}
			cur = cur[:0]
		}
	}
	if len(cur) > 0 {
		if tok := processToken(cur); tok != "" {
			tokens = append(tokens, tok)
		}
	}
	return tokens
}

func processToken(runes []rune) string {
	if len(runes) < 2 {
		return ""
	}
	word := string(runes)
	if isStopword(word) {
		return ""
	}
	return stemWord(word)
}

func isStopword(w string) bool {
	_, found := slices.BinarySearch(stopwords, w)
	return found
}

func stemWord(w string) string {
	for _, r := range rules {
		if strings.HasSuffix(w, r.suffix) {
			stemPart := w[:len(w)-len(r.suffix)]
			if r.cond != nil && !r.cond(w, stemPart) {
				continue
			}
			if utf8.RuneCountInString(stemPart) >= r.minStem {
				return stemPart + r.replacement
			}
		}
	}
	return w
}
