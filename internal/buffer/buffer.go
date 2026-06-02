package buffer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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

// GroupFilter restricts the snapshot to lines with an exact field value match.
type GroupFilter struct {
	Field string // empty disables grouping
	Value string // empty means all values for Field
}

// SnapshotOptions describes how visible lines should be filtered and highlighted.
type SnapshotOptions struct {
	Preset       config.FilterPreset
	Query        string
	Group        GroupFilter
	FieldFilters map[string]string
	Highlights   []config.HighlightRule
}

// LogBuffer keeps a bounded in-memory history of log lines.
type LogBuffer struct {
	mu         sync.RWMutex
	capacity   int
	lines      []Line
	start      int
	version    uint64
	nextIdx    int
	session    int
	fieldOrder []string
	fieldSet   map[string]struct{}
	baseRules  []config.HighlightRule
	compiledBy string
	compiled   []compiledRule
	cacheKey   string
	cacheVer   uint64
	cacheLines []ViewLine
	cachePreset       parsedPreset
	cacheRuntimeQuery parsedQuery
	cacheGroupFilter  parsedGroupFilter
	cacheFieldFilters parsedFieldFilters
	cacheCompiled     []compiledRule
	fieldValueCounts  map[string]map[string]int
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
		capacity:         capacity,
		lines:            make([]Line, 0, min(capacity, 1024)),
		fieldSet:         make(map[string]struct{}),
		baseRules:        append([]config.HighlightRule(nil), rules...),
		fieldValueCounts: make(map[string]map[string]int),
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
	_ = b.AppendLine(source, text)
}

// AppendLine stores a new log line in the ring buffer and returns it.
func (b *LogBuffer) AppendLine(source, text string) Line {
	b.mu.Lock()
	defer b.mu.Unlock()

	clean := stripANSI(text)
	fields, rawFields, fieldKeys := parseStructuredFields(clean)
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
	b.version++
	b.recordObservedFields(fieldKeys)

	if len(b.lines) == b.capacity {
		evictedLine := b.lines[b.start]
		b.removeFieldValues(evictedLine)
		if b.cacheKey != "" {
			b.removeFromSnapshotCache(evictedLine.Index)
		}
		b.lines[b.start] = line
		b.start = (b.start + 1) % b.capacity
	} else {
		b.lines = append(b.lines, line)
	}

	b.recordFieldValues(line)
	if b.cacheKey != "" {
		b.cacheVer = b.version
		if b.lineMatchesCacheFilters(line) {
			b.appendToSnapshotCache(line)
		}
	}

	return line
}

// Truncate clears buffered log lines while keeping session and observed field metadata.
func (b *LogBuffer) Truncate() {
	b.mu.Lock()
	defer b.mu.Unlock()

	clear(b.lines)
	b.lines = b.lines[:0]
	b.start = 0
	b.version++
	b.fieldValueCounts = make(map[string]map[string]int)
	b.invalidateSnapshotCache()
}

// ObservedFields returns all logfmt keys seen so far in first-seen order.
func (b *LogBuffer) ObservedFields() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]string(nil), b.fieldOrder...)
}

