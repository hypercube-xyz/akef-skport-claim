package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/logging"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

const (
	claimLockWait    = 10 * time.Minute
	lockPollInterval = 250 * time.Millisecond
)

type SKPortClient interface {
	Refresh(context.Context) (string, error)
	Status(context.Context, string) (skport.AttendanceResponse, error)
	ClaimOnce(context.Context, string) (skport.ClaimResponse, error)
}

type ClientFactory func(config.Account, time.Duration) SKPortClient

type Notifier interface {
	SendAll(context.Context, *config.Config, result.Run, *state.Store) []error
}

type Options struct {
	ConfigPath    string
	AccountName   string
	Silent        bool
	StatusOnly    bool
	ClientFactory ClientFactory
	Sleep         func(context.Context, time.Duration) error
	Output        io.Writer
	Logger        *slog.Logger
	Notifier      Notifier
	ClaimLockWait time.Duration
}

func Execute(ctx context.Context, options Options) (result.Run, int, error) {
	started := time.Now()
	cfg, err := config.Load(options.ConfigPath)
	if err != nil {
		return result.Run{}, report.ExitConfig, err
	}
	accounts, err := config.EnabledAccounts(cfg, options.AccountName)
	if err != nil {
		return result.Run{}, report.ExitConfig, err
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		return result.Run{}, report.ExitInternal, err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return result.Run{}, report.ExitInternal, err
	}

	logger, closer, err := configureLogger(options, cfg, cacheDir)
	if err != nil {
		return result.Run{}, report.ExitInternal, err
	}
	if closer != nil {
		defer func() {
			if err := closer.Close(); err != nil {
				logger.Warn("failed to close scheduled log", "error", err)
			}
		}()
	}
	logger.Info("starting attendance operation", "silent", options.Silent, "status_only", options.StatusOnly)

	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	// Jitter must happen before the claim lock is held. Otherwise an idle process
	// can block a manual or scheduled run for the entire random-delay window.
	if !options.StatusOnly && cfg.Run.RandomDelay.Duration > 0 {
		delay := randomDelay(cfg.Run.RandomDelay.Duration)
		logger.Info("waiting before requests", "duration", delay)
		if err := sleep(ctx, delay); err != nil {
			return result.Run{}, report.ExitTransient, err
		}
	}

	// Status is read-only and does not participate in the exclusive claim lock.
	// Claim-capable runs wait for the current owner, then re-check attendance,
	// which avoids both duplicate POSTs and a whole-day skip.
	var processLock *lock.Lock
	releaseClaimLock := func() {
		if processLock == nil {
			return
		}
		if err := processLock.Close(); err != nil {
			logger.Warn("failed to release claim lock", "error", err)
		}
		processLock = nil
	}
	defer releaseClaimLock()

	if !options.StatusOnly {
		wait := options.ClaimLockWait
		if wait <= 0 {
			wait = claimLockWait
		}
		logger.Debug("waiting for claim lock", "timeout", wait)
		processLock, err = lock.Wait(ctx, filepath.Join(cacheDir, "run.lock"), wait, lockPollInterval)
		if err != nil {
			return result.Run{}, report.ExitTransient, fmt.Errorf("acquire claim lock: %w", err)
		}
	}

	factory := options.ClientFactory
	if factory == nil {
		factory = func(account config.Account, timeout time.Duration) SKPortClient {
			return skport.New(account, timeout, skport.Options{})
		}
	}

	runReport := result.Run{}
	for i, account := range accounts {
		accountResult := executeAccount(ctx, factory(account, cfg.Run.RequestTimeout.Duration), account, options.StatusOnly)
		runReport.Accounts = append(runReport.Accounts, accountResult)
		if i < len(accounts)-1 && cfg.Run.AccountDelay.Duration > 0 {
			if err := sleep(ctx, cfg.Run.AccountDelay.Duration); err != nil {
				runReport.Duration = time.Since(started)
				_ = writeReport(options, logger, runReport)
				return runReport, report.ExitTransient, fmt.Errorf("wait between accounts: %w", err)
			}
		}
	}
	runReport.Duration = time.Since(started)

	// The lock protects refresh/status/claim sequencing only. Notifications and
	// state persistence happen after it is released so a slow destination cannot
	// block a later run from re-checking attendance.
	releaseClaimLock()

	if !options.StatusOnly {
		sendNotifications(ctx, logger, cacheDir, cfg, runReport, options.Notifier)
	}
	if err := writeReport(options, logger, runReport); err != nil {
		return runReport, report.ExitInternal, err
	}
	return runReport, report.ExitCode(runReport), nil
}

func randomDelay(maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return 0
	}
	// This jitter is not a secret or security boundary; math/rand is appropriate.
	return time.Duration(rand.Int64N(int64(maximum))) // #nosec G404
}

func configureLogger(options Options, cfg *config.Config, cacheDir string) (*slog.Logger, io.Closer, error) {
	if options.Logger != nil {
		return options.Logger, nil, nil
	}
	level := logging.ParseLevel(cfg.App.LogLevel)
	if options.Silent {
		logger, closer, _, err := logging.Scheduled(cacheDir, level)
		return logger, closer, err
	}
	return logging.Interactive(level), nil, nil
}

func writeReport(options Options, logger *slog.Logger, runReport result.Run) error {
	text := report.Format(runReport)
	if options.Silent {
		logger.Info("aggregate report", "report", text)
		return nil
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}
	if _, err := fmt.Fprintln(output, text); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
