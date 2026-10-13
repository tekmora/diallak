package retrieval

import (
	"reflect"
	"testing"
)

func TestTokenize_Negative_SplitsOnPunctuationMarks(t *testing.T) {
	input := "hello!world"
	got := Tokenize(input)
	want := []string{"hello", "world"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(%q) = %v, want %v", input, got, want)
	}
}
