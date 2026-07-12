package tui

import "github.com/charmbracelet/lipgloss"

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
	content := renderKeymapHelp()
	help := "[esc|?|q] close"
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, width, bodyHeight).Render(title+"\n"+subtitle+"\n\n"+content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
}
