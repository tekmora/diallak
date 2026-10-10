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

// TestStem_RuleCoverageTable holds one or two words for every rule of the stemmer. The words were
// chosen by a mutation run: each rule was changed on its own and words whose stem then differed
// from the official output were kept. Every pair is a line of the official vocabulary, so the table
// is correct by that reference. Without it a broken rule passes CI, where
// TestStem_OfficialVocabulary is skipped.
func TestStem_RuleCoverageTable(t *testing.T) {
	official := "abase>abas abated>abat abatement>abat abates>abat abbey>abbei abed>ab ability>abil " +
		"able>abl abominable>abomin abortive>abort abortives>abort abrogate>abrog " +
		"abundance>abund abundant>abund abus>abu abuser>abus aby>abi accidence>accid " +
		"accident>accid aching>ach acting>act activity>activ adamant>adam added>ad adding>ad " +
		"addition>addit adjacent>adjac admirable>admir admiral>admir adoption>adopt " +
		"adoptious>adopti adoration>ador adorer>ador adornment>adorn adriatic>adriat " +
		"adulation>adul affability>affabl affinity>affin agreed>agre allayment>allay allies>alli " +
		"allowance>allow amazement>amaz amiss>amiss amorous>amor amorously>amor animal>anim " +
		"apology>apolog appall>appal aptness>apt aries>ari ass>ass asses>ass awful>aw " +
		"badness>bad baked>bake barbarism>barbar bared>bare barely>bare barful>bar basely>base " +
		"bashfulness>bash bated>bate befall>befal canonize>canon capability>capabl " +
		"catechism>catech choleric>choler coherence>coher comical>comic covetously>covet " +
		"dedicate>dedic delicate>delic dumbly>dumbl egally>egal equality>equal equally>equal " +
		"esses>ess exceed>exce famously>famous felicity>felic finical>finic focative>foc " +
		"forgiveness>forgiv hideousness>hideous humbly>humbl inaudible>inaud infancy>infanc " +
		"intenible>inten memorize>memor morality>moral moralize>moral nativity>nativ " +
		"negative>neg orator>orat orators>orat potency>potenc potently>potent pudency>pudenc " +
		"royalize>royal silently>silent simplicity>simplic tediousness>tedious temporizer>tempor " +
		"traditional>tradit vacancy>vacanc wilfulness>wil"
	// Three rules of step 2 are not exercised by any word of the official vocabulary. These stems
	// follow from the algorithm definition: vietnamization -> vietnamize -> vietnam,
	// relational -> relate -> relat, feudalism -> feudal.
	derived := "vietnamization>vietnam relational>relat feudalism>feudal"

	pairs := append(strings.Fields(official), strings.Fields(derived)...)
	if len(pairs) != 110 {
		t.Fatalf("expected 110 pairs, got %d", len(pairs))
	}
	for _, pair := range pairs {
		word, want, ok := strings.Cut(pair, ">")
		if !ok {
			t.Fatalf("malformed pair: %q", pair)
		}
		if got := stem(word); got != want {
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
