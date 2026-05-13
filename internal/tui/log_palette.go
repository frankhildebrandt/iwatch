package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/config"
)

type logPalette struct {
	timeColor  lipgloss.Color
	keyColor   lipgloss.Color
	valueColor lipgloss.Color
}

func paletteForLogView(view config.LogViewConfig) logPalette {
	switch view.Palette {
	case "contrast":
		return logPalette{timeColor: lipgloss.Color("244"), keyColor: lipgloss.Color("14"), valueColor: lipgloss.Color("231")}
	case "ocean":
		return logPalette{timeColor: lipgloss.Color("67"), keyColor: lipgloss.Color("110"), valueColor: lipgloss.Color("195")}
	case "forest":
		return logPalette{timeColor: lipgloss.Color("64"), keyColor: lipgloss.Color("107"), valueColor: lipgloss.Color("193")}
	case "ember":
		return logPalette{timeColor: lipgloss.Color("130"), keyColor: lipgloss.Color("208"), valueColor: lipgloss.Color("230")}
	default:
		return logPalette{timeColor: lipgloss.Color("240"), keyColor: lipgloss.Color("245"), valueColor: lipgloss.Color("255")}
	}
}

func (p logPalette) renderTime(value string) string {
	return lipgloss.NewStyle().Foreground(p.timeColor).Render(value)
}

func (p logPalette) renderKey(value string) string {
	return lipgloss.NewStyle().Foreground(p.keyColor).Render(value)
}

func (p logPalette) renderValue(value string) string {
	return lipgloss.NewStyle().Foreground(p.valueColor).Render(value)
}
