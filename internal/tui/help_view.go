package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpView renders the full keymap help screen.
type HelpView struct{}

// NewHelpView creates the keymap help screen state.
func NewHelpView() *HelpView {
	return &HelpView{}
}

// View renders the keymap help screen.
func (h *HelpView) View(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("Keymap")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Render("Vollstaendige Tastaturbelegung")
	bodyHeight := max(5, height-1)
	content := h.renderContent()
	help := "[esc|?|q] close"
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, width, bodyHeight).Render(title+"\n"+subtitle+"\n\n"+content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
}

func (h *HelpView) renderContent() string {
	rows := []string{
		"Navigation",
		"  j/k, arrows                  Log scrollen oder Listen bewegen",
		"  pgup/pgdown, ctrl+u/ctrl+d   Seitenweise scrollen",
		"  home/end, G                  An Anfang oder Ende springen",
		"  n/N                          Naechster oder vorheriger Treffer",
		"",
		"Log",
		"  enter                        Tail oder Details fuer gewaehlte Zeile",
		"  enter enter                  Visuellen Trenner einfuegen",
		"  t                            Logbuffer leeren",
		"  r                            Aktives Command neu starten",
		"",
		"Panes und Ansichten",
		"  c                            Command-Palette oeffnen",
		"  w                            Watch-Events-Pane oeffnen",
		"  l                            Streams-Pane oeffnen",
		"  v                            Logfelder ein- oder ausblenden",
		"  F                            Live-Feldfilter oeffnen",
		"  g                            Config-Editor oeffnen",
		"  tab                          Fokus zwischen offenen Panes wechseln",
		"  o                            Stream-Details oeffnen",
		"",
		"Suche und Layout",
		"  /, f                         Bottom-Query oeffnen",
		"  [, ], left, right            Preset wechseln",
		"  S                            Split-Richtung wechseln",
		"",
		"App",
		"  ?                            Diese Hilfe oeffnen",
		"  esc                          Auswahl, Dialog oder Hilfe schliessen",
		"  q                            App beenden",
		"  esc esc esc                  App beenden",
	}
	return strings.Join(rows, "\n")
}
