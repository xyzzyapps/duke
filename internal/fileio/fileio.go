// Package fileio abstracts reading and writing the edited text file so the
// UI layer can be tested with an in-memory fake.
package fileio

import (
	"errors"
	"os"
)

// Store is the file persistence interface (mocked in UI tests).
type Store interface {
	// Read returns the file content. A missing file reports an error
	// satisfying errors.Is(err, os.ErrNotExist).
	Read(path string) (string, error)
	// Write stores content at path.
	Write(path string, content string) error
}

// OSStore reads and writes real files on disk.
type OSStore struct{}

// Read implements Store.
func (OSStore) Read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Write implements Store.
func (OSStore) Write(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// ErrNotExist re-exports the sentinel so callers do not import os just for
// existence checks.
var ErrNotExist = errors.New("file does not exist")
