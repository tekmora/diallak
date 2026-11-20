package ruleanswer

import (
	"strings"
	"testing"
)

func TestLoad_Positive_StrictValidTable(t *testing.T) {
	raw := `{"version":1,"triggers":[]}`
	table, err := Load([]byte(raw))
	if err != nil || table.Version != 1 {
		t.Fatalf("Load = (%v, %v), want version 1", table, err)
	}
}

func TestLoad_Negative_UnknownFieldAndTrailingData(t *testing.T) {
	for _, raw := range []string{`{"version":1,"triggers":[],"extra":true}`, `{"version":1,"triggers":[]} {}`} {
		if _, err := Load([]byte(raw)); err == nil {
			t.Errorf("Load(%s) error = nil", raw)
		}
	}
}

func TestLoad_Boundary_WhitespaceAfterDocument(t *testing.T) {
	if _, err := Load([]byte("{\"version\":1,\"triggers\":[]}  \n")); err != nil {
		t.Fatalf("Load whitespace suffix: %v", err)
	}
}

func TestLoad_Negative_DuplicateObjectKeys(t *testing.T) {
	for _, tc := range []struct{ raw, key string }{
		{`{"version":1,"version":1,"triggers":[]}`, "version"},
		{`{"version":1,"triggers":[{"id":"a","id":"b"}]}`, "id"},
		{`{"version":1,"triggers":[{"keywords":{"all":[],"all":[]}}]}`, "all"},
	} {
		_, err := Load([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("duplicate %s: %v", tc.key, err)
		}
	}
	if _, err := Load([]byte(`{"version":1,"triggers":[{"id":"a"},{"id":"b"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Negative_NonCanonicalObjectKeys(t *testing.T) {
	for _, tc := range []struct{ raw, key string }{
		{`{"version":2,"VERSION":1,"triggers":[]}`, "VERSION"},
		{`{"Version":1,"triggers":[]}`, "Version"},
		{`{"version":1,"triggers":[{"Keywords":{}}]}`, "Keywords"},
		// encoding/json folds the Kelvin sign to k when matching field names.
		{"{\"version\":1,\"triggers\":[{\"\u212aeywords\":{}}]}", "\u212aeywords"},
	} {
		_, err := Load([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("key %q: %v", tc.key, err)
		}
	}
}

func TestCheckJSONKeys_Boundary_Depth(t *testing.T) {
	for _, depth := range []int{maxJSONDepth, maxJSONDepth + 1} {
		raw := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		err := checkJSONKeys([]byte(raw))
		if (err != nil) != (depth > maxJSONDepth) {
			t.Errorf("depth %d: %v", depth, err)
		}
	}
}
