package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/atomicfile"
)

type File struct {
	Notifications map[string]time.Time `json:"notifications,omitempty"`
}

func Load(path string) (*File, error) {
	file := &File{Notifications: map[string]time.Time{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, file); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if file.Notifications == nil {
		file.Notifications = map[string]time.Time{}
	}
	return file, nil
}

func (f *File) Recent(key string, now time.Time, cooldown time.Duration) bool {
	last, ok := f.Notifications[key]
	if !ok {
		return false
	}
	age := now.Sub(last)
	return age >= 0 && age < cooldown
}

func (f *File) Record(key string, now time.Time) {
	if f.Notifications == nil {
		f.Notifications = map[string]time.Time{}
	}
	f.Notifications[key] = now.UTC()
}

func (f *File) Save(path string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
