package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveReplacesStateAndLoadInitializesMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := &Store{Notifications: map[string]time.Time{"old": time.Unix(10, 0)}}
	if err := first.Save(path); err != nil {
		t.Fatal(err)
	}
	second := &Store{Notifications: map[string]time.Time{"new": time.Unix(20, 0)}}
	if err := second.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := loaded.Notifications["old"]; exists || loaded.Notifications["new"].Unix() != 20 {
		t.Fatalf("unexpected saved state: %#v", loaded.Notifications)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("state file was not written: info=%v err=%v", info, err)
	}
}

func TestRecentRejectsFutureAndExpiredTimestamps(t *testing.T) {
	now := time.Unix(100, 0)
	store := &Store{Notifications: map[string]time.Time{
		"recent": now.Add(-time.Minute),
		"future": now.Add(time.Minute),
		"old":    now.Add(-2 * time.Hour),
	}}
	if !store.Recent("recent", now, time.Hour) || store.Recent("future", now, time.Hour) || store.Recent("old", now, time.Hour) {
		t.Fatal("cooldown classification is incorrect")
	}
}

func TestLoadFailuresAndEmptyState(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	store, err := Load(missing)
	if err != nil || store.Notifications == nil {
		t.Fatalf("missing state: store=%#v err=%v", store, err)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(invalid); err == nil {
		t.Fatal("invalid JSON should fail")
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("reading a directory as state should fail")
	}

	nullMap := filepath.Join(t.TempDir(), "null.json")
	if err := os.WriteFile(nullMap, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err = Load(nullMap)
	if err != nil || store.Notifications == nil {
		t.Fatalf("nil map was not initialized: %#v, %v", store, err)
	}
}

func TestRecordInitializesMapAndRecentBoundaries(t *testing.T) {
	now := time.Unix(100, 123)
	store := &Store{}
	store.Record("key", now)
	if got := store.Notifications["key"]; !got.Equal(now.UTC()) {
		t.Fatalf("recorded time=%s", got)
	}
	if store.Recent("missing", now, time.Hour) {
		t.Fatal("missing key was recent")
	}
	if !store.Recent("key", now.Add(time.Hour-time.Nanosecond), time.Hour) {
		t.Fatal("timestamp inside cooldown was not recent")
	}
	if store.Recent("key", now.Add(time.Hour), time.Hour) {
		t.Fatal("cooldown endpoint should be expired")
	}
}

func TestSaveFilesystemFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&Store{}).Save(filepath.Join(blocker, "state.json")); err == nil {
		t.Fatal("save below a regular file should fail")
	}
}
