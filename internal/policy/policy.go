// Package policy contains process-wide operational limits shared by the CLI and application runner.
package policy

import "time"

const (
	// ScheduledExecutionTimeout is the hard upper bound for one silent scheduled
	// invocation, including startup jitter, lock contention, API work, and
	// notifications.
	ScheduledExecutionTimeout = 30 * time.Minute

	// ClaimLockWait bounds how long a run waits for another claim-capable process
	// to finish. Waiting and re-checking attendance is safer than skipping the day
	// or issuing a concurrent claim request.
	ClaimLockWait = 10 * time.Minute

	// LockPollInterval controls how often a waiting process retries the advisory
	// file lock. It is intentionally small compared with network operations.
	LockPollInterval = 250 * time.Millisecond
)
