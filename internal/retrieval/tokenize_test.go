package retrieval

import (
	"reflect"
	"testing"
)

func TestTokenize_Positive_SplitsAndStems(t *testing.T) {
	text := "How many searches per hour does SnatchArr do by default?"
	got := Tokenize(text)
	want := []string{"mani", "search", "per", "hour", "snatcharr", "default"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize(%q) = %v, want %v", text, got, want)
	}
}

func TestTokenize_Positive_AppliesStemmerToTokens(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"categories entries queries", []string{"categori", "entri", "queri"}},
		{"searches snatches crashes", []string{"search", "snatch", "crash"}},
		{"supported clients downloading", []string{"support", "client", "download"}},
		{"releases instances secrets", []string{"releas", "instanc", "secret"}},
	}
	for _, tc := range tests {
		got := Tokenize(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Tokenize(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestTokenize_Boundary_ShortTokensAndSymbols(t *testing.T) {
	// 1-character tokens dropped
	if got := Tokenize("a b c d 1 2"); len(got) != 0 {
		t.Errorf("Tokenize single runes = %v, want empty", got)
	}

	// pure punctuation
	if got := Tokenize("... --- !!! ???"); len(got) != 0 {
		t.Errorf("Tokenize punctuation = %v, want empty", got)
	}

	// 2-character tokens kept if not stopwords
	got := Tokenize("go v2 ok")
	want := []string{"go", "v2", "ok"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize 2-rune tokens = %v, want %v", got, want)
	}
}
