package tui

import "strings"

type keymapSection struct {
	Title string
	Rows  []keymapRow
}

type keymapRow struct {
	Keys        string
	Description string
}

func keymapSections() []keymapSection {
	return []keymapSection{
		{
			Title: "Global",
			Rows: []keymapRow{
				{Keys: "?", Description: "Hilfe oeffnen/schliessen"},
				{Keys: "q", Description: "App beenden"},
				{Keys: "esc", Description: "Zurueck / Dialog schliessen"},
				{Keys: "tab", Description: "Fokus zwischen offenen Panes wechseln"},
			},
		},
		{
			Title: "Navigation",
			Rows: []keymapRow{
				{Keys: "j/k, arrows", Description: "Log scrollen oder Listen bewegen"},
				{Keys: "pgup/pgdown, ctrl+u/ctrl+d", Description: "Seitenweise scrollen"},
				{Keys: "home/end, G", Description: "An Anfang oder Ende springen"},
				{Keys: "n/N", Description: "Naechster oder vorheriger Treffer"},
			},
		},
		{
			Title: "Log",
			Rows: []keymapRow{
				{Keys: "enter", Description: "Tail oder Details fuer gewaehlte Zeile"},
				{Keys: "enter enter", Description: "Visuellen Trenner einfuegen"},
				{Keys: "t", Description: "Logbuffer leeren"},
				{Keys: "r", Description: "Aktives Command neu starten"},
				{Keys: "y", Description: "Share: aktuelle Zeile"},
				{Keys: "Y", Description: "Share: Zeile + Kontext"},
				{Keys: "O", Description: "Vite URL oeffnen (wenn erkannt)"},
			},
		},
		{
			Title: "Panes und Ansichten",
			Rows: []keymapRow{
				{Keys: "w", Description: "Events-Pane oeffnen/schliessen"},
				{Keys: "l", Description: "Streams-Pane oeffnen/schliessen"},
				{Keys: "v", Description: "Logfelder ein-/ausblenden"},
				{Keys: "b", Description: "Gruppierungsfeld waehlen"},
				{Keys: ", / .", Description: "Gruppierungswert vor/zurueck (inkl. alle)"},
				{Keys: "F", Description: "Live-Feldfilter oeffnen"},
				{Keys: "g", Description: "Config-Editor oeffnen"},
				{Keys: "o in Streams", Description: "Stream-Details oeffnen"},
			},
		},
		{
			Title: "Suche und Layout",
			Rows: []keymapRow{
				{Keys: "/, f", Description: "Bottom-Query oeffnen"},
				{Keys: "[, ], left, right", Description: "Preset wechseln"},
				{Keys: "S", Description: "Split-Richtung wechseln"},
			},
		},
		{
			Title: "Share",
			Rows: []keymapRow{
				{Keys: "y", Description: "Copy (OSC52)"},
				{Keys: "s", Description: "Export to file"},
				{Keys: "esc/enter", Description: "Schliessen"},
			},
		},
	}
}

func renderKeymapHelp() string {
	sections := keymapSections()
	var out []string
	for i, section := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, section.Title)
		for _, row := range section.Rows {
			out = append(out, "  "+keymapPadRight(row.Keys, 28)+" "+row.Description)
		}
	}
	return strings.Join(out, "\n")
}

func keymapPadRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

