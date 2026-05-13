package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTokenStore_SetGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	s, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got := s.Get(); got != "" {
		t.Errorf("expected empty before set, got %q", got)
	}
	if err := s.Set("abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := s.Get(); got != "abc123" {
		t.Errorf("Get: got %q", got)
	}
}

func TestTokenStore_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	s, _ := NewTokenStore(path)
	if err := s.Set("persisted"); err != nil {
		t.Fatalf("%v", err)
	}
	s2, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got := s2.Get(); got != "persisted" {
		t.Errorf("expected persisted, got %q", got)
	}
}

func TestTokenStore_FileModeIsPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	s, _ := NewTokenStore(path)
	if err := s.Set("xx"); err != nil {
		t.Fatalf("%v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected 0o600, got %o", mode)
	}
}

func TestTokenStore_FilePresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if FilePresent(path) {
		t.Errorf("expected false for missing file")
	}
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	if !FilePresent(path) {
		t.Errorf("expected true for existing file")
	}
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	if FilePresent(path) {
		t.Errorf("empty file is not 'present' for auth purposes")
	}
}

func TestTokenStore_NewMissingFileIsOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "token") // subdir doesn't exist yet
	s, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("expected to tolerate missing file: %v", err)
	}
	if got := s.Get(); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestTokenStore_SetCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "subdir", "token")
	s, _ := NewTokenStore(path)
	if err := s.Set("created"); err != nil {
		t.Fatalf("Set should mkdir -p: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("token file not created: %v", err)
	}
}

func TestTokenStore_Clear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	s, _ := NewTokenStore(path)
	_ = s.Set("xx")
	if err := s.Clear(); err != nil {
		t.Fatalf("%v", err)
	}
	if got := s.Get(); got != "" {
		t.Errorf("expected empty after Clear, got %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, got err=%v", err)
	}
	// Clear on already-absent file is ok.
	if err := s.Clear(); err != nil {
		t.Errorf("Clear on absent file: %v", err)
	}
}

func TestTokenStore_Concurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	s, _ := NewTokenStore(path)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.Set("worker-1") }()
		go func() { defer wg.Done(); _ = s.Get() }()
	}
	wg.Wait()
	if got := s.Get(); got != "worker-1" {
		t.Errorf("got %q", got)
	}
}
