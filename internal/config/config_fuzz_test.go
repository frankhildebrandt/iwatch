package config

import "testing"

func FuzzCloneAndDefaultMerge(f *testing.F) {
	f.Add("watch", 10, "cmd")

	f.Fuzz(func(t *testing.T, watchPath string, bufferLines int, commandID string) {
		cfg := Config{
			WatchPath:      watchPath,
			BufferLines:    bufferLines,
			DefaultCommand: commandID,
			UI: UIConfig{
				OpenPanes:      []string{"log"},
				SplitDirection: "vertical",
				FocusPane:      "log",
				Presets: []FilterPreset{
					{ID: DefaultPresetID, Title: "Default"},
				},
				ActivePreset: DefaultPresetID,
			},
		}

		cloned := Clone(cfg)
		merged := DefaultMerge(cloned)
		if merged.UI.ActivePreset == "" {
			t.Fatal("expected active preset to be normalized")
		}
	})
}
