package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
)

// AutomationEngine evaluates configured triggers on new lines.
type AutomationEngine struct {
	automations []compiledAutomation
}

type compiledAutomation struct {
	id      string
	trigger compiledTrigger
	actions []config.AutomationAction
}

type compiledTrigger struct {
	terms []bufferQueryTerm
	re    *regexp.Regexp
}

type bufferQueryTerm struct {
	key   string
	value string
	text  string
}

func NewAutomationEngine(cfg []config.AutomationConfig) *AutomationEngine {
	engine := &AutomationEngine{}
	engine.Configure(cfg)
	return engine
}

func (e *AutomationEngine) Configure(cfg []config.AutomationConfig) {
	compiled := make([]compiledAutomation, 0, len(cfg))
	for _, a := range cfg {
		if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Trigger) == "" {
			continue
		}
		compiled = append(compiled, compiledAutomation{
			id:      a.ID,
			trigger: compileTrigger(a),
			actions: append([]config.AutomationAction(nil), a.Actions...),
		})
	}
	e.automations = compiled
}

func compileTrigger(a config.AutomationConfig) compiledTrigger {
	trigger := compiledTrigger{}
	if a.Regex != "" {
		if re, err := regexp.Compile(a.Regex); err == nil {
			trigger.re = re
		}
	}
	trigger.terms = parseQueryTerms(a.Trigger)
	return trigger
}

func parseQueryTerms(raw string) []bufferQueryTerm {
	tokens := splitTokens(raw)
	terms := make([]bufferQueryTerm, 0, len(tokens))
	for _, token := range tokens {
		if key, value, ok := strings.Cut(token, "="); ok && key != "" {
			terms = append(terms, bufferQueryTerm{key: strings.ToLower(key), value: strings.ToLower(value)})
			continue
		}
		terms = append(terms, bufferQueryTerm{text: strings.ToLower(token)})
	}
	return terms
}

func (t compiledTrigger) matches(line buffer.Line) bool {
	if t.re != nil && t.re.MatchString(line.Text) {
		return true
	}
	for _, term := range t.terms {
		if !termMatches(term, line) {
			return false
		}
	}
	return len(t.terms) > 0
}

func termMatches(term bufferQueryTerm, line buffer.Line) bool {
	if term.key != "" {
		value, ok := line.Fields[term.key]
		return ok && strings.Contains(value, term.value)
	}
	if term.text == "" {
		return true
	}
	return strings.Contains(line.Plain, term.text)
}

func (e *AutomationEngine) Apply(app *App, line buffer.Line) {
	for _, a := range e.automations {
		if !a.trigger.matches(line) {
			continue
		}
		for _, action := range a.actions {
			e.applyAction(app, a.id, action, line)
		}
	}
}

func (e *AutomationEngine) applyAction(app *App, automationID string, action config.AutomationAction, line buffer.Line) {
	switch action.Type {
	case "event":
		msg := strings.TrimSpace(action.Message)
		if msg == "" {
			msg = "automation " + automationID + " matched"
		}
		app.eventsPane.Append(fmt.Sprintf("%s | %s: %s", automationID, line.Source, msg))
	default:
		// unknown action type -> ignore
	}
}

