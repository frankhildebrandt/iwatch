package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/buffer"
)

// DetailView renders the selected log line with parsed fields.
type DetailView struct {
	line   *buffer.ViewLine
	scroll int
}

// NewDetailView creates an empty detail screen state.
func NewDetailView() *DetailView {
	return &DetailView{}
}

// Open sets the selected line and resets the scroll position.
func (d *DetailView) Open(line buffer.ViewLine) {
	copyLine := line
	d.line = &copyLine
	d.scroll = 0
}

// Close clears the current detail selection.
func (d *DetailView) Close() {
	d.line = nil
	d.scroll = 0
}

// View renders the detail screen.
func (d *DetailView) View(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("Log Details")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Render("Ausgewaehlte Logzeile mit Rohtext und geparsten Feldern")
	bodyHeight := max(5, height-1)
	content := d.renderContent(width, bodyHeight)
	help := "[click log line] open [wheel/j/k, pgup/pgdown, home] scroll [esc|enter] close [q] quit"
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, width, bodyHeight).Render(title+"\n"+subtitle+"\n\n"+content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
}

func (d *DetailView) renderContent(width, height int) string {
	if d.line == nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Render("No line selected.")
	}

	sections := []string{
		d.renderMetadataSection(width),
		d.renderRawSection(width),
		d.renderFieldsSection(width),
	}
	lines := strings.Split(strings.Join(sections, "\n\n"), "\n")
	maxVisible := max(1, height-6)
	if d.scroll >= len(lines) {
		d.scroll = max(0, len(lines)-1)
	}
	start := min(d.scroll, max(0, len(lines)-maxVisible))
	end := min(len(lines), start+maxVisible)
	return strings.Join(lines[start:end], "\n")
}

func (d *DetailView) renderMetadataSection(width int) string {
	line := d.line
	rows := []string{
		renderDetailMetaRow("Source", line.Source),
		renderDetailMetaRow("Time", line.Timestamp.Format(time.RFC3339)),
		renderDetailMetaRow("Session", fmt.Sprintf("%d", line.Session)),
		renderDetailMetaRow("Index", fmt.Sprintf("%d", line.Index)),
	}
	return renderDetailSection("Metadata", strings.Join(rows, "\n"), width)
}

func (d *DetailView) renderRawSection(width int) string {
	return renderDetailSection("Raw", d.line.Text, width)
}

func (d *DetailView) renderFieldsSection(width int) string {
	if len(d.line.RawFields) == 0 {
		return renderDetailSection("Fields", "Keine geparsten Felder vorhanden.", width)
	}

	keys := make([]string, 0, len(d.line.RawFields))
	for key := range d.line.RawFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var rows []string
	for _, key := range keys {
		value := prettyFieldValue(d.line.RawFields[key])
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153")).Render(key))
		rows = append(rows, indentLines(value, "  ")...)
	}
	return renderDetailSection("Fields", strings.Join(rows, "\n"), width)
}

func (d *DetailView) lines() []string {
	if d.line == nil {
		return nil
	}

	line := d.line
	lines := []string{
		fmt.Sprintf("source: %s", line.Source),
		fmt.Sprintf("time: %s", line.Timestamp.Format(time.RFC3339)),
		fmt.Sprintf("session: %d", line.Session),
		fmt.Sprintf("index: %d", line.Index),
		"",
		"raw:",
		line.Text,
	}

	if len(line.RawFields) == 0 {
		return lines
	}

	lines = append(lines, "", "fields:")
	keys := make([]string, 0, len(line.RawFields))
	for key := range line.RawFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := line.RawFields[key]
		lines = append(lines, fmt.Sprintf("%s:", key))
		lines = append(lines, indentLines(prettyFieldValue(value), "  ")...)
	}
	return lines
}

func prettyFieldValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	var out bytes.Buffer
	if json.Valid([]byte(trimmed)) {
		if err := json.Indent(&out, []byte(trimmed), "", "  "); err == nil {
			return out.String()
		}
	}
	return value
}

func indentLines(value string, indent string) []string {
	parts := strings.Split(value, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, indent+part)
	}
	return lines
}

func renderDetailMetaRow(label, value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Render(label+":") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(value)
}

func renderDetailSection(title, body string, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("24")).
		Padding(0, 1)

	bodyStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, true, true, false).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(max(20, width-8))

	return titleStyle.Render(title) + "\n" + bodyStyle.Render(body)
}
