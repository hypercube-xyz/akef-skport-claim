package cli

import (
	"fmt"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/spf13/cobra"
)

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

	validateCommand := &cobra.Command{Use: "validate", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
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
	parent.AddCommand(initCommand, validateCommand, pathCommand)
	return parent
}
