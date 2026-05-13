package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenStore_NewReadErrorPropagates(t *testing.T) {
	// If path points at a directory, ReadFile returns a non-IsNotExist error.
	dir := t.TempDir()
	if _, err := NewTokenStore(dir); err == nil {
		t.Errorf("expected error when path is a directory")
	}
}

func TestTokenStore_SetMkdirFails(t *testing.T) {
	// Construct directly: simulate having a valid store but an unwritable parent.
	// Parent of token is a regular file, so MkdirAll on its child path fails
	// with ENOTDIR. We bypass NewTokenStore (which would also reject this).
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	s := &TokenStore{path: filepath.Join(blocker, "sub", "token")}
	if err := s.Set("abc"); err == nil {
		t.Errorf("expected error when MkdirAll fails")
	}
}

func TestTokenStore_NewLoadsTrimmedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  trimmed-token\n"), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	s, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got := s.Get(); got != "trimmed-token" {
		t.Errorf("expected trimmed, got %q", got)
	}
}

func TestFilePresent_OnDirectoryReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	if FilePresent(dir) {
		t.Errorf("dir should not count as present")
	}
}
