package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func imageMainPackage(argDefault string, mains []string) (string, error) {
	if argDefault != "" {
		for _, main := range mains {
			if main == argDefault {
				return main, nil
			}
		}
		return "", fmt.Errorf("MAIN_PACKAGE %q is not a main package", argDefault)
	}
	if len(mains) != 1 {
		return "", fmt.Errorf("MAIN_PACKAGE is unset; found %d main packages: %v", len(mains), mains)
	}
	return mains[0], nil
}

func moduleMainDirs(root string) ([]string, error) {
	dirs := make(map[string]bool)
	files := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", path, walkErr)
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		if files > 50000 {
			return fmt.Errorf("walk %s: exceeds 50000 files", root)
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if file.Name.Name == "main" {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return fmt.Errorf("relative path for %s: %w", path, err)
			}
			dirs["./"+filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	mains := make([]string, 0, len(dirs))
	for dir := range dirs {
		mains = append(mains, dir)
	}
	sort.Strings(mains)
	return mains, nil
}

// An empty image build default becomes ambiguous when the module gains another main package.
// Check the actual Dockerfile default against the module before image jobs run.
func TestDockerfile_Positive_MainPackageDefaultIsAMainPackage(t *testing.T) {
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	arg := regexp.MustCompile(`(?m)^ARG MAIN_PACKAGE=(\S*)$`).FindSubmatch(dockerfile)
	if arg == nil {
		t.Fatalf("Dockerfile has no MAIN_PACKAGE default")
	}
	mains, err := moduleMainDirs("../..")
	if err != nil {
		t.Fatalf("find main packages: %v", err)
	}
	main, err := imageMainPackage(string(arg[1]), mains)
	if err != nil {
		t.Fatalf("select image main package: %v", err)
	}
	if main != "./cmd/diallak" {
		t.Fatalf("image main package = %q, want ./cmd/diallak", main)
	}
}

func TestImageMainPackage_Negative_EmptyDefaultWithTwoMains(t *testing.T) {
	if _, err := imageMainPackage("", []string{"./cmd/diallak", "./tools/secrets/gate"}); err == nil {
		t.Fatalf("empty default with two main packages must fail")
	}
}

func TestImageMainPackage_Negative_DefaultNotAMain(t *testing.T) {
	if _, err := imageMainPackage("./cmd/missing", []string{"./cmd/diallak"}); err == nil {
		t.Fatalf("default that is not a main package must fail")
	}
}

func TestImageMainPackage_Boundary_OneOrNoMain(t *testing.T) {
	main, err := imageMainPackage("", []string{"./cmd/diallak"})
	if err != nil || main != "./cmd/diallak" {
		t.Fatalf("one main package: got %q, %v; want ./cmd/diallak, nil", main, err)
	}
	if _, err := imageMainPackage("", nil); err == nil {
		t.Fatalf("empty default with no main packages must fail")
	}
}
