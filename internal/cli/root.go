package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/app"
	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/logging"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/version"
	"github.com/spf13/cobra"
)

const scheduledExecutionTimeout = 30 * time.Minute

type rootOptions struct {
	configPath string
	silent     bool
	output     io.Writer
	errorOut   io.Writer
	exitCode   int
}

func Execute(ctx context.Context, args []string) int {
	options := &rootOptions{output: os.Stdout, errorOut: os.Stderr}
	preSilent := hasSilent(args)
	root := newRoot(options)
	root.SetArgs(args)
	if preSilent {
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
	} else {
		root.SetOut(options.output)
		root.SetErr(options.errorOut)
	}
	executionCtx := ctx
	if preSilent {
		var cancel context.CancelFunc
		executionCtx, cancel = context.WithTimeout(ctx, scheduledExecutionTimeout)
		defer cancel()
	}
	if err := root.ExecuteContext(executionCtx); err != nil {
		if preSilent {
			writeSilentError(err)
			return errorCode(err)
		}
		fmt.Fprintln(options.errorOut, err)
		return errorCode(err)
	}
	return options.exitCode
}

func newRoot(options *rootOptions) *cobra.Command {
	root := &cobra.Command{
		Use:               "akef-claim",
		Short:             "Local-only SKPORT Endfield daily attendance CLI",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			if options.silent && command.Name() != "run" {
				return &exitError{code: report.ExitConfig, err: errors.New("--silent is supported only with run")}
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return withExitCode(report.ExitConfig, err)
	})
	root.PersistentFlags().StringVar(&options.configPath, "config", "", "path to TOML configuration")
	root.PersistentFlags().BoolVar(&options.silent, "silent", false, "suppress terminal output and use scheduled logging")
	root.AddCommand(runCommand(options), statusCommand(options), doctorCommand(options), configCommand(options), notifyCommand(options), versionCommand())
	return root
}

func runCommand(options *rootOptions) *cobra.Command {
	var accountName string
	command := &cobra.Command{
		Use:   "run",
		Short: "Check status and claim once when available",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: options.configPath, AccountName: accountName, Silent: options.silent, Output: command.OutOrStdout()})
			if err != nil {
				return &exitError{code: code, err: err}
			}
			options.exitCode = code
			return nil
		},
	}
	command.Flags().StringVar(&accountName, "account", "", "run only the named enabled account")
	return command
}

func statusCommand(options *rootOptions) *cobra.Command {
	var accountName string
	command := &cobra.Command{
		Use:   "status",
		Short: "Check attendance without claiming",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: options.configPath, AccountName: accountName, StatusOnly: true, Output: command.OutOrStdout()})
			if err != nil {
				return &exitError{code: code, err: err}
			}
			options.exitCode = code
			return nil
		},
	}
	command.Flags().StringVar(&accountName, "account", "", "check only the named enabled account")
	return command
}

func versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Args: noArgs, Run: func(command *cobra.Command, _ []string) { fmt.Fprintln(command.OutOrStdout(), version.String()) }}
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("operation completed with exit code %d", e.code)
}

func (e *exitError) Unwrap() error { return e.err }

func errorCode(err error) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return report.ExitTransient
	}
	var typed *exitError
	if errors.As(err, &typed) && typed.code != 0 {
		return typed.code
	}
	if isUsageError(err) {
		return report.ExitConfig
	}
	return report.ExitInternal
}

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	var typed *exitError
	if errors.As(err, &typed) {
		return err
	}
	return &exitError{code: code, err: err}
}

func noArgs(command *cobra.Command, args []string) error {
	return withExitCode(report.ExitConfig, cobra.NoArgs(command, args))
}

func maximumNArgs(limit int) cobra.PositionalArgs {
	validate := cobra.MaximumNArgs(limit)
	return func(command *cobra.Command, args []string) error {
		return withExitCode(report.ExitConfig, validate(command, args))
	}
}

func isUsageError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"unknown command ",
		"unknown flag:",
		"flag needs an argument:",
		"required flag",
		"invalid argument ",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func hasSilent(args []string) bool {
	for _, arg := range args {
		if arg == "--silent" {
			return true
		}
		value, found := strings.CutPrefix(arg, "--silent=")
		if !found {
			continue
		}
		enabled, err := strconv.ParseBool(value)
		if err == nil && enabled {
			return true
		}
	}
	return false
}

func writeSilentError(err error) {
	cache, cacheErr := config.CacheDir()
	if cacheErr != nil {
		return
	}
	logger, closer, _, logErr := logging.Scheduled(cache, slog.LevelInfo)
	if logErr != nil {
		return
	}
	defer closer.Close()
	logger.Error("silent command failed", "error", err)
}

func countEnabled(cfg *config.Config) int {
	count := 0
	for _, account := range cfg.Accounts {
		if account.Enabled {
			count++
		}
	}
	return count
}
