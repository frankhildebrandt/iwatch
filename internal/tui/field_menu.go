package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/config"
)

type fieldMenu struct {
	cursor      int
	viewportTop int
	filter      string
}

func newFieldMenu() *fieldMenu {
	return &fieldMenu{}
}

func (m *fieldMenu) View(width, height int, observed []string, view config.LogViewConfig) string {
	fields := m.FilteredFields(orderedObservedFields(view, observed))
	hidden := hiddenFieldSet(view)

	if len(observed) == 0 {
		return paneStyle(true, min(width, 48), min(height, 8)).Render("Field Menu\n\nNoch keine logfmt-Felder erkannt.")
	}

	visibleRows := m.visibleRows(height)
	m.ensureVisible(len(fields), visibleRows)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Field Menu"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Filter: " + m.filterLabel()),
		"",
	}
	if len(fields) == 0 {
		lines = append(lines, "  Keine Felder passen zum Filter.")
	} else {
		end := min(len(fields), m.viewportTop+visibleRows)
		for idx := m.viewportTop; idx < end; idx++ {
			field := fields[idx]
			marker := "[x]"
			if _, ok := hidden[field]; ok {
				marker = "[ ]"
			}
			prefix := "  "
			if idx == m.cursor {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", prefix, marker, field))
		}
	}
	if len(fields) > visibleRows {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("%d-%d/%d", m.viewportTop+1, min(len(fields), m.viewportTop+visibleRows), len(fields))))
	}

	boxWidth := min(width, 56)
	boxHeight := min(height, max(8, len(lines)+2))
	return paneStyle(true, boxWidth, boxHeight).Render(strings.Join(lines, "\n"))
}

func (m *fieldMenu) InputBar() string {
	return lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Render("fields> type filter [backspace] edit [j/k, arrows] nav [space/enter] toggle [esc] close")
}

func (m *fieldMenu) FilteredFields(fields []string) []string {
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

func (m *fieldMenu) Move(delta int, total int, visibleRows int) {
	if total <= 0 {
		m.cursor = 0
		m.viewportTop = 0
		return
	}
	m.cursor += delta
	m.ensureVisible(total, visibleRows)
}

func (m *fieldMenu) CurrentField(fields []string) (string, bool) {
	filtered := m.FilteredFields(fields)
	if len(filtered) == 0 {
		return "", false
	}
	m.ensureVisible(len(filtered), 1)
	return filtered[m.cursor], true
}

func (m *fieldMenu) Type(value string) {
	if value == "" {
		return
	}
	m.filter += strings.ToLower(value)
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldMenu) Backspace() {
	if m.filter == "" {
		return
	}
	m.filter = m.filter[:len(m.filter)-1]
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldMenu) ClearFilter() {
	m.filter = ""
	m.cursor = 0
	m.viewportTop = 0
}

func (m *fieldMenu) visibleRows(height int) int {
	return max(1, min(12, height-8))
}

func (m *fieldMenu) filterLabel() string {
	if m.filter == "" {
		return "<alle>"
	}
	return m.filter
}

func (m *fieldMenu) ensureVisible(total int, visibleRows int) {
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
