# iwatch

`iwatch` ist ein interaktives Watch-Tool fuer Build- und Entwicklungs-Workflows. Es kombiniert automatische Command-Erkennung, Dateisystem-Watching und eine TUI zum Lesen, Filtern und Durchsuchen von Prozess-Output.

## Features

- Auto-Mode mit Erkennung von `package.json`-Scripts und `Makefile`-Targets
- Rekursives Watching des aktuellen Pfads oder eines expliziten `--path`
- Manueller Rebuild-Flow mit sichtbarem `rebuild possible`-Status
- TUI mit Log-View, Command-Palette, Bottom-Query und Watch-Events
- Scrollen per Cursor, Live-Filter, logfmt-verstaendige Feldabfragen und Highlight-Regeln
- Konfigurierbarer Ringbuffer, standardmaessig `1_000_000` Zeilen
- Beenden per `Esc` + `Esc` + `Esc`

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
- `tab`: Fokus zwischen offenen Panes wechseln
- `up/down/pgup/pgdown`: scrollen oder Listen bewegen
- `n` / `N`: zum naechsten oder vorherigen sichtbaren Treffer
- `s`: Split-Richtung wechseln
- `esc`: Bottom-Query schliessen
- `esc` dreimal: App beenden

## Query Syntax

- Freitext wie `heartbeat` matcht case-insensitive gegen die gesamte Logzeile.
- `key=value`-Filter wie `level=info` oder `lua-manager.resource=thread-example` matchen gegen logfmt-aehnliche Felder in der Zeile.
- Mehrere Tokens werden mit `AND` kombiniert, zum Beispiel `level=info heartbeat` oder `msg=heartbeat level=info`.
- Feldwerte matchen case-insensitive als Substring, also passt `msg=heart` auch auf `msg="thread example heartbeat"`.

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
    "focusPane": "log"
  }
}
```
