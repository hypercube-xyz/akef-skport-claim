package lock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTryIsNonBlockingAndExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	defer first.Close()
	second, acquired, err := Try(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if acquired || second != nil {
		t.Fatal("second lock unexpectedly acquired")
	}
}

func TestWaitAcquiresAfterCurrentOwnerReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}

	released := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = first.Close()
		close(released)
	}()

	second, err := Wait(context.Background(), path, time.Second, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	<-released
}

func TestWaitReturnsTypedTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	defer first.Close()

	_, err = Wait(context.Background(), path, 20*time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected ErrWaitTimeout, got %v", err)
	}
}
