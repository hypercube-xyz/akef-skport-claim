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
