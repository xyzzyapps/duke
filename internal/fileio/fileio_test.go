package fileio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// tempDir creates a scratch directory inside the package directory (never
// outside the working tree) and removes it after the test.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "tmp-store-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "note.txt")
	s := OSStore{}
	if err := s.Write(path, "alpha\nbeta\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := s.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "alpha\nbeta\n" {
		t.Fatalf("got %q", got)
	}
}

func TestReadMissingFileReportsNotExist(t *testing.T) {
	s := OSStore{}
	_, err := s.Read(filepath.Join(tempDir(t), "nope.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}
