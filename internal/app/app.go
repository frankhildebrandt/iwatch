package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/stream"
	"github.com/stackriot/iwatch/internal/tui"
)

func Run(args []string) error {
	var overrides config.CLIOverrides

	root := &cobra.Command{
		Use:   "iwatch",
		Short: "Interactive dev log TUI for build and run workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}

			cfg, usedPath, err := config.ResolveConfig(wd, overrides)
			if err != nil {
				return err
			}
			projectConfigPath := config.ResolveProjectConfigPath(wd, usedPath)

			discovered, err := detect.Discover(wd, cfg)
			if err != nil {
				return err
			}
			if len(discovered.Commands) == 0 {
				return fmt.Errorf("no commands detected in %s; add one to config or create a Makefile/package.json", wd)
			}

			defaultCommand := discovered.Default
			if overrides.CommandID != "" {
				defaultCommand = overrides.CommandID
			}
			if overrides.CommandID == "" {
				discovered.Commands, defaultCommand = resolvePositionalCommand(wd, discovered.Commands, defaultCommand, args)
			}

			logBuffer, err := buffer.New(cfg.BufferLines, cfg.HighlightRules)
			if err != nil {
				return fmt.Errorf("configure buffer: %w", err)
			}

			run := runner.New()
			streams := stream.New(cfg.Streams, wd)

			return tui.Run(cfg, projectConfigPath, discovered.Commands, defaultCommand, logBuffer, run, streams)
		},
	}

	root.Flags().StringVar(&overrides.ConfigPath, "config", "", "Explicit config file to load")
	root.Flags().IntVar(&overrides.BufferLines, "buffer-lines", 0, "Maximum number of log lines to keep")
	root.Flags().StringVar(&overrides.CommandID, "command", "", "Command ID to start immediately")

	root.SetArgs(args)
	return root.Execute()
}
