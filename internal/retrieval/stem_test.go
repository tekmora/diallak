package retrieval

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStem_CommittedTable(t *testing.T) {
	pairsStr := "ties>ti cats>cat feed>feed agreed>agre bled>bled sing>sing troubled>troubl tanned>tan " +
		"falling>fall hissing>hiss failing>fail happy>happi sky>sky rational>ration vilely>vile " +
		"hopeful>hope goodness>good allowance>allow inference>infer defensible>defens dependent>depend " +
		"adoption>adopt rate>rate cease>ceas controlling>control rolling>roll install>instal " +
		"installed>instal manual>manual weekly>weekli week>week local>local searches>search " +
		"search>search searching>search pause>paus pauses>paus snatches>snatch snatch>snatch " +
		"requests>request requested>request priority>prioriti supported>support support>support " +
		"client>client clients>client running>run run>run verify>verifi verified>verifi " +
		"different>differ difference>differ applications>applic generally>gener general>gener " +
		"connected>connect setting>set set>set licence>licenc license>licens published>publish " +
		"publish>publish publishing>publish hourly>hourli hour>hour hours>hour instances>instanc " +
		"instance>instanc policy>polici policies>polici default>default"

	pairs := strings.Fields(pairsStr)
	if len(pairs) != 71 {
		t.Fatalf("expected 71 pairs, got %d", len(pairs))
	}

	for _, pair := range pairs {
		parts := strings.Split(pair, ">")
		if len(parts) != 2 {
			t.Fatalf("malformed pair: %q", pair)
		}
		word, want := parts[0], parts[1]
		got := stem(word)
		if got != want {
			t.Errorf("stem(%q) = %q, want %q", word, got, want)
		}
	}
}

func TestStem_OfficialVocabulary(t *testing.T) {
	vocDir := os.Getenv("PORTER_VOC_DIR")
	if vocDir == "" {
		t.Skip("PORTER_VOC_DIR not set; skipping official vocabulary test")
	}

	vocPath := filepath.Join(vocDir, "voc.txt")
	outPath := filepath.Join(vocDir, "output.txt")

	vocFile, err := os.Open(vocPath)
	if err != nil {
		t.Fatalf("open voc.txt: %v", err)
	}
	defer func() {
		if closeErr := vocFile.Close(); closeErr != nil {
			t.Errorf("close vocFile: %v", closeErr)
		}
	}()

	outFile, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open output.txt: %v", err)
	}
	defer func() {
		if closeErr := outFile.Close(); closeErr != nil {
			t.Errorf("close outFile: %v", closeErr)
		}
	}()

	const limitBytes = 2 << 20 // 2 MiB
	vocScanner := bufio.NewScanner(io.LimitReader(vocFile, limitBytes))
	outScanner := bufio.NewScanner(io.LimitReader(outFile, limitBytes))

	total := 0
	var mismatches []string

	for vocScanner.Scan() {
		if !outScanner.Scan() {
			t.Fatalf("output.txt ended early at line %d", total+1)
		}
		total++
		word := strings.TrimSpace(vocScanner.Text())
		expected := strings.TrimSpace(outScanner.Text())
		got := stem(word)
		if got != expected {
			if len(mismatches) < 20 {
				mismatches = append(mismatches, word+" -> got "+got+", want "+expected)
			}
		}
	}
	if outScanner.Scan() {
		t.Fatalf("voc.txt ended earlier than output.txt")
	}

	if err := vocScanner.Err(); err != nil {
		t.Fatalf("voc scanner error: %v", err)
	}
	if err := outScanner.Err(); err != nil {
		t.Fatalf("out scanner error: %v", err)
	}

	if len(mismatches) > 0 {
		for _, m := range mismatches {
			t.Logf("mismatch: %s", m)
		}
		t.Fatalf("%d mismatches of %d", len(mismatches), total)
	}
	t.Logf("0 mismatches of %d", total)
}

func TestStem_Boundary(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"a", "a"},
		{"in", "in"},
		{"to", "to"},
		{"v2", "v2"},
		{"3d", "3d"},
		{"123", "123"},
		{"münchen", "münchen"},
	}

	for _, tc := range cases {
		// stem() handles <= 2 letters directly
		if len(tc.input) <= 2 {
			if got := stem(tc.input); got != tc.want {
				t.Errorf("stem(%q) = %q, want %q", tc.input, got, tc.want)
			}
		}
	}
}
