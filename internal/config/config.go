package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultBufferLines defines the fallback number of log lines kept in memory.
	DefaultBufferLines = 1_000_000
	// DefaultPresetID is the identifier of the built-in preset.
	DefaultPresetID = "default"
)

// Config contains the persisted app configuration.
type Config struct {
	WatchPath      string          `json:"watchPath"`
	BufferLines    int             `json:"bufferLines"`
	DefaultCommand string          `json:"defaultCommand"`
	Commands       []CommandConfig `json:"commands"`
	HighlightRules []HighlightRule `json:"highlightRules"`
	UI             UIConfig        `json:"ui"`
}

// CommandConfig defines a configured command entry.
type CommandConfig struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Cmd    string `json:"cmd"`
	CWD    string `json:"cwd,omitempty"`
	Source string `json:"source,omitempty"`
}

// HighlightRule configures a regex-based highlight style.
type HighlightRule struct {
	ID       string `json:"id"`
	Pattern  string `json:"pattern"`
	Style    string `json:"style"`
	Priority int    `json:"priority"`
}

// FilterCondition defines one field or text match inside a clause.
type FilterCondition struct {
	Field string `json:"field,omitempty"`
	Value string `json:"value"`
}

// FilterClause groups AND conditions that are ORed with sibling clauses.
type FilterClause struct {
	Conditions []FilterCondition `json:"conditions"`
}

// FilterPreset stores a named filter and optional highlight overrides.
type FilterPreset struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Clauses        []FilterClause  `json:"clauses"`
	HighlightRules []HighlightRule `json:"highlightRules,omitempty"`
}

// LogViewConfig controls how log lines are rendered in the UI.
type LogViewConfig struct {
	VisibleFields  []string `json:"visibleFields,omitempty"`
	ShowRawMessage *bool    `json:"showRawMessage,omitempty"`
	ShowSource     *bool    `json:"showSource,omitempty"`
	ShowTimestamp  *bool    `json:"showTimestamp,omitempty"`
	TimeFormat     string   `json:"timeFormat,omitempty"`
	WrapMode       string   `json:"wrapMode,omitempty"`
}

// UIConfig contains TUI layout and preset settings.
type UIConfig struct {
	OpenPanes      []string       `json:"openPanes"`
	SplitDirection string         `json:"splitDirection"`
	FocusPane      string         `json:"focusPane"`
	ActivePreset   string         `json:"activePreset,omitempty"`
	Presets        []FilterPreset `json:"presets,omitempty"`
	LogView        LogViewConfig  `json:"logView"`
}

// CLIOverrides contains command-line configuration overrides.
type CLIOverrides struct {
	ConfigPath  string
	WatchPath   string
	BufferLines int
	CommandID   string
}

// Default returns the normalized default configuration.
func Default() Config {
	cfg := Config{
		BufferLines: DefaultBufferLines,
		UI: UIConfig{
			OpenPanes:      []string{"log"},
			SplitDirection: "vertical",
			FocusPane:      "log",
			ActivePreset:   DefaultPresetID,
			Presets: []FilterPreset{
				{
					ID:    DefaultPresetID,
					Title: "Default",
				},
			},
			LogView: LogViewConfig{
				ShowRawMessage: boolPtr(true),
				ShowTimestamp:  boolPtr(true),
				ShowSource:     boolPtr(false),
				TimeFormat:     "time",
				WrapMode:       "off",
			},
		},
	}
	return normalize(cfg)
}

// ResolveConfig loads config files in precedence order and applies CLI overrides.
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

	cfg = normalize(cfg)
	return cfg, usedPath, nil
}

// ResolveProjectConfigPath returns the project-local config path that should be edited.
func ResolveProjectConfigPath(baseDir, usedPath string) string {
	if usedPath != "" {
		if samePath(usedPath, filepath.Join(baseDir, ".iwatch", "config.json")) || samePath(usedPath, filepath.Join(baseDir, ".iwatch.config.json")) {
			return usedPath
		}
	}

	projectPath := filepath.Join(baseDir, ".iwatch", "config.json")
	rootPath := filepath.Join(baseDir, ".iwatch.config.json")

	if _, err := os.Stat(projectPath); err == nil {
		return projectPath
	}
	if _, err := os.Stat(rootPath); err == nil {
		return rootPath
	}
	return projectPath
}

