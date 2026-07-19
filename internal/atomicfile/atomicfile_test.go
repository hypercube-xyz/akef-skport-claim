package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNewAndReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	if err := WriteNew(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(path, []byte("unexpected"), 0o600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second WriteNew must refuse replacement: %v", err)
	}
	if err := Write(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("unexpected contents: %q", data)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".atomic-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v, %v", matches, err)
	}
}
