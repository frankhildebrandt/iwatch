package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultBufferLines = 1_000_000

type Config struct {
	WatchPath      string          `json:"watchPath"`
	BufferLines    int             `json:"bufferLines"`
	DefaultCommand string          `json:"defaultCommand"`
	Commands       []CommandConfig `json:"commands"`
	HighlightRules []HighlightRule `json:"highlightRules"`
	UI             UIConfig        `json:"ui"`
}

type CommandConfig struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Cmd    string `json:"cmd"`
	CWD    string `json:"cwd,omitempty"`
	Source string `json:"source,omitempty"`
}

type HighlightRule struct {
	ID       string `json:"id"`
	Pattern  string `json:"pattern"`
	Style    string `json:"style"`
	Priority int    `json:"priority"`
}

type UIConfig struct {
	OpenPanes      []string `json:"openPanes"`
	SplitDirection string   `json:"splitDirection"`
	FocusPane      string   `json:"focusPane"`
}

type CLIOverrides struct {
	ConfigPath  string
	WatchPath   string
	BufferLines int
	CommandID   string
}

func Default() Config {
	return Config{
		BufferLines: DefaultBufferLines,
		UI: UIConfig{
			OpenPanes:      []string{"log"},
			SplitDirection: "vertical",
			FocusPane:      "log",
		},
	}
}

func ResolveConfig(baseDir string, overrides CLIOverrides) (Config, string, error) {
	cfg := Default()
	var usedPath string

	paths, err := candidatePaths(baseDir, overrides.ConfigPath)
	if err != nil {
		return Config{}, "", err
	}

	for _, path := range paths {
		loaded, ok, err := loadFile(path)
		if err != nil {
			return Config{}, "", err
		}
		if !ok {
			continue
		}
		cfg = merge(cfg, loaded)
		usedPath = path
	}

	if overrides.WatchPath != "" {
		cfg.WatchPath = overrides.WatchPath
	}
	if overrides.BufferLines > 0 {
		cfg.BufferLines = overrides.BufferLines
	}
	if overrides.CommandID != "" {
		cfg.DefaultCommand = overrides.CommandID
	}
	if cfg.BufferLines <= 0 {
		cfg.BufferLines = DefaultBufferLines
	}
	if len(cfg.UI.OpenPanes) == 0 {
		cfg.UI.OpenPanes = []string{"log"}
	}
	if cfg.UI.SplitDirection == "" {
		cfg.UI.SplitDirection = "vertical"
	}
	if cfg.UI.FocusPane == "" {
		cfg.UI.FocusPane = "log"
	}

	return cfg, usedPath, nil
}

func candidatePaths(baseDir, explicit string) ([]string, error) {
	if explicit != "" {
		return []string{explicit}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	return []string{
		filepath.Join(home, ".iwatch", "config.json"),
		filepath.Join(baseDir, ".iwatch", "config.json"),
		filepath.Join(baseDir, ".iwatch.config.json"),
	}, nil
}

func loadFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, true, nil
}

func merge(base, override Config) Config {
	out := base
	if override.WatchPath != "" {
		out.WatchPath = override.WatchPath
	}
	if override.BufferLines > 0 {
		out.BufferLines = override.BufferLines
	}
	if override.DefaultCommand != "" {
		out.DefaultCommand = override.DefaultCommand
	}
	if override.Commands != nil {
		out.Commands = override.Commands
	}
	if override.HighlightRules != nil {
		out.HighlightRules = override.HighlightRules
	}
	if len(override.UI.OpenPanes) > 0 {
		out.UI.OpenPanes = override.UI.OpenPanes
	}
	if override.UI.SplitDirection != "" {
		out.UI.SplitDirection = override.UI.SplitDirection
	}
	if override.UI.FocusPane != "" {
		out.UI.FocusPane = override.UI.FocusPane
	}
	return out
}
