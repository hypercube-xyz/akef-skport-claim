package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hypercube-xyz/akef-skport-claim/internal/app"
	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	processlock "github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/logging"
	"github.com/hypercube-xyz/akef-skport-claim/internal/notify"
	"github.com/hypercube-xyz/akef-skport-claim/internal/policy"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/version"
	"github.com/spf13/cobra"
)

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
		executionCtx, cancel = context.WithTimeout(ctx, policy.ScheduledExecutionTimeout)
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
	var account string
	command := &cobra.Command{
		Use:   "run",
		Short: "Check status and claim once when available",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: options.configPath, Account: account, Silent: options.silent, Output: command.OutOrStdout()})
			if err != nil {
				return &exitError{code: code, err: err}
			}
			options.exitCode = code
			return nil
		},
	}
	command.Flags().StringVar(&account, "account", "", "run only the named enabled account")
	return command
}

func statusCommand(options *rootOptions) *cobra.Command {
	var account string
	command := &cobra.Command{
		Use:   "status",
		Short: "Check attendance without claiming",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: options.configPath, Account: account, StatusOnly: true, Output: command.OutOrStdout()})
			if err != nil {
				return &exitError{code: code, err: err}
			}
			options.exitCode = code
			return nil
		},
	}
	command.Flags().StringVar(&account, "account", "", "check only the named enabled account")
	return command
}

func configCommand(options *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "config", Short: "Initialize and validate configuration"}
	var force bool
	initCommand := &cobra.Command{Use: "init", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		path, err := config.Init(options.configPath, force)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		fmt.Fprintln(command.OutOrStdout(), path)
		return nil
	}}
	initCommand.Flags().BoolVar(&force, "force", false, "replace an existing config")
	validate := &cobra.Command{Use: "validate", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(options.configPath)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		fmt.Fprintf(command.OutOrStdout(), "valid: %s (%d enabled accounts)\n", cfg.Path, countEnabled(cfg))
		return nil
	}}
	pathCommand := &cobra.Command{Use: "path", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		path, err := config.ResolvePath(options.configPath)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		fmt.Fprintln(command.OutOrStdout(), path)
		return nil
	}}
	parent.AddCommand(initCommand, validate, pathCommand)
	return parent
}

func notifyCommand(options *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "notify", Short: "Test notification destinations"}
	test := &cobra.Command{Use: "test [TARGET]", Args: maximumNArgs(1), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(options.configPath)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		matched := 0
		sender := notify.New(notify.Options{})
		for _, target := range cfg.Notifications.Targets {
			if name != "" && target.Name != name {
				continue
			}
			if name == "" && !target.Enabled {
				continue
			}
			target.Enabled = true
			if err := config.ValidateTarget(target); err != nil {
				return withExitCode(report.ExitConfig, fmt.Errorf("notification target %q: %w", target.Name, err))
			}
			if err := sender.SendTest(command.Context(), target); err != nil {
				return withExitCode(report.ExitTransient, fmt.Errorf("notification target %q: %w", target.Name, err))
			}
			matched++
			fmt.Fprintf(command.OutOrStdout(), "sent test notification to %s\n", target.Name)
		}
		if matched == 0 {
			if name == "" {
				return withExitCode(report.ExitConfig, errors.New("no enabled notification targets found"))
			}
			return withExitCode(report.ExitConfig, fmt.Errorf("notification target %q not found", name))
		}
		return nil
	}}
	parent.AddCommand(test)
	return parent
}

func doctorCommand(options *rootOptions) *cobra.Command {
	var network bool
	command := &cobra.Command{Use: "doctor", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		path, err := config.ResolvePath(options.configPath)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		fmt.Fprintf(command.OutOrStdout(), "PASS config path: %s\n", path)
		cfg, err := config.Load(path)
		if err != nil {
			fmt.Fprintf(command.OutOrStdout(), "FAIL config: %v\n", err)
			return &exitError{code: report.ExitConfig, err: err}
		}
		fmt.Fprintf(command.OutOrStdout(), "PASS config: %d enabled accounts\n", countEnabled(cfg))
		cache, err := config.CacheDir()
		if err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		if err := os.MkdirAll(cache, 0o700); err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		probe, err := os.CreateTemp(cache, ".doctor-*")
		if err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		probeName := probe.Name()
		if err := probe.Close(); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("cache write check failed: %w", err))
		}
		if err := os.Remove(probeName); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("cache cleanup check failed: %w", err))
		}
		fmt.Fprintln(command.OutOrStdout(), "PASS cache directory is writable")
		probeLock, acquired, lockErr := processlock.Try(command.Context(), filepath.Join(cache, "doctor.lock"))
		if lockErr != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("process lock check failed: %w", lockErr))
		}
		if !acquired {
			return withExitCode(report.ExitInternal, errors.New("process lock check could not acquire the doctor lock"))
		}
		if err := probeLock.Close(); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("process lock release failed: %w", err))
		}
		fmt.Fprintln(command.OutOrStdout(), "PASS process lock acquisition and release")
		if network {
			fmt.Fprintln(command.OutOrStdout(), "WARN network status checks requested; no claims will be sent")
			_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: path, StatusOnly: true, Output: command.OutOrStdout()})
			if err != nil {
				return &exitError{code: code, err: err}
			}
			options.exitCode = code
		} else {
			fmt.Fprintln(command.OutOrStdout(), "PASS network activity skipped (use --network to check sessions)")
		}
		return nil
	}}
	command.Flags().BoolVar(&network, "network", false, "perform SKPORT refresh/status checks without claiming")
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
