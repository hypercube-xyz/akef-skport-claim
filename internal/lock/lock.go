package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

var ErrWaitTimeout = errors.New("timed out waiting for process lock")

type Lock struct{ file *flock.Flock }

func Try(ctx context.Context, path string) (*Lock, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	file := flock.New(path)
	locked, err := file.TryLock()
	if err != nil {
		return nil, false, err
	}
	if !locked {
		return nil, false, nil
	}
	return &Lock{file: file}, true, nil
}

// Wait acquires the lock or returns when the caller's context is canceled or
// the supplied timeout expires. A single flock handle is reused while polling
// so the operation does not repeatedly open the lock file.
func Wait(ctx context.Context, path string, timeout, pollInterval time.Duration) (*Lock, error) {
	if timeout <= 0 {
		return nil, errors.New("process lock timeout must be greater than zero")
	}
	if pollInterval <= 0 {
		return nil, errors.New("process lock poll interval must be greater than zero")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	file := flock.New(path)
	for {
		locked, err := file.TryLock()
		if err != nil {
			return nil, err
		}
		if locked {
			return &Lock{file: file}, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return nil, fmt.Errorf("%w after %s", ErrWaitTimeout, timeout)
			}
			return nil, waitCtx.Err()
		case <-timer.C:
		}
	}
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Unlock()
}
