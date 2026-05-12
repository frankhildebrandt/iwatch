package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/stackriot/iwatch/internal/config"
)

type Command struct {
	ID     string
	Title  string
	Cmd    string
	CWD    string
	Source string
}

type Result struct {
	Commands []Command
	Default  string
}

func Discover(baseDir string, cfg config.Config) (Result, error) {
	commands := map[string]Command{}
	order := []string{}

	add := func(cmd Command) {
		if cmd.ID == "" || cmd.Cmd == "" {
			return
		}
		if _, exists := commands[cmd.ID]; exists {
			return
		}
		commands[cmd.ID] = cmd
		order = append(order, cmd.ID)
	}

	for _, cmd := range cfg.Commands {
		add(Command{
			ID:     cmd.ID,
			Title:  defaultTitle(cmd.Title, cmd.ID),
			Cmd:    cmd.Cmd,
			CWD:    pick(cmd.CWD, baseDir),
			Source: pick(cmd.Source, "config"),
		})
	}

	if packageCommands, defaultID, err := discoverPackageJSON(baseDir); err == nil {
		for _, cmd := range packageCommands {
			add(cmd)
		}
		if cfg.DefaultCommand == "" && defaultID != "" {
			cfg.DefaultCommand = defaultID
		}
	} else if !errorsIsNotExist(err) {
		return Result{}, err
	}

	if makeCommands, defaultID, err := discoverMakefile(baseDir); err == nil {
		for _, cmd := range makeCommands {
			add(cmd)
		}
		if cfg.DefaultCommand == "" && defaultID != "" {
			cfg.DefaultCommand = defaultID
		}
	} else if !errorsIsNotExist(err) {
		return Result{}, err
	}

	res := Result{}
	for _, id := range order {
		res.Commands = append(res.Commands, commands[id])
	}
	res.Default = cfg.DefaultCommand
	if res.Default == "" && len(res.Commands) > 0 {
		res.Default = res.Commands[0].ID
	}
	return res, nil
}

func discoverPackageJSON(baseDir string) ([]Command, string, error) {
	type packageJSON struct {
		Scripts map[string]string `json:"scripts"`
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "package.json"))
	if err != nil {
		return nil, "", err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, "", fmt.Errorf("parse package.json: %w", err)
	}

	var commands []Command
	keys := make([]string, 0, len(pkg.Scripts))
	for key := range pkg.Scripts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		commands = append(commands, Command{
			ID:     "npm:" + key,
			Title:  "npm " + key,
			Cmd:    "npm run " + key,
			CWD:    baseDir,
			Source: "package.json",
		})
	}

	for _, preferred := range []string{"dev", "start", "build"} {
		if _, ok := pkg.Scripts[preferred]; ok {
			return commands, "npm:" + preferred, nil
		}
	}
	if len(commands) == 0 {
		return nil, "", nil
	}
	return commands, commands[0].ID, nil
}

func discoverMakefile(baseDir string) ([]Command, string, error) {
	makefile := filepath.Join(baseDir, "Makefile")
	data, err := os.ReadFile(makefile)
	if err != nil {
		return nil, "", err
	}

	re := regexp.MustCompile(`^([A-Za-z0-9_.-]+):`)
	lines := strings.Split(string(data), "\n")
	var targets []string
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		match := re.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		target := match[1]
		if strings.HasPrefix(target, ".") {
			continue
		}
		targets = append(targets, target)
	}

	seen := map[string]struct{}{}
	var commands []Command
	for _, target := range targets {
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		commands = append(commands, Command{
			ID:     "make:" + target,
			Title:  "make " + target,
			Cmd:    "make " + target,
			CWD:    baseDir,
			Source: "Makefile",
		})
	}

	defaultID := ""
	for _, preferred := range []string{"run", "dev", "build"} {
		if _, ok := seen[preferred]; ok {
			defaultID = "make:" + preferred
			break
		}
	}
	if defaultID == "" && len(commands) > 0 {
		defaultID = commands[0].ID
	}
	return commands, defaultID, nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func pick(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func defaultTitle(title, fallback string) string {
	if title != "" {
		return title
	}
	return fallback
}
