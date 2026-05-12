package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stackriot/iwatch/internal/config"
)

func (a *App) activateEditorRow(row editorRow) (tea.Model, tea.Cmd) {
	switch row.kind {
	case editorSave:
		if err := config.Save(a.configPath, a.editor.draft); err != nil {
			a.eventsPane.Append("save config: " + err.Error())
			return a, nil
		}
		a.cfg = a.editor.draft
		a.mode = modeMain
		a.eventsPane.Append("saved config to " + a.configPath)
	case editorCancel:
		a.mode = modeMain
	case editorPresetSelect:
		preset := a.editor.draft.UI.Presets[row.index]
		a.editor.draft.UI.ActivePreset = preset.ID
	case editorPresetAdd:
		a.addPreset()
	case editorPresetDup:
		return a.duplicatePreset()
	case editorPresetDelete:
		return a.deleteActivePreset()
	case editorClauseAdd:
		active := activePresetPtr(&a.editor.draft)
		active.Clauses = append(active.Clauses, config.FilterClause{Conditions: []config.FilterCondition{{Value: "value"}}})
	case editorRuleAdd:
		active := activePresetPtr(&a.editor.draft)
		active.HighlightRules = append(active.HighlightRules, config.HighlightRule{ID: "rule", Style: "warn", Priority: 50, Pattern: "(?i)warn"})
	case editorToggleRaw:
		a.editor.draft.UI.LogView.ShowRawMessage = boolPtr(!boolValue(a.editor.draft.UI.LogView.ShowRawMessage))
	case editorToggleSource:
		a.editor.draft.UI.LogView.ShowSource = boolPtr(!boolValue(a.editor.draft.UI.LogView.ShowSource))
	case editorToggleTime:
		a.editor.draft.UI.LogView.ShowTimestamp = boolPtr(!boolValue(a.editor.draft.UI.LogView.ShowTimestamp))
	case editorTimeFormat:
		a.editor.draft.UI.LogView.TimeFormat = cycleValue(timeFormats, a.editor.draft.UI.LogView.TimeFormat, 1)
	case editorWrapMode:
		a.editor.draft.UI.LogView.WrapMode = cycleValue(wrapModes, a.editor.draft.UI.LogView.WrapMode, 1)
	default:
		return a.startEditingRow(row)
	}
	a.editor.draft = config.DefaultMerge(a.editor.draft)
	return a, nil
}

func (a *App) startEditingRow(row editorRow) (tea.Model, tea.Cmd) {
	switch row.kind {
	case editorPresetID, editorPresetTitle, editorClause, editorRule, editorFields:
		a.editor.editing = true
		a.editor.input.SetValue(row.value)
		a.editor.input.CursorEnd()
		a.editor.input.Focus()
	}
	return a, nil
}

func (a *App) stopEditing() {
	a.editor.editing = false
	a.editor.input.Blur()
}

func (a *App) applyEditorInput(row editorRow) {
	value := strings.TrimSpace(a.editor.input.Value())
	active := activePresetPtr(&a.editor.draft)

	switch row.kind {
	case editorPresetID:
		if value != "" {
			oldID := active.ID
			active.ID = value
			if a.editor.draft.UI.ActivePreset == oldID {
				a.editor.draft.UI.ActivePreset = value
			}
		}
	case editorPresetTitle:
		if value != "" {
			active.Title = value
		}
	case editorClause:
		clause := parseClause(value)
		if row.index >= 0 && row.index < len(active.Clauses) {
			active.Clauses[row.index] = clause
		}
	case editorRule:
		rule := parseRule(value)
		if row.index >= 0 && row.index < len(active.HighlightRules) {
			active.HighlightRules[row.index] = rule
		}
	case editorFields:
		a.editor.draft.UI.LogView.VisibleFields = parseFields(value)
	}
	a.editor.draft = config.DefaultMerge(a.editor.draft)
}

func (a *App) addEditorItem(row editorRow) (tea.Model, tea.Cmd) {
	switch row.kind {
	case editorPresetSelect, editorPresetAdd, editorPresetDup, editorPresetDelete:
		a.addPreset()
	case editorClause, editorClauseAdd:
		active := activePresetPtr(&a.editor.draft)
		active.Clauses = append(active.Clauses, config.FilterClause{Conditions: []config.FilterCondition{{Value: "value"}}})
	case editorRule, editorRuleAdd:
		active := activePresetPtr(&a.editor.draft)
		active.HighlightRules = append(active.HighlightRules, config.HighlightRule{ID: "rule", Style: "warn", Priority: 50, Pattern: "(?i)warn"})
	}
	a.editor.draft = config.DefaultMerge(a.editor.draft)
	return a, nil
}

func (a *App) deleteEditorItem(row editorRow) (tea.Model, tea.Cmd) {
	active := activePresetPtr(&a.editor.draft)
	switch row.kind {
	case editorClause:
		if row.index >= 0 && row.index < len(active.Clauses) {
			active.Clauses = append(active.Clauses[:row.index], active.Clauses[row.index+1:]...)
		}
	case editorRule:
		if row.index >= 0 && row.index < len(active.HighlightRules) {
			active.HighlightRules = append(active.HighlightRules[:row.index], active.HighlightRules[row.index+1:]...)
		}
	case editorPresetSelect, editorPresetDelete:
		return a.deleteActivePreset()
	}
	a.editor.draft = config.DefaultMerge(a.editor.draft)
	return a, nil
}

func (a *App) duplicatePreset() (tea.Model, tea.Cmd) {
	active := activePreset(a.editor.draft)
	copyPreset := active
	copyPreset.ID = active.ID + "-copy"
	copyPreset.Title = active.Title + " Copy"
	copyPreset.Clauses = append([]config.FilterClause(nil), active.Clauses...)
	copyPreset.HighlightRules = append([]config.HighlightRule(nil), active.HighlightRules...)
	a.editor.draft.UI.Presets = append(a.editor.draft.UI.Presets, copyPreset)
	a.editor.draft.UI.ActivePreset = copyPreset.ID
	a.editor.draft = config.DefaultMerge(a.editor.draft)
	return a, nil
}

func (a *App) deleteActivePreset() (tea.Model, tea.Cmd) {
	if len(a.editor.draft.UI.Presets) <= 1 {
		return a, nil
	}
	activeID := a.editor.draft.UI.ActivePreset
	nextPresets := make([]config.FilterPreset, 0, len(a.editor.draft.UI.Presets)-1)
	for _, preset := range a.editor.draft.UI.Presets {
		if preset.ID != activeID {
			nextPresets = append(nextPresets, preset)
		}
	}
	a.editor.draft.UI.Presets = nextPresets
	a.editor.draft.UI.ActivePreset = nextPresets[0].ID
	a.editor.draft = config.DefaultMerge(a.editor.draft)
	return a, nil
}

func (a *App) addPreset() {
	next := len(a.editor.draft.UI.Presets) + 1
	preset := config.FilterPreset{
		ID:    fmt.Sprintf("preset-%d", next),
		Title: fmt.Sprintf("Preset %d", next),
	}
	a.editor.draft.UI.Presets = append(a.editor.draft.UI.Presets, preset)
	a.editor.draft.UI.ActivePreset = preset.ID
}
