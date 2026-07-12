package tui

import (
	"fmt"
	"testing"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
)

func BenchmarkLogPaneViewLargeSnapshot(b *testing.B) {
	buf, err := buffer.New(100_000, nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 100_000; i++ {
		buf.Append("stdout", fmt.Sprintf(`level=INFO component=api msg="line-%d"`, i))
	}

	pane := NewLogPane(config.Default(), buf)
	lines := buf.Snapshot(buffer.SnapshotOptions{})
	observed := buf.ObservedFields()
	view := config.Default().UI.LogView
	ctx := logPaneContext{
		CommandTitle:  "cmd",
		PresetTitle:   "default",
		ProcessStatus: "running",
		AppStatus:     "clean",
		BufferLen:     len(lines),
		BufferCap:     100_000,
	}
	pane.autoScroll = true
	pane.cursor = len(lines) - 1
	pane.viewportTop = pane.tailViewportTop(120, 40, lines, observed, view)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pane.View(120, 40, true, lines, observed, view, ctx)
	}
}
