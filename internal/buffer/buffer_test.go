package buffer

import (
	"testing"

	"github.com/stackriot/iwatch/internal/config"
)

func TestRingBufferDropsOldest(t *testing.T) {
	buf, err := New(2, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "one")
	buf.Append("stdout", "two")
	buf.Append("stdout", "three")

	lines := buf.Snapshot(SnapshotOptions{})
	if len(lines) != 2 || lines[0].Text != "two" || lines[1].Text != "three" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
}

func TestRingBufferKeepsOrderAfterMultipleWraps(t *testing.T) {
	buf, err := New(3, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"one", "two", "three", "four", "five"} {
		buf.Append("stdout", line)
	}

	lines := buf.Snapshot(SnapshotOptions{})
	if len(lines) != 3 {
		t.Fatalf("len = %d", len(lines))
	}
	for idx, want := range []string{"three", "four", "five"} {
		if lines[idx].Text != want {
			t.Fatalf("line[%d] = %q, want %q", idx, lines[idx].Text, want)
		}
	}
}

func TestTruncateClearsLinesAndKeepsObservedFields(t *testing.T) {
	buf, err := New(3, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello"`)
	buf.Truncate()

	if got := buf.Len(); got != 0 {
		t.Fatalf("Len() after Truncate() = %d", got)
	}
	if got := buf.Snapshot(SnapshotOptions{}); len(got) != 0 {
		t.Fatalf("Snapshot() after Truncate() = %+v", got)
	}
	if got := buf.ObservedFields(); len(got) != 2 || got[0] != "level" || got[1] != "msg" {
		t.Fatalf("ObservedFields() after Truncate() = %#v", got)
	}

	buf.Append("stdout", `level=ERROR msg="again"`)
	lines := buf.Snapshot(SnapshotOptions{})
	if len(lines) != 1 || lines[0].Text != `level=ERROR msg="again"` {
		t.Fatalf("unexpected lines after append: %+v", lines)
	}
}

func TestSnapshotLimitReturnsLastMatchingLines(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`level=INFO msg="one"`,
		`level=ERROR msg="two"`,
		`level=INFO msg="three"`,
		`level=ERROR msg="four"`,
	} {
		buf.Append("stdout", line)
	}

	lines := buf.Snapshot(SnapshotOptions{Query: "level=error", Limit: 1})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].RawFields["msg"]; got != "four" {
		t.Fatalf("msg = %q, want four", got)
	}
}

