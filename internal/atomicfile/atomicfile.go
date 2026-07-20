package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces path with data only after the complete contents have been
// written and flushed to a temporary file in the same directory.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	tmp, err := writeTemp(dir, data, perm)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

// WriteNew creates path without replacing a file created by another process.
// Linking a fully written temporary file makes the new path visible atomically.
func WriteNew(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	tmp, err := writeTemp(dir, data, perm)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	return nil
}

func writeTemp(dir string, data []byte, perm os.FileMode) (*os.File, error) {
	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return nil, fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return nil, fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("flush temporary file: %w", err)
	}
	ok = true
	return tmp, nil
}
