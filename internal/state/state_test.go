package state

import (
	"testing"
	"time"
)

func TestRecent(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	store := &Store{Notifications: map[string]time.Time{}}

	// No entry → not recent.
	if store.Recent("key1", now, time.Hour) {
		t.Error("Recent() = true; want false (no entry)")
	}

	// Record and check.
	store.Record("key1", now)
	if !store.Recent("key1", now, time.Hour) {
		t.Error("Recent() = false; want true (just recorded)")
	}

	// Past cooldown → not recent.
	future := now.Add(2 * time.Hour)
	if store.Recent("key1", future, time.Hour) {
		t.Error("Recent() = true; want false (past cooldown)")
	}

	// Within cooldown.
	soon := now.Add(30 * time.Minute)
	if !store.Recent("key1", soon, time.Hour) {
		t.Error("Recent() = false; want true (within cooldown)")
	}
}

func TestRecord(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	store := &Store{Notifications: nil}

	// Record into nil map.
	store.Record("key1", now)
	if store.Notifications == nil {
		t.Fatal("Record() did not initialize Notifications map")
	}
	if !store.Notifications["key1"].Equal(now.UTC()) {
		t.Errorf("Record() = %v; want %v UTC", store.Notifications["key1"], now.UTC())
	}
}

func TestRecent_NegativeAge(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	store := &Store{Notifications: map[string]time.Time{}}
	store.Record("key1", now)
	// now is before the recorded time → negative age, should not be recent.
	past := now.Add(-1 * time.Hour)
	if store.Recent("key1", past, time.Hour) {
		t.Error("Recent() = true; want false (negative age)")
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state.json"

	store := &Store{Notifications: map[string]time.Time{}}
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	store.Record("key1", now)

	if err := store.Save(path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !loaded.Notifications["key1"].Equal(now.UTC()) {
		t.Errorf("Load() = %v; want %v", loaded.Notifications["key1"], now.UTC())
	}
}

func TestLoad_NotExist(t *testing.T) {
	store, err := Load("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if store.Notifications == nil {
		t.Error("Load() should return initialized map for missing file")
	}
}

func TestLoad_Fixture(t *testing.T) {
	store, err := Load("testdata/state.json")
	if err != nil {
		t.Fatalf("Load(fixture) error: %v", err)
	}
	tm := store.Notifications["key1"]
	if tm.IsZero() {
		t.Error("key1 not found in fixture")
	}
}