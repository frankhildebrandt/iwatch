package app

import (
	"strings"

	"github.com/stackriot/iwatch/internal/detect"
)

func resolvePositionalCommand(baseDir string, commands []detect.Command, defaultCommand string, args []string) ([]detect.Command, string) {
	if len(args) == 0 {
		return commands, defaultCommand
	}

	commandLine := strings.Join(args, " ")
	if matchedID, ok := matchDiscoveredCommand(commands, commandLine, args); ok {
		return commands, matchedID
	}

	commandID := "cli:" + commandLine
	return append(commands, detect.Command{
		ID:     commandID,
		Title:  commandLine,
		Cmd:    commandLine,
		CWD:    baseDir,
		Source: "cli",
	}), commandID
}

func matchDiscoveredCommand(commands []detect.Command, commandLine string, args []string) (string, bool) {
	for _, command := range commands {
		if command.ID == commandLine || command.Cmd == commandLine || command.Title == commandLine {
			return command.ID, true
		}
	}

	if len(args) != 2 {
		return "", false
	}

	switch args[0] {
	case "make":
		return matchCommandID(commands, "make:"+args[1])
	case "npm":
		return matchCommandID(commands, "npm:"+args[1])
	}

	return "", false
}

func matchCommandID(commands []detect.Command, commandID string) (string, bool) {
	for _, command := range commands {
		if command.ID == commandID {
			return command.ID, true
		}
	}
	return "", false
}
