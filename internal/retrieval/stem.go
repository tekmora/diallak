package retrieval

import "strings"

func isConsonant(s string, i int) bool {
	switch s[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if i == 0 {
			return true
		}
		yCount := 0
		for j := i - 1; j >= 0; j-- {
			if s[j] == 'y' {
				yCount++
				continue
			}
			prevCons := s[j] != 'a' && s[j] != 'e' && s[j] != 'i' && s[j] != 'o' && s[j] != 'u'
			if yCount%2 == 0 {
				return !prevCons
			}
			return prevCons
		}
		return i%2 == 0
	default:
		return true
	}
}

func measure(s string) int {
	n := len(s)
	if n == 0 {
		return 0
	}
	i := 0
	for i < n && isConsonant(s, i) {
		i++
	}
	m := 0
	for i < n {
		for i < n && !isConsonant(s, i) {
			i++
		}
		if i == n {
			break
		}
		for i < n && isConsonant(s, i) {
			i++
		}
		m++
	}
	return m
}

func containsVowel(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isConsonant(s, i) {
			return true
		}
	}
	return false
}

func endsDoubleConsonant(s string) bool {
	n := len(s)
	if n < 2 {
		return false
	}
	return s[n-1] == s[n-2] && isConsonant(s, n-1)
}

func endsCVC(s string) bool {
	n := len(s)
	if n < 3 {
		return false
	}
	if !isConsonant(s, n-3) || isConsonant(s, n-2) || !isConsonant(s, n-1) {
		return false
	}
	last := s[n-1]
	return last != 'w' && last != 'x' && last != 'y'
}

type stepRule struct {
	suffix      string
	replacement string
	cond        func(stem string) bool
}

func applyStepRules(w string, rules []stepRule) string {
	for _, r := range rules {
		if strings.HasSuffix(w, r.suffix) {
			stemPart := w[:len(w)-len(r.suffix)]
			if r.cond == nil || r.cond(stemPart) {
				return stemPart + r.replacement
			}
			return w
		}
	}
	return w
}

var step1aRules = []stepRule{
	{suffix: "sses", replacement: "ss"},
	{suffix: "ies", replacement: "i"},
	{suffix: "ss", replacement: "ss"},
	{suffix: "s", replacement: ""},
}

func step1a(w string) string {
	return applyStepRules(w, step1aRules)
}

func step1b(w string) string {
	if strings.HasSuffix(w, "eed") {
		stemPart := w[:len(w)-3]
		if measure(stemPart) > 0 {
			return stemPart + "ee"
		}
		return w
	}

	secondOrThird := false
	stemPart := ""
	if strings.HasSuffix(w, "ed") {
		candidate := w[:len(w)-2]
		if containsVowel(candidate) {
			stemPart = candidate
			secondOrThird = true
		}
	} else if strings.HasSuffix(w, "ing") {
		candidate := w[:len(w)-3]
		if containsVowel(candidate) {
			stemPart = candidate
			secondOrThird = true
		}
	}

	if !secondOrThird {
		return w
	}
	return step1bPost(stemPart)
}

func step1bPost(stemPart string) string {
	if strings.HasSuffix(stemPart, "at") || strings.HasSuffix(stemPart, "bl") || strings.HasSuffix(stemPart, "iz") {
		return stemPart + "e"
	}
	if endsDoubleConsonant(stemPart) {
		last := stemPart[len(stemPart)-1]
		if last != 'l' && last != 's' && last != 'z' {
			return stemPart[:len(stemPart)-1]
		}
		return stemPart
	}
	if measure(stemPart) == 1 && endsCVC(stemPart) {
		return stemPart + "e"
	}
	return stemPart
}

func step1c(w string) string {
	if strings.HasSuffix(w, "y") {
		stemPart := w[:len(w)-1]
		if containsVowel(stemPart) {
			return stemPart + "i"
		}
	}
	return w
}

func condM0(stemPart string) bool {
	return measure(stemPart) > 0
}

func condM1(stemPart string) bool {
	return measure(stemPart) > 1
}

