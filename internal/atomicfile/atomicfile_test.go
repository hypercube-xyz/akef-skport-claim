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
	data, err := os.ReadFile(path) // #nosec G304 -- path is inside t.TempDir.
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

func TestFilesystemFailures(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func(string) error{
		"replace": func(path string) error { return Write(path, []byte("x"), 0o600) },
		"new":     func(path string) error { return WriteNew(path, []byte("x"), 0o600) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := write(filepath.Join(blocker, "child")); err == nil {
				t.Fatal("write below a regular file should fail")
			}
		})
	}
	if _, err := writeTemp(filepath.Join(t.TempDir(), "missing"), []byte("x"), 0o600); err == nil {
		t.Fatal("temporary file in missing directory should fail")
	}
	targetDirectory := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(targetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDirectory, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(targetDirectory, []byte("replacement"), 0o600); err == nil {
		t.Fatal("regular file should not replace a non-empty directory")
	}
}
