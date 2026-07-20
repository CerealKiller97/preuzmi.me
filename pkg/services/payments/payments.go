// Package payments tracks which receipts have been paid.
//
// The state lives in receipts/payments.json, deliberately separate from
// receipts/meta.json: meta.json is curated by hand, while this file is written
// by the application. Keeping them apart means a write here can never clobber
// hand-edited metadata, and a receipt can be marked paid even when it has no
// metadata entry at all.
package payments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const fileName = "payments.json"

// Store is a concurrency-safe, file-backed set of paid receipts.
type Store struct {
	mu   sync.RWMutex
	path string
	// paid maps a receipt key to the moment it was marked paid.
	paid map[string]int64
}

// Key builds the identifier for a receipt. The period is expected to already be
// normalized by the caller, so that "010-2025" and "10-2025" cannot produce two
// different entries for the same receipt.
func Key(period, provider string) string {
	return strings.ToLower(period + "|" + provider)
}

// New loads the store from dir, creating an empty one when the file is absent.
func New(dir string) (*Store, error) {
	s := &Store{
		path: filepath.Join(dir, fileName),
		paid: map[string]int64{},
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing marked paid yet.
			return s, nil
		}

		return nil, err
	}

	// An empty file is a valid starting point, json.Unmarshal is not happy with it.
	if len(strings.TrimSpace(string(b))) == 0 {
		return s, nil
	}

	if err := json.Unmarshal(b, &s.paid); err != nil {
		return nil, err
	}

	return s, nil
}

// PaidAt reports when a receipt was marked paid.
func (s *Store) PaidAt(key string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	at, ok := s.paid[key]

	return at, ok
}

// Set marks a receipt paid or unpaid and persists the change.
func (s *Store) Set(key string, paid bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var at int64
	if paid {
		at = time.Now().Unix()
		s.paid[key] = at
	} else {
		delete(s.paid, key)
	}

	if err := s.persist(); err != nil {
		return 0, err
	}

	return at, nil
}

// persist writes the whole map out. The caller must hold the write lock.
//
// The write goes to a temporary file that is then renamed over the target, so
// an interrupted write can never leave a half-written file behind.
func (s *Store) persist() error {
	data, err := json.MarshalIndent(s.paid, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, fileName+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), s.path)
}
