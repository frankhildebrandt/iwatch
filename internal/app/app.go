package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/tui"
	"github.com/stackriot/iwatch/internal/watch"
)

func Run(args []string) error {
	var overrides config.CLIOverrides

	root := &cobra.Command{
		Use:   "iwatch",
		Short: "Interactive watch tool for build and run workflows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}

			cfg, _, err := config.ResolveConfig(wd, overrides)
			if err != nil {
				return err
			}

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

			watchPath := cfg.WatchPath
			if watchPath == "" {
				watchPath = wd
			}
			if !filepath.IsAbs(watchPath) {
				watchPath = filepath.Join(wd, watchPath)
			}

			logBuffer, err := buffer.New(cfg.BufferLines, cfg.HighlightRules)
			if err != nil {
				return fmt.Errorf("configure buffer: %w", err)
			}

			run := runner.New()
			watcher, err := watch.New(watchPath)
			if err != nil {
				return fmt.Errorf("start watcher: %w", err)
			}

			return tui.Run(cfg, discovered.Commands, defaultCommand, logBuffer, run, watcher)
		},
	}

	root.Flags().StringVar(&overrides.WatchPath, "path", "", "Path to watch instead of the current working directory")
	root.Flags().StringVar(&overrides.ConfigPath, "config", "", "Explicit config file to load")
	root.Flags().IntVar(&overrides.BufferLines, "buffer-lines", 0, "Maximum number of log lines to keep")
	root.Flags().StringVar(&overrides.CommandID, "command", "", "Command ID to start immediately")

	root.SetArgs(args)
	return root.Execute()
}
