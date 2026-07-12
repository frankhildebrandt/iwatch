package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ShareView renders a copy/export friendly snapshot of logs.
type ShareView struct {
	contents string
	status   string
	path     string
	copied   bool
	err      error
	scroll   int
}

// NewShareView creates an empty share screen state.
func NewShareView() *ShareView {
	return &ShareView{scroll: 0}
}

func (s *ShareView) Open(contents string) {
	s.contents = contents
	s.status = ""
	s.path = ""
	s.copied = false
	s.err = nil
	s.scroll = 0
}

func (s *ShareView) Close() {
	s.contents = ""
	s.status = ""
	s.path = ""
	s.copied = false
	s.err = nil
	s.scroll = 0
}

func (s *ShareView) ApplyResult(msg shareResultMsg) {
	s.copied = msg.copied
	s.path = msg.path
	s.err = msg.err
	if msg.contents != "" {
		s.contents = msg.contents
	}
	if msg.err != nil {
		s.status = "error: " + msg.err.Error()
		return
	}
	if msg.copied {
		s.status = "copied to clipboard"
		return
	}
	if msg.path != "" {
		s.status = "exported to " + msg.path
		return
	}
	s.status = "done"
}

func (s *ShareView) View(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("Share")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Render("Copy/paste snippet for agents or issues")
	bodyHeight := max(5, height-1)

	status := s.status
	if status == "" {
		status = "ready"
	}
	if s.path != "" {
		status += " | file: " + s.path
	}
	header := title + "\n" + subtitle + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(status)

	content := s.renderContent(width, bodyHeight)
	help := "[y] copy [s] export [j/k, pgup/pgdown, home] scroll [esc|enter] close [q] quit"
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, width, bodyHeight).Render(header+"\n\n"+content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
}

func (s *ShareView) renderContent(width, height int) string {
	if s.contents == "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Render("No share contents.")
	}
	lines := strings.Split(s.contents, "\n")
	maxVisible := max(1, height-6)
	if s.scroll >= len(lines) {
		s.scroll = max(0, len(lines)-1)
	}
	start := min(s.scroll, max(0, len(lines)-maxVisible))
	end := min(len(lines), start+maxVisible)
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, true, true, true).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(max(20, width-6)).
		Render(strings.Join(lines[start:end], "\n"))
	return box
}

func shareCopyCmd(contents string) tea.Cmd {
	return func() tea.Msg {
		if err := osc52Copy(contents); err != nil {
			return shareResultMsg{err: err}
		}
		return shareResultMsg{copied: true}
	}
}

func shareExportCmd(basePath string, contents string) tea.Cmd {
	return func() tea.Msg {
		path, err := exportShareFile(basePath, contents)
		if err != nil {
			return shareResultMsg{err: err}
		}
		return shareResultMsg{path: path}
	}
}

func exportShareFile(configPath string, contents string) (string, error) {
	baseDir := filepath.Dir(configPath)
	dir := filepath.Join(baseDir, "share")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create share dir: %w", err)
	}
	name := fmt.Sprintf("share_%s.txt", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write share file: %w", err)
	}
	return path, nil
}

func osc52Copy(contents string) error {
	// Keep payload bounded to avoid giant terminal escape sequences.
	const maxBytes = 200_000
	if len(contents) > maxBytes {
		contents = contents[:maxBytes] + "\n…(truncated)\n"
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(contents))
	seq := "\x1b]52;c;" + encoded + "\x07"
	if _, err := os.Stdout.WriteString(seq); err != nil {
		return fmt.Errorf("osc52 copy: %w", err)
	}
	return nil
}