var step2Rules = []stepRule{
	{suffix: "ization", replacement: "ize", cond: condM0},
	{suffix: "ational", replacement: "ate", cond: condM0},
	{suffix: "fulness", replacement: "ful", cond: condM0},
	{suffix: "ousness", replacement: "ous", cond: condM0},
	{suffix: "iveness", replacement: "ive", cond: condM0},
	{suffix: "biliti", replacement: "ble", cond: condM0},
	{suffix: "tional", replacement: "tion", cond: condM0},
	{suffix: "alism", replacement: "al", cond: condM0},
	{suffix: "ation", replacement: "ate", cond: condM0},
	{suffix: "aliti", replacement: "al", cond: condM0},
	{suffix: "iviti", replacement: "ive", cond: condM0},
	{suffix: "entli", replacement: "ent", cond: condM0},
	{suffix: "ousli", replacement: "ous", cond: condM0},
	{suffix: "ator", replacement: "ate", cond: condM0},
	{suffix: "enci", replacement: "ence", cond: condM0},
	{suffix: "anci", replacement: "ance", cond: condM0},
	{suffix: "izer", replacement: "ize", cond: condM0},
	{suffix: "bli", replacement: "ble", cond: condM0},
	{suffix: "alli", replacement: "al", cond: condM0},
	{suffix: "logi", replacement: "log", cond: condM0},
	{suffix: "eli", replacement: "e", cond: condM0},
}

var step3Rules = []stepRule{
	{suffix: "icate", replacement: "ic", cond: condM0},
	{suffix: "ative", replacement: "", cond: condM0},
	{suffix: "alize", replacement: "al", cond: condM0},
	{suffix: "iciti", replacement: "ic", cond: condM0},
	{suffix: "ical", replacement: "ic", cond: condM0},
	{suffix: "ful", replacement: "", cond: condM0},
	{suffix: "ness", replacement: "", cond: condM0},
}

var step4Rules = []stepRule{
	{suffix: "ement", replacement: "", cond: condM1},
	{suffix: "ance", replacement: "", cond: condM1},
	{suffix: "ence", replacement: "", cond: condM1},
	{suffix: "able", replacement: "", cond: condM1},
	{suffix: "ible", replacement: "", cond: condM1},
	{suffix: "ment", replacement: "", cond: condM1},
	{suffix: "ent", replacement: "", cond: condM1},
	{suffix: "ant", replacement: "", cond: condM1},
	{suffix: "ism", replacement: "", cond: condM1},
	{suffix: "ate", replacement: "", cond: condM1},
	{suffix: "iti", replacement: "", cond: condM1},
	{suffix: "ous", replacement: "", cond: condM1},
	{suffix: "ive", replacement: "", cond: condM1},
	{suffix: "ize", replacement: "", cond: condM1},
	{suffix: "ion", replacement: "", cond: func(s string) bool {
		return measure(s) > 1 && len(s) > 0 && (s[len(s)-1] == 's' || s[len(s)-1] == 't')
	}},
	{suffix: "al", replacement: "", cond: condM1},
	{suffix: "er", replacement: "", cond: condM1},
	{suffix: "ic", replacement: "", cond: condM1},
	{suffix: "ou", replacement: "", cond: condM1},
}

func step5a(w string) string {
	if strings.HasSuffix(w, "e") {
		stemPart := w[:len(w)-1]
		m := measure(stemPart)
		if m > 1 || (m == 1 && !endsCVC(stemPart)) {
			return stemPart
		}
	}
	return w
}

func step5b(w string) string {
	if measure(w) > 1 && endsDoubleConsonant(w) && strings.HasSuffix(w, "l") {
		return w[:len(w)-1]
	}
	return w
}

func stem(word string) string {
	if len(word) <= 2 {
		return word
	}
	w := step1a(word)
	w = step1b(w)
	w = step1c(w)
	w = applyStepRules(w, step2Rules)
	w = applyStepRules(w, step3Rules)
	w = applyStepRules(w, step4Rules)
	w = step5a(w)
	w = step5b(w)
	return w
}