// DistinctFieldValues returns sorted unique values for field across buffered lines.
func (b *LogBuffer) DistinctFieldValues(field string) []string {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	counts, ok := b.fieldValueCounts[field]
	if !ok || len(counts) == 0 {
		return nil
	}

	out := make([]string, 0, len(counts))
	for value := range counts {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// Snapshot returns the currently visible lines for the supplied query and preset.
// The returned slice is read-only and remains valid until the next buffer mutation.
func (b *LogBuffer) Snapshot(opts SnapshotOptions) []ViewLine {
	presetQuery := parsePreset(opts.Preset)
	runtimeQuery := parseQuery(opts.Query)
	groupFilter := parseGroupFilter(opts.Group)
	fieldFilters := parseFieldFilters(opts.FieldFilters)
	rules := opts.Highlights
	if len(rules) == 0 {
		rules = b.baseRules
	}
	compiled, _ := b.rulesFor(rules)
	key := snapshotCacheKey(opts, rules)

	b.mu.RLock()
	if key == b.cacheKey && b.cacheVer == b.version && b.cacheLines != nil {
		lines := b.cacheLines
		b.mu.RUnlock()
		return lines
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	if key == b.cacheKey && b.cacheVer == b.version && b.cacheLines != nil {
		return b.cacheLines
	}

	out := b.fullSnapshot(presetQuery, runtimeQuery, groupFilter, fieldFilters, compiled)
	b.cacheKey = key
	b.cacheVer = b.version
	b.cacheLines = out
	b.cachePreset = presetQuery
	b.cacheRuntimeQuery = runtimeQuery
	b.cacheGroupFilter = groupFilter
	b.cacheFieldFilters = fieldFilters
	b.cacheCompiled = compiled
	return out
}

// SnapshotLinePosition returns the snapshot slice index for a buffered line index.
func (b *LogBuffer) SnapshotLinePosition(lineIndex int) (int, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return snapshotLineIndex(b.cacheLines, lineIndex)
}

// Len returns the number of buffered lines.
func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

func (b *LogBuffer) lineAt(index int) Line {
	if len(b.lines) < b.capacity {
		return b.lines[index]
	}
	return b.lines[(b.start+index)%b.capacity]
}

func (b *LogBuffer) fullSnapshot(presetQuery parsedPreset, runtimeQuery parsedQuery, groupFilter parsedGroupFilter, fieldFilters parsedFieldFilters, compiled []compiledRule) []ViewLine {
	out := make([]ViewLine, 0, len(b.lines))
	for idx := 0; idx < len(b.lines); idx++ {
		line := b.lineAt(idx)
		if !presetQuery.matches(line) || !runtimeQuery.matches(line) || !groupFilter.matches(line) || !fieldFilters.matches(line) {
			continue
		}
		view := ViewLine{Line: line}
		view.HighlightRule = matchRule(compiled, line.Text)
		out = append(out, view)
	}
	return out
}

func (b *LogBuffer) recordObservedFields(fields []string) {
	for _, key := range fields {
		if _, ok := b.fieldSet[key]; ok {
			continue
		}
		b.fieldSet[key] = struct{}{}
		b.fieldOrder = append(b.fieldOrder, key)
	}
}

func snapshotCacheKey(opts SnapshotOptions, rules []config.HighlightRule) string {
	return strings.Join([]string{
		opts.Query,
		presetKey(opts.Preset),
		groupFilterKey(opts.Group),
		fieldFiltersKey(opts.FieldFilters),
		rulesKey(rules),
	}, "\x00")
}

func groupFilterKey(filter GroupFilter) string {
	field := strings.ToLower(strings.TrimSpace(filter.Field))
	value := strings.ToLower(strings.TrimSpace(filter.Value))
	if field == "" || value == "" {
		return ""
	}
	return field + "=" + value
}

func presetKey(preset config.FilterPreset) string {
	var builder strings.Builder
	builder.WriteString(preset.ID)
	builder.WriteByte('|')
	builder.WriteString(preset.Title)
	for _, clause := range preset.Clauses {
		builder.WriteByte('[')
		for _, cond := range clause.Conditions {
			builder.WriteString(cond.Field)
			builder.WriteByte('=')
			builder.WriteString(cond.Value)
			builder.WriteByte(';')
		}
		builder.WriteByte(']')
	}
	return builder.String()
}

func fieldFiltersKey(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(filters[key])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func (b *LogBuffer) invalidateSnapshotCache() {
	b.cacheKey = ""
	b.cacheLines = nil
	b.cacheVer = 0
}

func (b *LogBuffer) lineMatchesCacheFilters(line Line) bool {
	return b.cachePreset.matches(line) &&
		b.cacheRuntimeQuery.matches(line) &&
		b.cacheGroupFilter.matches(line) &&
		b.cacheFieldFilters.matches(line)
}

func (b *LogBuffer) appendToSnapshotCache(line Line) {
	view := ViewLine{Line: line}
	view.HighlightRule = matchRule(b.cacheCompiled, line.Text)
	b.cacheLines = append(b.cacheLines, view)
}

func (b *LogBuffer) removeFromSnapshotCache(lineIndex int) {
	pos, ok := snapshotLineIndex(b.cacheLines, lineIndex)
	if !ok {
		return
	}
	if pos == 0 {
		b.cacheLines = b.cacheLines[1:]
		return
	}
	b.cacheLines = append(b.cacheLines[:pos], b.cacheLines[pos+1:]...)
}

func snapshotLineIndex(lines []ViewLine, lineIndex int) (int, bool) {
	if len(lines) == 0 {
		return 0, false
	}
	lo, hi := 0, len(lines)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case lines[mid].Index == lineIndex:
			return mid, true
		case lines[mid].Index < lineIndex:
			lo = mid + 1
		default:
			hi = mid - 1
		}
	}
	return 0, false
}

func (b *LogBuffer) recordFieldValues(line Line) {
	for field, value := range line.Fields {
		if value == "" {
			continue
		}
		counts, ok := b.fieldValueCounts[field]
		if !ok {
			counts = make(map[string]int)
			b.fieldValueCounts[field] = counts
		}
		counts[value]++
	}
}

func (b *LogBuffer) removeFieldValues(line Line) {
	for field, value := range line.Fields {
		if value == "" {
			continue
		}
		counts, ok := b.fieldValueCounts[field]
		if !ok {
			continue
		}
		counts[value]--
		if counts[value] <= 0 {
			delete(counts, value)
		}
		if len(counts) == 0 {
			delete(b.fieldValueCounts, field)
		}
	}
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

type parsedFieldFilters struct {
	terms []queryTerm
}

type parsedGroupFilter struct {
	field string
	value string
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

func parseFieldFilters(filters map[string]string) parsedFieldFilters {
	terms := make([]queryTerm, 0, len(filters))
	for key, value := range filters {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.ToLower(strings.TrimSpace(value))
		if key == "" || value == "" {
			continue
		}
		terms = append(terms, queryTerm{key: key, value: value})
	}
	return parsedFieldFilters{terms: terms}
}

func parseGroupFilter(filter GroupFilter) parsedGroupFilter {
	return parsedGroupFilter{
		field: strings.ToLower(strings.TrimSpace(filter.Field)),
		value: strings.ToLower(strings.TrimSpace(filter.Value)),
	}
}

func (g parsedGroupFilter) matches(line Line) bool {
	if g.field == "" || g.value == "" {
		return true
	}
	lineValue, ok := line.Fields[g.field]
	return ok && lineValue == g.value
}

func (q parsedQuery) matches(line Line) bool {
	for _, term := range q.terms {
		if !conditionMatches(term, line) {
			return false
		}
	}
	return true
}

func (f parsedFieldFilters) matches(line Line) bool {
	for _, term := range f.terms {
		if !conditionMatches(term, line) {
			return false
		}
	}
	return true
}

func conditionMatches(term queryTerm, line Line) bool {
	if term.key != "" {
		if term.key == "source" {
			return strings.Contains(strings.ToLower(line.Source), term.value)
		}
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

func parseLogfmtFields(value string) (map[string]string, map[string]string, []string) {
	fields := map[string]string{}
	rawFields := map[string]string{}
	var keys []string
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
		keys = append(keys, key)
	}

	return fields, rawFields, keys
}

func parseStructuredFields(value string) (map[string]string, map[string]string, []string) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") && json.Valid([]byte(trimmed)) {
		fields, rawFields, keys, ok := parseJSONFields(trimmed)
		if ok {
			return fields, rawFields, keys
		}
	}
	return parseLogfmtFields(value)
}

func parseJSONFields(value string) (map[string]string, map[string]string, []string, bool) {
	var root any
	if err := json.Unmarshal([]byte(value), &root); err != nil {
		return nil, nil, nil, false
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}

	const (
		maxKeys     = 200
		maxDepth    = 6
		maxValueLen = 4096
	)

	fields := make(map[string]string, 32)
	rawFields := make(map[string]string, 32)
	keys := make([]string, 0, 32)

	var count int
	var walk func(prefix string, v any, depth int)
	walk = func(prefix string, v any, depth int) {
		if count >= maxKeys || depth > maxDepth || v == nil {
			return
		}
		switch vv := v.(type) {
		case map[string]any:
			for k, child := range vv {
				if k == "" {
					continue
				}
				key := k
				if prefix != "" {
					key = prefix + "." + k
				}
				walk(key, child, depth+1)
				if count >= maxKeys {
					return
				}
			}
		case []any:
			// Avoid exploding arrays; keep a short summary.
			raw := fmt.Sprintf("%d items", len(vv))
			if len(raw) > maxValueLen {
				raw = raw[:maxValueLen]
			}
			fields[strings.ToLower(prefix)] = strings.ToLower(raw)
			rawFields[prefix] = raw
			keys = append(keys, strings.ToLower(prefix))
			count++
		case string:
			raw := vv
			if len(raw) > maxValueLen {
				raw = raw[:maxValueLen]
			}
			fields[strings.ToLower(prefix)] = strings.ToLower(raw)
			rawFields[prefix] = raw
			keys = append(keys, strings.ToLower(prefix))
			count++
		case bool, float64:
			raw := fmt.Sprintf("%v", vv)
			fields[strings.ToLower(prefix)] = strings.ToLower(raw)
			rawFields[prefix] = raw
			keys = append(keys, strings.ToLower(prefix))
			count++
		default:
			// Fallback: best-effort stringification.
			raw := fmt.Sprintf("%v", vv)
			if len(raw) > maxValueLen {
				raw = raw[:maxValueLen]
			}
			fields[strings.ToLower(prefix)] = strings.ToLower(raw)
			rawFields[prefix] = raw
			keys = append(keys, strings.ToLower(prefix))
			count++
		}
	}

	for k, v := range obj {
		if k == "" {
			continue
		}
		walk(k, v, 1)
		if count >= maxKeys {
			break
		}
	}

	if len(keys) == 0 {
		return nil, nil, nil, false
	}
	return fields, rawFields, keys, true
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
