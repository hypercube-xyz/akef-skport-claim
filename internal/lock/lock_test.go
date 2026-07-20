package lock

import (
	"context"
	"errors"
	"os"
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
	defer func() {
		if err := first.Close(); err != nil {
			t.Errorf("release first lock: %v", err)
		}
	}()
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
	defer func() {
		if err := second.Close(); err != nil {
			t.Errorf("release second lock: %v", err)
		}
	}()
	<-released
}

func TestWaitReturnsTypedTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	first, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Errorf("release first lock: %v", err)
		}
	}()

	_, err = Wait(context.Background(), path, 20*time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected ErrWaitTimeout, got %v", err)
	}
}

func TestCanceledAndInvalidLockRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "run.lock")
	if _, _, err := Try(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Try canceled error=%v", err)
	}
	for _, test := range []struct {
		timeout time.Duration
		poll    time.Duration
	}{
		{0, time.Millisecond}, {time.Second, 0},
	} {
		if _, err := Wait(context.Background(), path, test.timeout, test.poll); err == nil {
			t.Fatalf("Wait(%s, %s) should fail", test.timeout, test.poll)
		}
	}
	if _, err := Wait(ctx, path, time.Second, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait canceled error=%v", err)
	}
	var nilLock *Lock
	if err := nilLock.Close(); err != nil {
		t.Fatalf("nil Close()=%v", err)
	}
}

func TestWaitReturnsCallerCancellationWhileContended(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	held, acquired, err := Try(context.Background(), path)
	if err != nil || !acquired {
		t.Fatalf("hold lock: acquired=%v err=%v", acquired, err)
	}
	t.Cleanup(func() { _ = held.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Wait(ctx, path, time.Second, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait cancellation=%v", err)
	}
}

func TestLockPathBelowRegularFileFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "run.lock")
	if _, _, err := Try(context.Background(), path); err == nil {
		t.Fatal("Try below a regular file should fail")
	}
	if _, err := Wait(context.Background(), path, time.Second, time.Millisecond); err == nil {
		t.Fatal("Wait below a regular file should fail")
	}
}
