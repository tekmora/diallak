// SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadSecretFile_Positive_TrimsRegularFile(t *testing.T) {
	path := writeSecret(t, "regular", " \nexample-value\t")
	got, err := readSecretFile(path, EnvTokenFile, "fixture")
	if err != nil || got != "example-value" {
		t.Fatalf("readSecretFile did not return the trimmed fixture: %v", err)
	}
}

func TestReadSecretFile_Negative_RejectsFIFOWithoutWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create FIFO fixture: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := readSecretFile(path, EnvTokenFile, "fixture")
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil || err.Error() != "fixture file is not a regular file" {
			t.Fatalf("readSecretFile returned %v, want a non-regular file error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readSecretFile blocked on a FIFO without a writer for 5 seconds")
	}
}

func TestReadSecretFile_Negative_RejectsDirectory(t *testing.T) {
	_, err := readSecretFile(t.TempDir(), EnvTokenFile, "fixture")
	if err == nil || err.Error() != "fixture file is not a regular file" {
		t.Fatalf("readSecretFile returned %v, want a non-regular file error", err)
	}
}

func TestReadSecretFile_Negative_RejectsCharDevice(t *testing.T) {
	_, err := readSecretFile("/dev/null", EnvTokenFile, "fixture")
	if err == nil || err.Error() != "fixture file is not a regular file" {
		t.Fatalf("readSecretFile returned %v, want a non-regular file error", err)
	}
}

func TestReadSecretFile_Boundary_FollowsSymlinkToRegularFile(t *testing.T) {
	target := writeSecret(t, "regular", "example-value")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink fixture: %v", err)
	}
	got, err := readSecretFile(link, EnvTokenFile, "fixture")
	if err != nil || got != "example-value" {
		t.Fatalf("readSecretFile did not follow the symlink to the fixture: %v", err)
	}
}
