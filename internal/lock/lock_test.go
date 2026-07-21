package lock

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, acquired, err := Try(context.Background(), path)
	if err != nil {
		t.Fatalf("Try() error: %v", err)
	}
	if !acquired {
		t.Fatal("Try() acquired = false; want true")
	}
	if lock == nil {
		t.Fatal("Try() lock = nil")
	}
	if err := lock.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestTry_AlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("first Try() failed: %v", err)
	}
	defer first.Close()

	_, acquired, err = Try(context.Background(), path)
	if err != nil {
		t.Fatalf("second Try() error: %v", err)
	}
	if acquired {
		t.Error("second Try() should fail to acquire already-held lock")
	}
}

func TestTry_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, acquired, err := Try(ctx, filepath.Join(t.TempDir(), "test.lock"))
	if err == nil {
		t.Error("Try() with canceled context should return error")
	}
	if acquired {
		t.Error("Try() should not acquire lock with canceled context")
	}
}

func TestWait_Timeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("Try() failed: %v", err)
	}
	defer first.Close()

	_, err = Wait(context.Background(), path, 100*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Error("Wait() should timeout when lock is held")
	}
}

func TestWait_Acquires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, err := Wait(context.Background(), path, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestWait_InvalidParams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	if _, err := Wait(context.Background(), path, 0, time.Second); err == nil {
		t.Error("Wait() with zero timeout should fail")
	}
	if _, err := Wait(context.Background(), path, time.Second, 0); err == nil {
		t.Error("Wait() with zero poll interval should fail")
	}
}

func TestWait_ContextCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("Try() failed: %v", err)
	}
	defer first.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Wait(ctx, path, time.Second, 10*time.Millisecond)
	if err == nil {
		t.Error("Wait() with canceled context should return error")
	}
}

func TestClose_Nil(t *testing.T) {
	var l *Lock
	if err := l.Close(); err != nil {
		t.Errorf("Close() on nil lock: %v", err)
	}
}