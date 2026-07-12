package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type groupMenu struct {
	cursor      int
	viewportTop int
	filter      string
}

func newGroupMenu() *groupMenu {
	return &groupMenu{}
}

func (m *groupMenu) View(width, height int, observed []string, activeField string) string {
	fields := m.FilteredFields(observed)
	if len(observed) == 0 {
		return paneStyle(true, min(width, 64), min(height, 8)).Render("Group By\n\nNoch keine logfmt-Felder erkannt.")
	}

	visibleRows := m.visibleRows(height)
	m.ensureVisible(len(fields), visibleRows)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Group By"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.subtitle(activeField)),
		"",
	}
	if len(fields) == 0 {
		lines = append(lines, "  Keine Felder passen zum Filter.")
	} else {
		end := min(len(fields), m.viewportTop+visibleRows)
		for idx := m.viewportTop; idx < end; idx++ {
			field := fields[idx]
			marker := "   "
			if field == activeField {
				marker = " * "
			}
			prefix := "  "
			if idx == m.cursor {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s%s", prefix, marker, field))
		}
	}
	if len(fields) > visibleRows {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("%d-%d/%d", m.viewportTop+1, min(len(fields), m.viewportTop+visibleRows), len(fields))))
	}

	boxWidth := min(width, 72)
	boxHeight := min(height, max(8, len(lines)+2))
	return paneStyle(true, boxWidth, boxHeight).Render(strings.Join(lines, "\n"))
}

func (m *groupMenu) InputBar() string {
	return lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Render("group> type filter [j/k, arrows] nav [space/enter] select [esc] close")
}

func (m *groupMenu) FilteredFields(fields []string) []string {
	if m.filter == "" {
		return fields
	}

	out := make([]string, 0, len(fields))
	filter := strings.ToLower(m.filter)
	for _, field := range fields {
		if strings.Contains(field, filter) {
			out = append(out, field)
		}
	}
	return out
}

func (m *groupMenu) Move(delta int, total int, visibleRows int) {
	if total <= 0 {
		m.cursor = 0
		m.viewportTop = 0
		return
	}
	m.cursor += delta
	m.ensureVisible(total, visibleRows)
}

func (m *groupMenu) CurrentField(fields []string) (string, bool) {
	filtered := m.FilteredFields(fields)
	if len(filtered) == 0 {
		return "", false
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(filtered) {
		m.cursor = len(filtered) - 1
	}
	return filtered[m.cursor], true
}

func (m *groupMenu) TypeFilter(value string) {
	if value == "" {
		return
	}
	m.filter += strings.ToLower(value)
	m.cursor = 0
	m.viewportTop = 0
}

func (m *groupMenu) Backspace() {
	if m.filter == "" {
		return
	}
	m.filter = m.filter[:len(m.filter)-1]
	m.cursor = 0
	m.viewportTop = 0
}

func (m *groupMenu) ClearFilter() {
	m.filter = ""
	m.cursor = 0
	m.viewportTop = 0
}

func (m *groupMenu) visibleRows(height int) int {
	return max(1, min(12, height-8))
}

func (m *groupMenu) subtitle(activeField string) string {
	label := "<alle>"
	if m.filter != "" {
		label = m.filter
	}
	return fmt.Sprintf("Gruppierung: %s | aktiv: %s", label, activeField)
}

func (m *groupMenu) ensureVisible(total int, visibleRows int) {
	if total <= 0 {
		m.cursor = 0
		m.viewportTop = 0
		return
	}
	if visibleRows <= 0 {
		visibleRows = 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= total {
		m.cursor = total - 1
	}
	if m.cursor < m.viewportTop {
		m.viewportTop = m.cursor
	}
	if m.cursor >= m.viewportTop+visibleRows {
		m.viewportTop = m.cursor - visibleRows + 1
	}
	maxTop := max(0, total-visibleRows)
	if m.viewportTop > maxTop {
		m.viewportTop = maxTop
	}
	if m.viewportTop < 0 {
		m.viewportTop = 0
	}
}
