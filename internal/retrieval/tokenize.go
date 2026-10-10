package retrieval

import (
	"slices"
	"unicode"
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

// Tokenize converts text to lowercase tokens, split on non-letters and non-digits,
// dropping tokens shorter than 2 runes, dropping function stopwords, and applying
// the Porter stemmer to ASCII alphabetic tokens longer than 2 characters.
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
	if len(word) > 2 && isASCIIAlpha(word) {
		return stem(word)
	}
	return word
}

func isASCIIAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

func isStopword(w string) bool {
	_, found := slices.BinarySearch(stopwords, w)
	return found
}