func TestSnapshotCacheInvalidatesOnAppend(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="one"`)

	lines := buf.Snapshot(SnapshotOptions{Query: "level=info"})
	if len(lines) != 1 {
		t.Fatalf("initial len = %d", len(lines))
	}

	buf.Append("stdout", `level=INFO msg="two"`)
	lines = buf.Snapshot(SnapshotOptions{Query: "level=info"})
	if len(lines) != 2 {
		t.Fatalf("len after append = %d", len(lines))
	}
	if got := lines[1].RawFields["msg"]; got != "two" {
		t.Fatalf("last msg = %q", got)
	}
}

func TestFilterKeepsRuleHighlightOnly(t *testing.T) {
	buf, err := New(10, []config.HighlightRule{{ID: "err", Pattern: "error", Style: "error", Priority: 1}})
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "all good")
	buf.Append("stderr", "fatal error")

	lines := buf.Snapshot(SnapshotOptions{Query: "fatal"})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if lines[0].Matched || lines[0].HighlightRule != "error" {
		t.Fatalf("unexpected line: %+v", lines[0])
	}
}

func TestLogfmtFieldFilters(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `(time=2026-05-12T12:23:26.044+02:00 level=INFO msg="thread example heartbeat" lua-manager.resource=thread-example)`)
	buf.Append("stdout", `plain text line`)

	lines := buf.Snapshot(SnapshotOptions{Query: `level=info heartbeat`})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].Fields["msg"]; got != "thread example heartbeat" {
		t.Fatalf("unexpected msg field: %q", got)
	}
	if got := lines[0].RawFields["msg"]; got != "thread example heartbeat" {
		t.Fatalf("unexpected raw msg field: %q", got)
	}
	if got := lines[0].Fields["lua-manager.resource"]; got != "thread-example" {
		t.Fatalf("unexpected resource field: %q", got)
	}
}

func TestObservedFieldsTracksFirstSeenOrder(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `level=INFO msg="hello"`)
	buf.Append("stdout", `msg="again" component=api status=200 LEVEL=DEBUG`)

	got := buf.ObservedFields()
	want := []string{"level", "msg", "component", "status"}
	if len(got) != len(want) {
		t.Fatalf("ObservedFields() = %#v", got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("ObservedFields()[%d] = %q, want %q; all fields: %#v", idx, got[idx], want[idx], got)
		}
	}
}

func TestInvalidLogfmtStillMatchesPlainText(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `not-logfmt but mentions heartbeat`)

	lines := buf.Snapshot(SnapshotOptions{Query: `heartbeat`})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
}

func TestPresetClausesUseORLogic(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `level=INFO msg="heartbeat one"`)
	buf.Append("stdout", `level=ERROR msg="panic two"`)
	buf.Append("stdout", `level=DEBUG msg="skip three"`)

	lines := buf.Snapshot(SnapshotOptions{
		Preset: config.FilterPreset{
			Clauses: []config.FilterClause{
				{Conditions: []config.FilterCondition{{Field: "level", Value: "info"}}},
				{Conditions: []config.FilterCondition{{Value: "panic"}}},
			},
		},
	})

	if len(lines) != 2 {
		t.Fatalf("len = %d", len(lines))
	}
}

func TestFieldFiltersUseContainsAndANDLogic(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `level=INFO component=api msg="ready"`)
	buf.Append("stdout", `level=ERROR component=api msg="bad"`)
	buf.Append("stdout", `level=ERROR component=worker msg="bad"`)

	lines := buf.Snapshot(SnapshotOptions{
		FieldFilters: map[string]string{
			"level":     "err",
			"component": "api",
		},
	})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].Fields["component"]; got != "api" {
		t.Fatalf("component = %q", got)
	}
}

func TestFieldFiltersIgnoreEmptyValuesAndRequireExistingFields(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `level=ERROR component=api`)

	lines := buf.Snapshot(SnapshotOptions{
		FieldFilters: map[string]string{
			"level":   "err",
			"missing": "value",
			"empty":   "",
		},
	})
	if len(lines) != 0 {
		t.Fatalf("expected missing field filter to exclude line, got %d", len(lines))
	}

	lines = buf.Snapshot(SnapshotOptions{
		FieldFilters: map[string]string{
			"level": "ERR",
			"empty": "",
		},
	})
	if len(lines) != 1 {
		t.Fatalf("expected empty field filter to be ignored, got %d", len(lines))
	}
}

func TestPresetHighlightsOverrideFallbackRules(t *testing.T) {
	buf, err := New(10, []config.HighlightRule{{ID: "base", Pattern: "error", Style: "error", Priority: 10}})
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `fatal error`)

	lines := buf.Snapshot(SnapshotOptions{
		Preset: config.FilterPreset{
			HighlightRules: []config.HighlightRule{{ID: "preset", Pattern: "fatal", Style: "warn", Priority: 50}},
		},
		Highlights: []config.HighlightRule{{ID: "preset", Pattern: "fatal", Style: "warn", Priority: 50}},
	})

	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if lines[0].HighlightRule != "warn" {
		t.Fatalf("highlight = %q", lines[0].HighlightRule)
	}
}

func TestLogfmtFieldParsingUsesSpaceSeparatedKeyValuePairs(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `level=INFO msg="hello world" component=api status=200`)

	lines := buf.Snapshot(SnapshotOptions{Query: `component=api msg=hello`})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].Fields["msg"]; got != "hello world" {
		t.Fatalf("unexpected msg field: %q", got)
	}
	if got := lines[0].RawFields["msg"]; got != "hello world" {
		t.Fatalf("unexpected raw msg field: %q", got)
	}
	if got := lines[0].Fields["status"]; got != "200" {
		t.Fatalf("unexpected status field: %q", got)
	}
}

func TestLogfmtFieldParsingHandlesEscapedQuotedValues(t *testing.T) {
	buf, err := New(10, nil)
	if err != nil {
		t.Fatal(err)
	}

	buf.Append("stdout", `msg="say \"hello\"" path='/tmp/demo folder'`)

	lines := buf.Snapshot(SnapshotOptions{Query: `msg=say path=demo`})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].Fields["msg"]; got != `say "hello"` {
		t.Fatalf("unexpected msg field: %q", got)
	}
	if got := lines[0].RawFields["msg"]; got != `say "hello"` {
		t.Fatalf("unexpected raw msg field: %q", got)
	}
	if got := lines[0].Fields["path"]; got != "/tmp/demo folder" {
		t.Fatalf("unexpected path field: %q", got)
	}
}
