package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/logging"
	"github.com/hypercube-xyz/akef-skport-claim/internal/model"
	"github.com/hypercube-xyz/akef-skport-claim/internal/notify"
	"github.com/hypercube-xyz/akef-skport-claim/internal/policy"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

type SKPortClient interface {
	Refresh(context.Context) (string, error)
	Status(context.Context, string) (skport.AttendanceResponse, error)
	ClaimOnce(context.Context, string) (skport.ClaimResponse, error)
}

type ClientFactory func(config.Account, time.Duration) SKPortClient

type Notifier interface {
	SendAll(context.Context, *config.Config, model.RunReport, *state.File) []error
}

type Options struct {
	ConfigPath    string
	Account       string
	Silent        bool
	StatusOnly    bool
	ClientFactory ClientFactory
	Sleep         func(context.Context, time.Duration) error
	Output        io.Writer
	Logger        *slog.Logger
	Notifier      Notifier
	LockWait      time.Duration
}

func Execute(ctx context.Context, options Options) (model.RunReport, int, error) {
	started := time.Now()
	cfg, err := config.Load(options.ConfigPath)
	if err != nil {
		return model.RunReport{}, report.ExitConfig, err
	}
	accounts, err := config.EnabledAccounts(cfg, options.Account)
	if err != nil {
		return model.RunReport{}, report.ExitConfig, err
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		return model.RunReport{}, report.ExitInternal, err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return model.RunReport{}, report.ExitInternal, err
	}

	logger, closer, err := configureLogger(options, cfg, cacheDir)
	if err != nil {
		return model.RunReport{}, report.ExitInternal, err
	}
	if closer != nil {
		defer closer.Close()
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
			return model.RunReport{}, report.ExitTransient, err
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
		wait := options.LockWait
		if wait <= 0 {
			wait = policy.ClaimLockWait
		}
		logger.Debug("waiting for claim lock", "timeout", wait)
		processLock, err = lock.Wait(ctx, filepath.Join(cacheDir, "run.lock"), wait, policy.LockPollInterval)
		if err != nil {
			return model.RunReport{}, report.ExitTransient, fmt.Errorf("acquire claim lock: %w", err)
		}
	}

	factory := options.ClientFactory
	if factory == nil {
		factory = func(account config.Account, timeout time.Duration) SKPortClient {
			return skport.New(account, timeout, skport.Options{})
		}
	}

	runReport := model.RunReport{}
	for i, account := range accounts {
		result := executeAccount(ctx, factory(account, cfg.Run.RequestTimeout.Duration), account, options.StatusOnly)
		runReport.Results = append(runReport.Results, result)
		if i < len(accounts)-1 && cfg.Run.AccountDelay.Duration > 0 {
			if err := sleep(ctx, cfg.Run.AccountDelay.Duration); err != nil {
				runReport.Duration = time.Since(started)
				writeReport(options, logger, runReport)
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
		statePath := filepath.Join(cacheDir, "state.json")
		stateFile, stateErr := state.Load(statePath)
		if stateErr != nil {
			logger.Warn("failed to load notification state", "error", stateErr)
			stateFile = &state.File{Notifications: map[string]time.Time{}}
		}
		sender := options.Notifier
		if sender == nil {
			sender = notify.New(notify.Options{})
		}
		for _, notifyErr := range sender.SendAll(ctx, cfg, runReport, stateFile) {
			logger.Warn("notification failed", "error", notifyErr)
		}
		if err := stateFile.Save(statePath); err != nil {
			logger.Warn("failed to save notification state", "error", err)
		}
	}
	writeReport(options, logger, runReport)
	return runReport, report.ExitCode(runReport), nil
}

func randomDelay(maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(maximum)))
}

func executeAccount(ctx context.Context, client SKPortClient, account config.Account, statusOnly bool) model.AccountResult {
	result := model.AccountResult{Account: account.Name}
	token, err := client.Refresh(ctx)
	if err != nil {
		return fromError(result, err)
	}
	status, err := client.Status(ctx, token)
	if err != nil {
		return fromError(result, err)
	}
	if skport.IsAuthCode(status.Code) {
		result.Outcome = model.AuthExpired
		result.Summary = "status indicates login is required"
		return result
	}
	if status.Code != 0 {
		result.Outcome = model.TransientError
		result.Summary = "status API returned an error"
		return result
	}

	attendance := status.State()
	if !attendance.SessionValid {
		result.Outcome = model.AuthExpired
		result.Summary = "status indicates login is required"
		return result
	}
	if !attendance.AvailableKnown && !attendance.DoneKnown {
		result.Outcome = model.InternalError
		result.Summary = "status response did not contain a recognizable attendance state"
		return result
	}
	if statusOnly {
		switch {
		case attendance.Available:
			result.Outcome, result.Summary = model.Skipped, "attendance is available"
		case attendance.Done:
			result.Outcome, result.Summary = model.AlreadyClaimed, doneSummary(attendance)
		default:
			result.Outcome, result.Summary = model.Unavailable, "attendance unavailable"
		}
		return result
	}
	if !attendance.Available {
		if attendance.Done {
			result.Outcome, result.Summary = model.AlreadyClaimed, doneSummary(attendance)
		} else {
			result.Outcome, result.Summary = model.Unavailable, "no action required"
		}
		return result
	}

	expected := status.AvailableRewards()
	claim, err := client.ClaimOnce(ctx, token)
	if err != nil {
		return fromError(result, err)
	}
	switch claim.Classify() {
	case skport.ClaimSuccess:
		result.Outcome = model.Claimed
		result.Rewards = claim.Rewards()
		if len(result.Rewards) == 0 {
			result.Rewards = expected
		}
		result.Summary = rewardsSummary(result.Rewards)
	case skport.ClaimAlreadyDone:
		result.Outcome, result.Summary = model.AlreadyClaimed, "API reports already claimed"
	case skport.ClaimAuthError:
		result.Outcome, result.Summary = model.AuthExpired, "claim rejected because login is required"
	case skport.ClaimAPIError:
		result.Outcome, result.Summary = model.ClaimError, "claim API returned a definite error"
	}
	return result
}

func doneSummary(attendance skport.AttendanceState) string {
	if attendance.Conflict {
		return "conflicting attendance flags; treated as already claimed"
	}
	return "already claimed"
}

func fromError(result model.AccountResult, err error) model.AccountResult {
	var typed *skport.Error
	if errors.As(err, &typed) {
		switch typed.Kind {
		case skport.ErrorAuth:
			result.Outcome, result.Summary = model.AuthExpired, "login is required"
		case skport.ErrorTransient:
			result.Outcome, result.Summary = model.TransientError, "temporary network or server failure"
		case skport.ErrorClaim:
			result.Outcome, result.Summary = model.ClaimError, "claim API returned a definite error"
		case skport.ErrorAmbiguous:
			result.Outcome, result.Summary = model.AmbiguousClaim, "claim result is unknown; it will not be retried automatically"
		default:
			result.Outcome, result.Summary = model.InternalError, "unexpected internal error"
		}
		return result
	}
	result.Outcome, result.Summary = model.InternalError, "unexpected internal error"
	return result
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

func writeReport(options Options, logger *slog.Logger, runReport model.RunReport) {
	text := report.Format(runReport)
	if options.Silent {
		logger.Info("aggregate report", "report", text)
		return
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}
	fmt.Fprintln(output, text)
}

func rewardsSummary(rewards []model.Reward) string {
	if len(rewards) == 0 {
		return "rewards unknown"
	}
	var builder strings.Builder
	for i, reward := range rewards {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(reward.Summary())
	}
	return builder.String()
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
