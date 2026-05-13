// Package auth holds the worker's token store and the inference-token Fiber
// middleware.
package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TokenStore persists a single worker-JWT to disk and serves it from memory
// for in-process callers. Safe for concurrent use.
type TokenStore struct {
	path string
	mu   sync.RWMutex
	tok  string
}

// NewTokenStore opens the store at path. If the file exists and is non-empty,
// its contents are loaded as the current token. A missing file is not an
// error — the worker is expected to bootstrap one.
func NewTokenStore(path string) (*TokenStore, error) {
	s := &TokenStore{path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		s.tok = strings.TrimSpace(string(b))
		return s, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	return nil, err
}

// Get returns the current token; empty string if not yet bootstrapped.
func (s *TokenStore) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tok
}

// Set persists the supplied token to disk (mode 0o600) and updates the
// in-memory value. Parent directories are created with 0o700 as needed.
func (s *TokenStore) Set(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, []byte(token), 0o600); err != nil {
		return err
	}
	s.tok = token
	return nil
}

// Clear deletes the on-disk token and resets the in-memory value to empty.
// Returns nil if the file was already absent.
func (s *TokenStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tok = ""
	err := os.Remove(s.path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// FilePresent reports whether the file at path exists and is non-empty.
// Worker config consults this to decide whether BOOTSTRAP_TOKEN is required.
func FilePresent(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

// writeFileAtomic writes via a tmp file + rename so a crash during Set leaves
// the previous token intact rather than truncating to empty.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once rename succeeds
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
