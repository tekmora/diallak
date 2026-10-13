package eval

import (
	"bytes"
	"testing"
	"testing/fstest"
)

func TestLoadSet_Boundary_ExactOneMebibyteAllowed(t *testing.T) {
	base := sampleSetJSON()
	total := 1 << 20
	if len(base) > total {
		t.Fatalf("sampleSetJSON exceeds %d bytes", total)
	}
	padding := bytes.Repeat([]byte(" "), total-len(base))
	data := append([]byte(base), padding...)
	if len(data) != total {
		t.Fatalf("data length = %d, want %d", len(data), total)
	}

	fsys := fstest.MapFS{
		"cases.json": &fstest.MapFile{Data: data},
	}
	set, err := LoadSet(fsys, "cases.json")
	if err != nil {
		t.Fatalf("LoadSet with exactly 1 MiB failed: %v", err)
	}
	if set.Version != 1 {
		t.Fatalf("set.Version = %d, want 1", set.Version)
	}
}
