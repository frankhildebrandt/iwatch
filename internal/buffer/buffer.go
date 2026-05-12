package buffer

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/stackriot/iwatch/internal/config"
)

type Line struct {
	Index     int
	Session   int
	Source    string
	Text      string
	Plain     string
	Fields    map[string]string
	Timestamp time.Time
}

type ViewLine struct {
	Line
	Matched       bool
	HighlightRule string
}

type LogBuffer struct {
	mu       sync.RWMutex
	capacity int
	lines    []Line
	nextIdx  int
	session  int
	rules    []compiledRule
}

type compiledRule struct {
	id       string
	pattern  *regexp.Regexp
	style    string
	priority int
}

func New(capacity int, rules []config.HighlightRule) (*LogBuffer, error) {
	if capacity <= 0 {
		capacity = config.DefaultBufferLines
	}

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

	return &LogBuffer{
		capacity: capacity,
		lines:    make([]Line, 0, min(capacity, 1024)),
		rules:    compiled,
	}, nil
}

func (b *LogBuffer) StartSession(title string) {
	b.Append("system", "=== "+title+" ===")
	b.mu.Lock()
	b.session++
	b.mu.Unlock()
}

func (b *LogBuffer) Append(source, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	line := Line{
		Index:     b.nextIdx,
		Session:   b.session,
		Source:    source,
		Text:      text,
		Plain:     strings.ToLower(stripANSI(text)),
		Fields:    parseLogfmtFields(stripANSI(text)),
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

func (b *LogBuffer) Snapshot(query string) []ViewLine {
	compiled := parseQuery(query)

	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []ViewLine
	for _, line := range b.lines {
		matched := compiled.matches(line)
		if !matched {
			continue
		}
		view := ViewLine{Line: line, Matched: compiled.active()}
		view.HighlightRule = b.matchRule(line.Text)
		out = append(out, view)
	}
	return out
}

func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

func (b *LogBuffer) matchRule(text string) string {
	selected := ""
	priority := -1 << 30
	for _, rule := range b.rules {
		if rule.pattern.MatchString(text) && rule.priority >= priority {
			selected = rule.style
			priority = rule.priority
		}
	}
	return selected
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

type parsedQuery struct {
	terms []queryTerm
}

type queryTerm struct {
	key   string
	value string
	text  string
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

func (q parsedQuery) active() bool {
	return len(q.terms) > 0
}

func (q parsedQuery) matches(line Line) bool {
	for _, term := range q.terms {
		if term.key != "" {
			value, ok := line.Fields[term.key]
			if !ok || !strings.Contains(value, term.value) {
				return false
			}
			continue
		}
		if !strings.Contains(line.Plain, term.text) {
			return false
		}
	}
	return true
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

func parseLogfmtFields(value string) map[string]string {
	fields := map[string]string{}
	runes := []rune(value)

	for i := 0; i < len(runes); {
		for i < len(runes) && (unicode.IsSpace(runes[i]) || runes[i] == '(' || runes[i] == ')') {
			i++
		}
		if i >= len(runes) {
			break
		}

		keyStart := i
		for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != '=' {
			i++
		}
		if i >= len(runes) || runes[i] != '=' {
			for i < len(runes) && !unicode.IsSpace(runes[i]) {
				i++
			}
			continue
		}

		key := strings.ToLower(string(runes[keyStart:i]))
		i++
		if key == "" {
			continue
		}

		var rawValue string
		if i < len(runes) && (runes[i] == '"' || runes[i] == '\'') {
			quote := runes[i]
			i++
			var builder strings.Builder
			escaped := false
			for i < len(runes) {
				r := runes[i]
				i++
				if escaped {
					builder.WriteRune(r)
					escaped = false
					continue
				}
				if r == '\\' {
					escaped = true
					continue
				}
				if r == quote {
					break
				}
				builder.WriteRune(r)
			}
			rawValue = builder.String()
		} else {
			valueStart := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != ')' {
				i++
			}
			rawValue = string(runes[valueStart:i])
		}

		fields[key] = strings.ToLower(rawValue)
	}

	return fields
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
