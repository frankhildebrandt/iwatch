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

	lines := buf.Snapshot("")
	if len(lines) != 2 || lines[0].Text != "two" || lines[1].Text != "three" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
}

func TestFilterAndHighlight(t *testing.T) {
	buf, err := New(10, []config.HighlightRule{{ID: "err", Pattern: "error", Style: "error", Priority: 1}})
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "all good")
	buf.Append("stderr", "fatal error")

	lines := buf.Snapshot("fatal")
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if !lines[0].Matched || lines[0].HighlightRule != "error" {
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

	lines := buf.Snapshot(`level=info heartbeat`)
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if got := lines[0].Fields["msg"]; got != "thread example heartbeat" {
		t.Fatalf("unexpected msg field: %q", got)
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

	lines := buf.Snapshot(`heartbeat`)
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
}
