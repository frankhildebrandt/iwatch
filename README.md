# iwatch

`iwatch` ist ein interaktives Watch-Tool fuer Build- und Entwicklungs-Workflows. Es kombiniert automatische Command-Erkennung, Dateisystem-Watching und eine TUI zum Lesen, Filtern und Durchsuchen von Prozess-Output.

## Features

- Auto-Mode mit Erkennung von `package.json`-Scripts und `Makefile`-Targets
- Rekursives Watching des aktuellen Pfads oder eines expliziten `--path`
- Manueller Rebuild-Flow mit sichtbarem `rebuild possible`-Status
- TUI mit Log-View, Command-Palette, Bottom-Query und Watch-Events
- Scrollen per Cursor, Live-Filter, logfmt-verstaendige Feldabfragen und Highlight-Regeln
- Selektierbare Logzeilen mit Detailansicht fuer geparste Felder und pretty printed JSON-Werte
- Benannte Filter-Presets mit OR-Klauseln, Highlighting, Logfeld-Anzeige und Zeitformaten
- Konfigurierbarer Ringbuffer, standardmaessig `1_000_000` Zeilen
- Beenden per `q` oder `Esc` + `Esc` + `Esc`

## Installation

```bash
go mod tidy
go build ./cmd/iwatch
```

## Usage

```bash
iwatch
iwatch --path ./src
iwatch --command npm:dev
iwatch --buffer-lines 200000
iwatch --config ./.iwatch.config.json
```

## Keybindings

- `r`: aktives Command stoppen und neu starten
- `c`: Command-Palette oeffnen oder schliessen
- `/`: Bottom-Query oeffnen
- `f`: Bottom-Query oeffnen
- `w`: Watch-Events-Pane oeffnen
- `g`: Fullscreen-Config-Editor oeffnen
- `[` / `]` oder `left/right`: zwischen konfigurierten Presets umschalten
- `tab`: Fokus zwischen offenen Panes wechseln
- `up/down`, `j/k`: scrollen oder Listen bewegen
- `pgup/pgdown`, `ctrl+u/ctrl+d`: seitenweise scrollen
- `home/end`, `G`: an Anfang oder Ende springen
- `s`: Logzeilen-Auswahl mit Cursor an- oder ausschalten
- `enter`: ans Log-Ende springen oder in der Auswahl die Detailansicht oeffnen
- `enter` zweimal schnell: visuellen Trenner einfuegen
- `n` / `N`: zum naechsten oder vorherigen sichtbaren Treffer
- `S`: Split-Richtung wechseln
- `esc` oder `enter` in der Detailansicht: zurueck zur Logansicht
- `esc`: Bottom-Query schliessen
- `q`: App beenden
- `esc` dreimal: App beenden

## Query Syntax

- Freitext wie `heartbeat` matcht case-insensitive gegen die gesamte Logzeile.
- `key=value`-Filter wie `level=info` oder `lua-manager.resource=thread-example` matchen gegen logfmt-aehnliche Felder in der Zeile.
- Mehrere Tokens werden mit `AND` kombiniert, zum Beispiel `level=info heartbeat` oder `msg=heartbeat level=info`.
- Feldwerte matchen case-insensitive als Substring, also passt `msg=heart` auch auf `msg="thread example heartbeat"`.

Die Bottom-Query wirkt zusaetzlich auf das aktive Preset. Strukturierte OR-Filter werden in der Config-Datei ueber Presets gepflegt.

## Config Editor

- `g` oeffnet einen Fullscreen-Editor fuer Presets, Filter, Highlighting, sichtbare Logfelder und Darstellungsoptionen.
- `up/down` oder `j/k` bewegt die Auswahl.
- `pgup/pgdown` oder `ctrl+u/ctrl+d` bewegt sich seitenweise.
- `enter` toggelt oder fuehrt die aktuelle Aktion aus.
- `e` bearbeitet Textfelder wie Preset-ID, Clauses, Highlight-Regeln oder sichtbare Logfelder.
- `a` fuegt je nach Bereich Presets, OR-Clauses oder Highlight-Regeln hinzu.
- `d` loescht den aktuellen Preset-/Clause-/Rule-Eintrag.
- `y` dupliziert das aktive Preset.
- `q` beendet die App direkt, `esc` verlaesst den Editor.
- Speichern schreibt projektlokal nach `./.iwatch/config.json`, bestehende `./.iwatch.config.json` wird weiterverwendet.

## Config

Geladene Config-Dateien in Prioritaetsreihenfolge:

1. `~/.iwatch/config.json`
2. `./.iwatch/config.json`
3. `./.iwatch.config.json`

CLI-Flags haben Vorrang vor Config-Werten.

Beispiel:

```json
{
  "watchPath": "./",
  "bufferLines": 1000000,
  "defaultCommand": "npm:dev",
  "commands": [
    {
      "id": "go:test",
      "title": "go test",
      "cmd": "go test ./...",
      "source": "config"
    }
  ],
  "highlightRules": [
    {
      "id": "errors",
      "pattern": "(?i)error|failed",
      "style": "error",
      "priority": 100
    },
    {
      "id": "warn",
      "pattern": "(?i)warn",
      "style": "warn",
      "priority": 50
    }
  ],
  "ui": {
    "openPanes": ["log", "events"],
    "splitDirection": "vertical",
    "focusPane": "log",
    "activePreset": "ops",
    "presets": [
      {
        "id": "ops",
        "title": "Ops",
        "clauses": [
          {
            "conditions": [
              { "field": "level", "value": "error" }
            ]
          },
          {
            "conditions": [
              { "value": "panic" }
            ]
          }
        ],
        "highlightRules": [
          {
            "id": "panic",
            "pattern": "(?i)panic|fatal",
            "style": "error",
            "priority": 100
          }
        ]
      },
      {
        "id": "heartbeat",
        "title": "Heartbeat",
        "clauses": [
          {
            "conditions": [
              { "field": "msg", "value": "heartbeat" },
              { "field": "level", "value": "info" }
            ]
          }
        ]
      }
    ],
    "logView": {
      "visibleFields": ["level", "msg", "lua-manager.resource"],
      "showRawMessage": true,
      "showSource": false,
      "showTimestamp": true,
      "timeFormat": "date-short",
      "wrapMode": "field"
    }
  }
}
```

### Preset-Struktur

- `ui.presets[*].clauses` ist eine OR-Liste.
- `clauses[*].conditions` innerhalb einer Clause werden mit AND kombiniert.
- Bedingungen mit `field` matchen gegen erkannte logfmt-Felder.
- Bedingungen ohne `field` matchen als Freitext gegen die komplette Logzeile.

### Log View

- `timeFormat`: `time`, `date-short`, `relative`
  Bei `date-short` und `relative` wird keine separate Zeitspalte gerendert; die feste Zeitspalte ist nur fuer `time` sichtbar.
- `wrapMode`: `off`, `simple`, `field`
- `visibleFields`: steuert Reihenfolge und Sichtbarkeit einzelner logfmt-Felder
- Root-`highlightRules` bleiben als Rueckwaertskompatibilitaets-Fallback aktiv, wenn ein Preset keine eigenen Regeln hat
