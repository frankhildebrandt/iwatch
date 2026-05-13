package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type fieldFilterMenu struct {
	cursor       int
	viewportTop  int
	filter       string
	editingField string
}

func newFieldFilterMenu() *fieldFilterMenu {
	return &fieldFilterMenu{}
}

func (m *fieldFilterMenu) View(width, height int, observed []string, filters map[string]string) string {
	fields := m.FilteredFields(observed)
	if len(observed) == 0 {
		return paneStyle(true, min(width, 64), min(height, 8)).Render("Field Filter\n\nNoch keine logfmt-Felder erkannt.")
	}

	visibleRows := m.visibleRows(height)
	m.ensureVisible(len(fields), visibleRows)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Field Filter"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.subtitle(filters)),
		"",
	}
	if len(fields) == 0 {
		lines = append(lines, "  Keine Felder passen zum Filter.")
	} else {
		end := min(len(fields), m.viewportTop+visibleRows)
		for idx := m.viewportTop; idx < end; idx++ {
			field := fields[idx]
			value := filters[field]
			marker := "[ ]"
			if value != "" {
				marker = "[x]"
			}
			prefix := "  "
			if idx == m.cursor {
				prefix = "> "
			}
			suffix := ""
			if value != "" {
				suffix = fmt.Sprintf(" contains %q", value)
			}
			if field == m.editingField {
				suffix = fmt.Sprintf(" contains %q_", value)
			}
			lines = append(lines, fmt.Sprintf("%s%s %s%s", prefix, marker, field, suffix))
		}
	}
	if len(fields) > visibleRows {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("%d-%d/%d", m.viewportTop+1, min(len(fields), m.viewportTop+visibleRows), len(fields))))
	}

	boxWidth := min(width, 72)
	boxHeight := min(height, max(8, len(lines)+2))
	return paneStyle(true, boxWidth, boxHeight).Render(strings.Join(lines, "\n"))
}

func (m *fieldFilterMenu) InputBar() string {
	if m.editingField != "" {
		return lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Render("filter value> type contains [backspace] edit [ctrl+u] clear [esc] field list")
	}
	return lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Render("field filters> type field filter [j/k, arrows] nav [enter] edit contains [esc] close")
}

func (m *fieldFilterMenu) FilteredFields(fields []string) []string {
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

func (m *fieldFilterMenu) Move(delta int, total int, visibleRows int) {
	if total <= 0 {
		m.cursor = 0
		m.viewportTop = 0
		return
	}
	m.cursor += delta
	m.ensureVisible(total, visibleRows)
}

func (m *fieldFilterMenu) CurrentField(fields []string) (string, bool) {
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

func (m *fieldFilterMenu) TypeFieldFilter(value string) {
	if value == "" {
		return
	}
	m.filter += strings.ToLower(value)
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldFilterMenu) BackspaceFieldFilter() {
	if m.filter == "" {
		return
	}
	m.filter = m.filter[:len(m.filter)-1]
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldFilterMenu) ClearFieldFilter() {
	m.filter = ""
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldFilterMenu) StartEditing(field string) {
	m.editingField = field
}

func (m *fieldFilterMenu) StopEditing() {
	m.editingField = ""
}

func (m *fieldFilterMenu) Editing() bool {
	return m.editingField != ""
}

func (m *fieldFilterMenu) visibleRows(height int) int {
	return max(1, min(12, height-8))
}

func (m *fieldFilterMenu) subtitle(filters map[string]string) string {
	if m.editingField != "" {
		return "Wert fuer " + m.editingField + " bearbeiten"
	}
	label := "<alle>"
	if m.filter != "" {
		label = m.filter
	}
	active := countActiveFieldFilters(filters)
	return fmt.Sprintf("Feldfilter: %s | aktiv: %d", label, active)
}

func (m *fieldFilterMenu) ensureVisible(total int, visibleRows int) {
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