// Save normalizes and writes the configuration to disk.
func Save(path string, cfg Config) error {
	cfg = normalize(cfg)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// DefaultMerge normalizes a partially edited config using default values.
func DefaultMerge(cfg Config) Config {
	return normalize(cfg)
}

// Clone returns a deep copy of the configuration.
func Clone(cfg Config) Config {
	out := cfg
	out.Commands = append([]CommandConfig(nil), cfg.Commands...)
	out.HighlightRules = append([]HighlightRule(nil), cfg.HighlightRules...)
	out.UI.OpenPanes = append([]string(nil), cfg.UI.OpenPanes...)
	out.UI.Presets = make([]FilterPreset, len(cfg.UI.Presets))
	for i, preset := range cfg.UI.Presets {
		out.UI.Presets[i] = preset
		out.UI.Presets[i].Clauses = make([]FilterClause, len(preset.Clauses))
		for j, clause := range preset.Clauses {
			out.UI.Presets[i].Clauses[j] = clause
			out.UI.Presets[i].Clauses[j].Conditions = append([]FilterCondition(nil), clause.Conditions...)
		}
		out.UI.Presets[i].HighlightRules = append([]HighlightRule(nil), preset.HighlightRules...)
	}
	out.UI.LogView.VisibleFields = append([]string(nil), cfg.UI.LogView.VisibleFields...)
	if cfg.UI.LogView.ShowRawMessage != nil {
		out.UI.LogView.ShowRawMessage = boolPtr(*cfg.UI.LogView.ShowRawMessage)
	}
	if cfg.UI.LogView.ShowSource != nil {
		out.UI.LogView.ShowSource = boolPtr(*cfg.UI.LogView.ShowSource)
	}
	if cfg.UI.LogView.ShowTimestamp != nil {
		out.UI.LogView.ShowTimestamp = boolPtr(*cfg.UI.LogView.ShowTimestamp)
	}
	return out
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
	if override.UI.ActivePreset != "" {
		out.UI.ActivePreset = override.UI.ActivePreset
	}
	if override.UI.Presets != nil {
		out.UI.Presets = override.UI.Presets
	}
	if override.UI.LogView.VisibleFields != nil {
		out.UI.LogView.VisibleFields = override.UI.LogView.VisibleFields
	}
	if override.UI.LogView.ShowRawMessage != nil {
		out.UI.LogView.ShowRawMessage = override.UI.LogView.ShowRawMessage
	}
	if override.UI.LogView.ShowSource != nil {
		out.UI.LogView.ShowSource = override.UI.LogView.ShowSource
	}
	if override.UI.LogView.ShowTimestamp != nil {
		out.UI.LogView.ShowTimestamp = override.UI.LogView.ShowTimestamp
	}
	if override.UI.LogView.TimeFormat != "" {
		out.UI.LogView.TimeFormat = override.UI.LogView.TimeFormat
	}
	if override.UI.LogView.WrapMode != "" {
		out.UI.LogView.WrapMode = override.UI.LogView.WrapMode
	}
	return out
}

func normalize(cfg Config) Config {
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
	if cfg.UI.ActivePreset == "" {
		cfg.UI.ActivePreset = DefaultPresetID
	}
	if cfg.UI.LogView.TimeFormat == "" {
		cfg.UI.LogView.TimeFormat = "time"
	}
	if cfg.UI.LogView.WrapMode == "" {
		cfg.UI.LogView.WrapMode = "off"
	}
	if cfg.UI.LogView.ShowRawMessage == nil {
		cfg.UI.LogView.ShowRawMessage = boolPtr(true)
	}
	if cfg.UI.LogView.ShowSource == nil {
		cfg.UI.LogView.ShowSource = boolPtr(false)
	}
	if cfg.UI.LogView.ShowTimestamp == nil {
		cfg.UI.LogView.ShowTimestamp = boolPtr(true)
	}
	if !*cfg.UI.LogView.ShowRawMessage && !*cfg.UI.LogView.ShowSource && !*cfg.UI.LogView.ShowTimestamp && len(cfg.UI.LogView.VisibleFields) == 0 {
		cfg.UI.LogView.ShowRawMessage = boolPtr(true)
		cfg.UI.LogView.ShowTimestamp = boolPtr(true)
	}
	cfg.UI.Presets = normalizePresets(cfg.UI.Presets)
	if !presetExists(cfg.UI.Presets, cfg.UI.ActivePreset) {
		cfg.UI.ActivePreset = cfg.UI.Presets[0].ID
	}
	return cfg
}

func normalizePresets(presets []FilterPreset) []FilterPreset {
	if len(presets) == 0 {
		return []FilterPreset{{ID: DefaultPresetID, Title: "Default"}}
	}

	out := make([]FilterPreset, 0, len(presets))
	seen := map[string]int{}
	for idx, preset := range presets {
		if preset.ID == "" {
			preset.ID = fmt.Sprintf("preset-%d", idx+1)
		}
		if preset.Title == "" {
			preset.Title = preset.ID
		}
		if n, ok := seen[preset.ID]; ok {
			n++
			seen[preset.ID] = n
			preset.ID = fmt.Sprintf("%s-%d", preset.ID, n)
		} else {
			seen[preset.ID] = 1
		}
		out = append(out, preset)
	}
	return out
}

func presetExists(presets []FilterPreset, id string) bool {
	for _, preset := range presets {
		if preset.ID == id {
			return true
		}
	}
	return false
}

func samePath(left, right string) bool {
	la := filepath.Clean(left)
	ra := filepath.Clean(right)
	return la == ra
}

func boolPtr(value bool) *bool {
	return &value
}
