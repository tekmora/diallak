package retrieval

import (
	"reflect"
	"testing"
)

func TestTokenize_Positive_SplitsAndStems(t *testing.T) {
	text := "How many searches per hour does SnatchArr do by default?"
	got := Tokenize(text)
	want := []string{"many", "search", "per", "hour", "snatcharr", "default"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize(%q) = %v, want %v", text, got, want)
	}
}

func TestTokenize_Positive_SuffixStemmerTable(t *testing.T) {
	tests := []struct {
		word string
		want string
	}{
		{"categories", "category"},
		{"entries", "entry"},
		{"queries", "query"},
		{"searches", "search"},
		{"snatches", "snatch"},
		{"crashes", "crash"},
		{"processes", "process"},
		{"indexes", "index"},
		{"snatching", "snatch"},
		{"waiting", "wait"},
		{"downloading", "download"},
		{"supported", "support"},
		{"clients", "client"},
		{"downloads", "download"},
		{"secrets", "secret"},
		{"instances", "instance"},
		{"releases", "release"},
		{"status", "status"},
		{"apps", "apps"},
		{"pods", "pods"},
		{"runs", "runs"},
		{"cross", "cross"},
	}
	for _, tc := range tests {
		got := stemWord(tc.word)
		if got != tc.want {
			t.Errorf("stemWord(%q) = %q, want %q", tc.word, got, tc.want)
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
