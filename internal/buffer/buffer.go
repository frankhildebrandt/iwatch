package buffer

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/stackriot/iwatch/internal/config"
)

// Line stores one buffered log line and its parsed metadata.
type Line struct {
	Index     int
	Session   int
	Source    string
	Text      string
	Plain     string
	Fields    map[string]string
	RawFields map[string]string
	Timestamp time.Time
}

// ViewLine decorates a buffered line with view-specific metadata.
type ViewLine struct {
	Line
	Matched       bool
	HighlightRule string
}

// SnapshotOptions describes how visible lines should be filtered and highlighted.
type SnapshotOptions struct {
	Preset     config.FilterPreset
	Query      string
	Highlights []config.HighlightRule
}

// LogBuffer keeps a bounded in-memory history of log lines.
type LogBuffer struct {
	mu         sync.RWMutex
	capacity   int
	lines      []Line
	nextIdx    int
	session    int
	baseRules  []config.HighlightRule
	compiledBy string
	compiled   []compiledRule
}

type compiledRule struct {
	id       string
	pattern  *regexp.Regexp
	style    string
	priority int
}

// New creates a log buffer with compiled highlight rules.
func New(capacity int, rules []config.HighlightRule) (*LogBuffer, error) {
	if capacity <= 0 {
		capacity = config.DefaultBufferLines
	}
	if _, err := compileRules(rules); err != nil {
		return nil, err
	}

	return &LogBuffer{
		capacity:  capacity,
		lines:     make([]Line, 0, min(capacity, 1024)),
		baseRules: append([]config.HighlightRule(nil), rules...),
	}, nil
}

// StartSession inserts a session marker and advances the session counter.
func (b *LogBuffer) StartSession(title string) {
	b.Append("system", "=== "+title+" ===")
	b.mu.Lock()
	b.session++
	b.mu.Unlock()
}

// Append stores a new log line in the ring buffer.
func (b *LogBuffer) Append(source, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	clean := stripANSI(text)
	fields, rawFields := parseLogfmtFields(clean)
	line := Line{
		Index:     b.nextIdx,
		Session:   b.session,
		Source:    source,
		Text:      text,
		Plain:     strings.ToLower(clean),
		Fields:    fields,
		RawFields: rawFields,
		Timestamp: time.Now(),
	}
	b.nextIdx++

	if len(b.lines) == b.capacity {
		copy(b.lines, b.lines[1:])
		b.lines[len(b.lines)-1] = line
		return
	}

	b.lines = append(b.lines, line)
}

// Snapshot returns the currently visible lines for the supplied query and preset.
func (b *LogBuffer) Snapshot(opts SnapshotOptions) []ViewLine {
	presetQuery := parsePreset(opts.Preset)
	runtimeQuery := parseQuery(opts.Query)
	rules := opts.Highlights
	if len(rules) == 0 {
		rules = b.baseRules
	}
	compiled, _ := b.rulesFor(rules)

	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []ViewLine
	for _, line := range b.lines {
		if !presetQuery.matches(line) || !runtimeQuery.matches(line) {
			continue
		}
		view := ViewLine{Line: line}
		view.HighlightRule = matchRule(compiled, line.Text)
		out = append(out, view)
	}
	return out
}

// Len returns the number of buffered lines.
func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

func (b *LogBuffer) rulesFor(rules []config.HighlightRule) ([]compiledRule, error) {
	key := rulesKey(rules)

	b.mu.Lock()
	defer b.mu.Unlock()

	if key == b.compiledBy {
		return b.compiled, nil
	}

	compiled, err := compileRules(rules)
	if err != nil {
		return nil, err
	}
	b.compiledBy = key
	b.compiled = compiled
	return b.compiled, nil
}

func compileRules(rules []config.HighlightRule) ([]compiledRule, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiledRule{
			id:       rule.ID,
			pattern:  re,
			style:    rule.Style,
			priority: rule.Priority,
		})
	}
	return compiled, nil
}

func matchRule(rules []compiledRule, text string) string {
	selected := ""
	priority := -1 << 30
	for _, rule := range rules {
		if rule.pattern.MatchString(text) && rule.priority >= priority {
			selected = rule.style
			priority = rule.priority
		}
	}
	return selected
}

