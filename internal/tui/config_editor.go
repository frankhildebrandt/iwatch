package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/config"
)

type editorKind string

const (
	editorSave          editorKind = "save"
	editorCancel        editorKind = "cancel"
	editorPresetSelect  editorKind = "preset-select"
	editorPresetAdd     editorKind = "preset-add"
	editorPresetDup     editorKind = "preset-dup"
	editorPresetDelete  editorKind = "preset-delete"
	editorPresetID      editorKind = "preset-id"
	editorPresetTitle   editorKind = "preset-title"
	editorPresetStreams editorKind = "preset-streams"
	editorClause        editorKind = "clause"
	editorClauseAdd     editorKind = "clause-add"
	editorRule          editorKind = "rule"
	editorRuleAdd       editorKind = "rule-add"
	editorStream        editorKind = "stream"
	editorStreamAdd     editorKind = "stream-add"
	editorFields        editorKind = "fields"
	editorHiddenFields  editorKind = "hidden-fields"
	editorToggleRaw     editorKind = "toggle-raw"
	editorToggleSource  editorKind = "toggle-source"
	editorToggleTime    editorKind = "toggle-time"
	editorTimeFormat    editorKind = "time-format"
	editorWrapMode      editorKind = "wrap-mode"
	editorPalette       editorKind = "palette"
)

// ConfigEditor owns the fullscreen config editing workflow.
type ConfigEditor struct {
	configPath string
	draft      config.Config
	selected   int
	editing    bool
	input      textinput.Model
}

type editorRow struct {
	section string
	kind    editorKind
	label   string
	value   string
	index   int
}

// NewConfigEditor creates the editor state.
func NewConfigEditor(configPath string) *ConfigEditor {
	input := textinput.New()
	input.CharLimit = 512
	return &ConfigEditor{
		configPath: configPath,
		input:      input,
	}
}

// Open clones the app config into the editor draft state.
func (e *ConfigEditor) Open(cfg config.Config) {
	e.draft = config.Clone(cfg)
	e.selected = 0
	e.editing = false
	e.input.SetValue("")
}

// View renders the fullscreen config editor.
func (e *ConfigEditor) View(width, height, pageSize int) string {
	rows := e.rows()
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Config Editor")
	subtitle := "Sections: Presets, Filter, Highlight, Log fields, Display"
	lines := []string{title, subtitle, ""}

	currentSection := ""
	for idx, row := range rows {
		if row.section != currentSection {
			currentSection = row.section
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render(currentSection))
		}
		prefix := "  "
		if idx == e.selected {
			prefix = "> "
		}
		value := row.value
		if e.editing && idx == e.selected {
			value = e.input.View()
		}
		lines = append(lines, fmt.Sprintf("%s%s: %s", prefix, row.label, value))
	}

	help := "[up/down] move [enter] toggle/action [e] edit [a] add [d] delete [y] duplicate preset [saverow] save [cancelrow/esc] leave"
	content := strings.Join(lines, "\n")
	bodyHeight := max(5, height-1)
	_ = pageSize
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, width, bodyHeight).Render(content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
}

func (e *ConfigEditor) rows() []editorRow {
	draft := e.draft
	rows := []editorRow{
		{section: "Actions", kind: editorSave, label: "Save changes", value: e.configPath},
		{section: "Actions", kind: editorCancel, label: "Cancel changes", value: "discard draft"},
	}

	for idx, preset := range draft.UI.Presets {
		label := preset.Title
		if preset.Title == "" {
			label = preset.ID
		}
		value := preset.ID
		if preset.ID == draft.UI.ActivePreset {
			value += " (active)"
		}
		rows = append(rows, editorRow{section: "Presets", kind: editorPresetSelect, label: label, value: value, index: idx})
	}
	rows = append(rows,
		editorRow{section: "Presets", kind: editorPresetAdd, label: "Add preset", value: "new preset"},
		editorRow{section: "Presets", kind: editorPresetDup, label: "Duplicate active preset", value: "copy filters + highlights"},
		editorRow{section: "Presets", kind: editorPresetDelete, label: "Delete active preset", value: draft.UI.ActivePreset},
	)

	active := activePreset(draft)
	rows = append(rows,
		editorRow{section: "Filter", kind: editorPresetID, label: "Preset ID", value: active.ID},
		editorRow{section: "Filter", kind: editorPresetTitle, label: "Preset title", value: active.Title},
		editorRow{section: "Filter", kind: editorPresetStreams, label: "Active streams", value: strings.Join(active.Streams, ", ")},
	)
	for idx, clause := range active.Clauses {
		rows = append(rows, editorRow{section: "Filter", kind: editorClause, label: fmt.Sprintf("OR clause %d", idx+1), value: formatClause(clause), index: idx})
	}
	rows = append(rows, editorRow{section: "Filter", kind: editorClauseAdd, label: "Add OR clause", value: "field=value text"})

	for idx, rule := range active.HighlightRules {
		rows = append(rows, editorRow{section: "Highlight", kind: editorRule, label: fmt.Sprintf("Rule %d", idx+1), value: formatRule(rule), index: idx})
	}
	rows = append(rows, editorRow{section: "Highlight", kind: editorRuleAdd, label: "Add highlight rule", value: "id|style|priority|pattern"})

	for idx, stream := range draft.Streams {
		rows = append(rows, editorRow{section: "Streams", kind: editorStream, label: fmt.Sprintf("Stream %d", idx+1), value: formatStream(stream), index: idx})
	}
	rows = append(rows, editorRow{section: "Streams", kind: editorStreamAdd, label: "Add stream", value: "id|title|type|role|source|cmd|cwd|enabled|autoStart"})

	rows = append(rows,
		editorRow{section: "Log fields", kind: editorFields, label: "Visible fields", value: strings.Join(draft.UI.LogView.VisibleFields, ", ")},
		editorRow{section: "Log fields", kind: editorHiddenFields, label: "Hidden fields", value: strings.Join(draft.UI.LogView.HiddenFields, ", ")},
		editorRow{section: "Log fields", kind: editorToggleRaw, label: "Show raw message", value: fmt.Sprintf("%t", boolValue(draft.UI.LogView.ShowRawMessage))},
		editorRow{section: "Log fields", kind: editorToggleSource, label: "Show source", value: fmt.Sprintf("%t", boolValue(draft.UI.LogView.ShowSource))},
		editorRow{section: "Log fields", kind: editorToggleTime, label: "Show timestamp", value: fmt.Sprintf("%t", boolValue(draft.UI.LogView.ShowTimestamp))},
		editorRow{section: "Display", kind: editorTimeFormat, label: "Time format", value: draft.UI.LogView.TimeFormat},
		editorRow{section: "Display", kind: editorWrapMode, label: "Wrap mode", value: draft.UI.LogView.WrapMode},
		editorRow{section: "Display", kind: editorPalette, label: "Palette", value: draft.UI.LogView.Palette},
	)

	return rows
}
