package buffer

import (
	"testing"

	"github.com/stackriot/iwatch/internal/config"
)

func FuzzLogBufferSnapshot(f *testing.F) {
	f.Add("stdout", `level=INFO msg="hello world"`, "level=info")
	f.Add("stderr", `{"json":true}`, "json")

	f.Fuzz(func(t *testing.T, source, line, query string) {
		buf, err := New(16, []config.HighlightRule{{ID: "any", Pattern: ".*", Style: "warn", Priority: 1}})
		if err != nil {
			t.Fatalf("new buffer: %v", err)
		}
		buf.Append(source, line)
		_ = buf.Snapshot(configuredSnapshot(query))
	})
}

func configuredSnapshot(query string) SnapshotOptions {
	return SnapshotOptions{
		Preset: config.FilterPreset{
			ID:    "default",
			Title: "Default",
		},
		Query: query,
	}
}
