package cli

import (
	"errors"
	"fmt"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/notify"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/spf13/cobra"
)

func notifyCommand(options *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "notify", Short: "Test notification destinations"}
	testCommand := &cobra.Command{Use: "test [TARGET]", Args: maximumNArgs(1), RunE: func(command *cobra.Command, args []string) error {
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
			if err := writeOutput(command.OutOrStdout(), "sent test notification to %s\n", target.Name); err != nil {
				return err
			}
		}
		if matched == 0 {
			if name == "" {
				return withExitCode(report.ExitConfig, errors.New("no enabled notification targets found"))
			}
			return withExitCode(report.ExitConfig, fmt.Errorf("notification target %q not found", name))
		}
		return nil
	}}
	parent.AddCommand(testCommand)
	return parent
}
