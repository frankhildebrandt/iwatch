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