func rulesKey(rules []config.HighlightRule) string {
	var builder strings.Builder
	for _, rule := range rules {
		builder.WriteString(rule.ID)
		builder.WriteByte('|')
		builder.WriteString(rule.Pattern)
		builder.WriteByte('|')
		builder.WriteString(rule.Style)
		builder.WriteByte('|')
		builder.WriteString(strconv.Itoa(rule.Priority))
		builder.WriteByte('\n')
	}
	return builder.String()
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

type parsedPreset struct {
	clauses []parsedClause
}

type parsedClause struct {
	conditions []queryTerm
}

type parsedQuery struct {
	terms []queryTerm
}

type queryTerm struct {
	key   string
	value string
	text  string
}

func parsePreset(preset config.FilterPreset) parsedPreset {
	clauses := make([]parsedClause, 0, len(preset.Clauses))
	for _, clause := range preset.Clauses {
		conditions := make([]queryTerm, 0, len(clause.Conditions))
		for _, cond := range clause.Conditions {
			term := queryTerm{value: strings.ToLower(cond.Value)}
			if cond.Field != "" {
				term.key = strings.ToLower(cond.Field)
			} else {
				term.text = strings.ToLower(cond.Value)
			}
			conditions = append(conditions, term)
		}
		clauses = append(clauses, parsedClause{conditions: conditions})
	}
	return parsedPreset{clauses: clauses}
}

func (p parsedPreset) matches(line Line) bool {
	if len(p.clauses) == 0 {
		return true
	}
	for _, clause := range p.clauses {
		if clause.matches(line) {
			return true
		}
	}
	return false
}

func (c parsedClause) matches(line Line) bool {
	for _, condition := range c.conditions {
		if !conditionMatches(condition, line) {
			return false
		}
	}
	return true
}

func parseQuery(raw string) parsedQuery {
	tokens := splitTokens(raw)
	terms := make([]queryTerm, 0, len(tokens))
	for _, token := range tokens {
		if key, value, ok := strings.Cut(token, "="); ok && key != "" {
			terms = append(terms, queryTerm{
				key:   strings.ToLower(key),
				value: strings.ToLower(value),
			})
			continue
		}
		terms = append(terms, queryTerm{text: strings.ToLower(token)})
	}
	return parsedQuery{terms: terms}
}

func (q parsedQuery) matches(line Line) bool {
	for _, term := range q.terms {
		if !conditionMatches(term, line) {
			return false
		}
	}
	return true
}

func conditionMatches(term queryTerm, line Line) bool {
	if term.key != "" {
		value, ok := line.Fields[term.key]
		return ok && strings.Contains(value, term.value)
	}
	if term.text == "" {
		return true
	}
	return strings.Contains(line.Plain, term.text)
}

func splitTokens(value string) []string {
	var tokens []string
	var current strings.Builder
	quote := rune(0)
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != 0:
			escaped = true
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func parseLogfmtFields(value string) (map[string]string, map[string]string) {
	fields := map[string]string{}
	rawFields := map[string]string{}
	for _, token := range splitTokens(value) {
		token = strings.Trim(token, "()")
		if token == "" {
			continue
		}

		key, rawValue, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		rawValue = strings.TrimSpace(rawValue)
		if key == "" || rawValue == "" {
			continue
		}

		if unquoted, ok := unquoteValue(rawValue); ok {
			rawValue = unquoted
		}
		fields[key] = strings.ToLower(rawValue)
		rawFields[key] = rawValue
	}

	return fields, rawFields
}

func unquoteValue(value string) (string, bool) {
	if len(value) < 2 {
		return value, false
	}
	if (value[0] != '"' || value[len(value)-1] != '"') && (value[0] != '\'' || value[len(value)-1] != '\'') {
		return value, false
	}

	var builder strings.Builder
	escaped := false
	for _, r := range value[1 : len(value)-1] {
		if escaped {
			builder.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		builder.WriteRune(r)
	}
	if escaped {
		builder.WriteRune('\\')
	}
	return builder.String(), true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
