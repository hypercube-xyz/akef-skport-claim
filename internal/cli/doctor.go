package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hypercube-xyz/akef-skport-claim/internal/app"
	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/spf13/cobra"
)

func doctorCommand(options *rootOptions) *cobra.Command {
	var network bool
	command := &cobra.Command{Use: "doctor", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		path, err := config.ResolvePath(options.configPath)
		if err != nil {
			return withExitCode(report.ExitConfig, err)
		}
		if err := writeOutput(command.OutOrStdout(), "PASS config path: %s\n", path); err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			if writeErr := writeOutput(command.OutOrStdout(), "FAIL config: %v\n", err); writeErr != nil {
				return writeErr
			}
			return &exitError{code: report.ExitConfig, err: err}
		}
		if err := writeOutput(command.OutOrStdout(), "PASS config: %d enabled accounts\n", countEnabled(cfg)); err != nil {
			return err
		}

		cacheDir, err := config.CacheDir()
		if err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		probe, err := os.CreateTemp(cacheDir, ".doctor-*")
		if err != nil {
			return withExitCode(report.ExitInternal, err)
		}
		probeName := probe.Name()
		defer func() { _ = os.Remove(probeName) }()
		if err := probe.Close(); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("cache write check failed: %w", err))
		}
		if err := os.Remove(probeName); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("cache cleanup check failed: %w", err))
		}
		if err := writeOutput(command.OutOrStdout(), "PASS cache directory is writable\n"); err != nil {
			return err
		}

		probeLock, acquired, err := lock.Try(command.Context(), filepath.Join(cacheDir, "doctor.lock"))
		if err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("process lock check failed: %w", err))
		}
		if !acquired {
			return withExitCode(report.ExitInternal, errors.New("process lock check could not acquire the doctor lock"))
		}
		if err := probeLock.Close(); err != nil {
			return withExitCode(report.ExitInternal, fmt.Errorf("process lock release failed: %w", err))
		}
		if err := writeOutput(command.OutOrStdout(), "PASS process lock acquisition and release\n"); err != nil {
			return err
		}

		if !network {
			return writeOutput(command.OutOrStdout(), "PASS network activity skipped (use --network to check sessions)\n")
		}
		if err := writeOutput(command.OutOrStdout(), "WARN network status checks requested; no claims will be sent\n"); err != nil {
			return err
		}
		_, code, err := app.Execute(command.Context(), app.Options{ConfigPath: path, StatusOnly: true, Output: command.OutOrStdout()})
		if err != nil {
			return &exitError{code: code, err: err}
		}
		options.exitCode = code
		return nil
	}}
	command.Flags().BoolVar(&network, "network", false, "perform SKPORT refresh/status checks without claiming")
	return command
}
