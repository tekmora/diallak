package corpus

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Match tests whether a forward-slash path name matches pattern.
// If pattern contains no "**", it evaluates path.Match.
// If pattern contains exactly one "/**/", it matches any file under <prefix>/
// at any depth whose base name matches <glob>.
// Any other use of "**", empty patterns, leading slashes, or ".." path elements
// return an error.
func Match(pattern, name string) (bool, error) {
	if pattern == "" {
		return false, errors.New("pattern is empty")
	}
	if strings.HasPrefix(pattern, "/") || hasDotDotElement(pattern) {
		return false, fmt.Errorf("pattern %q contains leading slash or .. element", pattern)
	}
	if strings.HasPrefix(name, "/") || hasDotDotElement(name) {
		return false, fmt.Errorf("name %q contains leading slash or .. element", name)
	}

	numStars := strings.Count(pattern, "**")
	if numStars == 0 {
		matched, err := path.Match(pattern, name)
		if err != nil {
			return false, fmt.Errorf("match pattern %q against %q: %w", pattern, name, err)
		}
		return matched, nil
	}

	if numStars != 1 || strings.Count(pattern, "/**/") != 1 {
		return false, fmt.Errorf("pattern %q has unsupported ** use", pattern)
	}

	idx := strings.Index(pattern, "/**/")
	prefix := pattern[:idx]
	glob := pattern[idx+4:]

	prefixWithSlash := prefix + "/"
	if !strings.HasPrefix(name, prefixWithSlash) {
		return false, nil
	}

	matched, err := path.Match(glob, path.Base(name))
	if err != nil {
		return false, fmt.Errorf("match glob %q against %q: %w", glob, path.Base(name), err)
	}
	return matched, nil
}

func hasDotDotElement(s string) bool {
	rem := s
	for len(rem) > 0 {
		var seg string
		idx := strings.IndexByte(rem, '/')
		if idx == -1 {
			seg = rem
			rem = ""
		} else {
			seg = rem[:idx]
			rem = rem[idx+1:]
		}
		if seg == ".." {
			return true
		}
	}
	return false
}
