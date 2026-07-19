package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/atomicfile"
)

type Store struct {
	Notifications map[string]time.Time `json:"notifications,omitempty"`
}

func Load(path string) (*Store, error) {
	store := &Store{Notifications: map[string]time.Time{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if store.Notifications == nil {
		store.Notifications = map[string]time.Time{}
	}
	return store, nil
}

func (s *Store) Recent(key string, now time.Time, cooldown time.Duration) bool {
	last, ok := s.Notifications[key]
	if !ok {
		return false
	}
	age := now.Sub(last)
	return age >= 0 && age < cooldown
}

func (s *Store) Record(key string, now time.Time) {
	if s.Notifications == nil {
		s.Notifications = map[string]time.Time{}
	}
	s.Notifications[key] = now.UTC()
}

func (s *Store) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
