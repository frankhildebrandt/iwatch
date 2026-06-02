# iwatch

`iwatch` ist ein interaktives Watch-Tool fuer Build- und Entwicklungs-Workflows. Es kombiniert automatische Command-Erkennung, Dateisystem-Watching und eine TUI zum Lesen, Filtern und Durchsuchen von Prozess-Output.

## Features

- Auto-Mode mit Erkennung von `package.json`-Scripts und `Makefile`-Targets
- Rekursives Watching des aktuellen Pfads oder eines expliziten `--path`
- Manueller Rebuild-Flow mit sichtbarem `rebuild possible`-Status
- TUI mit Log-View, Command-Palette, Bottom-Query und Watch-Events
- Zusaetzliche LogStreams fuer Dateien und Hintergrundprozesse, pro Preset aktivierbar
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
- `p`: Command-Output-Pane oeffnen oder schliessen
- `?`: Keymap-Hilfe oeffnen oder schliessen
- `l`: Streams-Pane oeffnen oder schliessen
- `/`: Bottom-Query oeffnen
- `f`: Bottom-Query oeffnen
- `w`: Watch-Events-Pane oeffnen
- `v`: Logfeld-Menue oeffnen, per Tippen filtern und erkannte logfmt-Felder ein-/ausblenden
- `F`: Live-Feldfilter oeffnen, Felder waehlen und per Contains-Wert filtern
- `g`: Fullscreen-Config-Editor oeffnen
- `[` / `]` oder `left/right`: zwischen konfigurierten Presets umschalten
- `tab`: Fokus zwischen offenen Panes wechseln
- `up/down`, `j/k`: scrollen oder Listen bewegen
- `pgup/pgdown`, `ctrl+u/ctrl+d`: seitenweise scrollen
- `home/end`, `G`: an Anfang oder Ende springen
- `enter`: ans Log-Ende springen oder in der Auswahl die Detailansicht oeffnen
- `enter` im Commands-Pane: ausgewaehltes Command im Command-Output-Pane starten
- `o` im Commands-Pane: ausgewaehltes Command als separaten Stream starten
- `enter` im Streams-Pane: ausgewaehlten on-demand Stream starten oder laufenden Stream stoppen
- `o` im Streams-Pane: Ausgabe eines einzelnen Streams modal anzeigen
- `t`: Logbuffer leeren und wieder ans Log-Ende springen
- `enter` zweimal schnell: visuellen Trenner einfuegen
- `n` / `N`: zum naechsten oder vorherigen sichtbaren Treffer
- `S`: Split-Richtung wechseln
- `esc` oder `enter` in der Detailansicht: zurueck zur Logansicht
- `esc`: Bottom-Query schliessen
- `y`: Share/Copy der aktuellen Zeile (Agent-friendly Snippet)
- `Y`: Share/Copy der aktuellen Zeile inkl. Kontext (+/- 20 Zeilen)
- `O`: erkannte Vite-URL im Browser oeffnen (wenn verfuegbar)
- `q`: App beenden
- `esc` dreimal: App beenden

## Query Syntax

- Freitext wie `heartbeat` matcht case-insensitive gegen die gesamte Logzeile.
- `key=value`-Filter wie `level=info` oder `lua-manager.resource=thread-example` matchen gegen logfmt-aehnliche Felder in der Zeile.
- Mehrere Tokens werden mit `AND` kombiniert, zum Beispiel `level=info heartbeat` oder `msg=heartbeat level=info`.
- Feldwerte matchen case-insensitive als Substring, also passt `msg=heart` auch auf `msg="thread example heartbeat"`.

Die Bottom-Query wirkt zusaetzlich auf das aktive Preset. Strukturierte OR-Filter werden in der Config-Datei ueber Presets gepflegt.

Der Live-Feldfilter unter `F` wirkt nur fuer die laufende Sitzung. Mehrere gesetzte Feldfilter werden mit `AND` kombiniert und matchen case-insensitive per Contains gegen erkannte logfmt-Felder.

## Structured Logging (logfmt + JSON Lines)

`iwatch` erkennt strukturierte Felder sowohl aus logfmt-aehnlichen `key=value` Tokens als auch aus **JSON Lines** (eine JSON pro Zeile).\n\nBeispiele:\n\n- logfmt:\n  - `level=INFO msg=\"hello\" request_id=abc`\n- JSON Lines:\n  - `{\"level\":\"INFO\",\"msg\":\"hello\",\"http\":{\"port\":8080}}`\n\nNested JSON wird als `dot.path` flach gemacht (z.B. `http.port=8080`) und ist dann im Filter nutzbar.

## Dev-Flow: Go Backend + Vite (Port Handshake)

Wenn du Go-Backend und Vite getrennt startest, kann `iwatch` den Backend-Port/URL aus den Logs erkennen und diese Info an den Vite-Devserver als ENV weitergeben.\n\nEmpfohlen:\n\n- Backend loggt structured `url=...` oder `port=...` (logfmt oder JSON Lines)\n- Vite wird als Stream mit `role: \"vite\"` konfiguriert, Backend als `role: \"backend\"`\n\nMinimal-Beispiel fuer `streams`:\n\n```json\n{\n  \"streams\": [\n    {\n      \"id\": \"backend\",\n      \"title\": \"Go backend\",\n      \"type\": \"process\",\n      \"role\": \"backend\",\n      \"cmd\": \"go run ./cmd/server\",\n      \"autoStart\": true\n    },\n    {\n      \"id\": \"vite\",\n      \"title\": \"Vite dev\",\n      \"type\": \"process\",\n      \"role\": \"vite\",\n      \"cmd\": \"npm run dev\",\n      \"autoStart\": false\n    }\n  ]\n}\n```\n\nSobald `iwatch` eine Backend-URL erkennt, setzt es beim Vite-Stream:\n\n- `BACKEND_URL`\n- `BACKEND_PORT` (wenn ableitbar)\n\nund startet den Vite-Stream (on-demand) automatisch.\n\nWenn `iwatch` die Vite-URL erkennt, kannst du sie mit `O` direkt oeffnen.

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
  "streams": [
    {
      "id": "app-log",
      "title": "App log",
      "type": "file",
      "source": "./logs/app.log"
    },
    {
      "id": "worker",
      "title": "Worker",
      "type": "process",
      "cmd": "npm run worker",
      "cwd": "./",
      "autoStart": false
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
        "streams": ["app-log", "worker"],
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
      "hiddenFields": ["debug"],
      "showRawMessage": true,
      "showSource": false,
      "showTimestamp": true,
      "timeFormat": "date-short",
      "wrapMode": "field",
      "palette": "default"
    }
  }
}
```

### Preset-Struktur

- `ui.presets[*].clauses` ist eine OR-Liste.
- `clauses[*].conditions` innerhalb einer Clause werden mit AND kombiniert.
- Bedingungen mit `field` matchen gegen erkannte logfmt-Felder.
- Bedingungen ohne `field` matchen als Freitext gegen die komplette Logzeile.
- `ui.presets[*].streams` aktiviert gezielt Stream-IDs fuer dieses Preset. Wenn kein Preset ueberhaupt `streams` definiert, gelten alle konfigurierten Streams als aktiv; `autoStart: false` bleibt dabei on-demand.

### LogStreams

- Root-`streams` definiert zusaetzliche Eingaben fuer die globale LogView.
- `type: "file"` liest `source` als Datei- oder Glob-Pfad. Beim Start springt der Stream ans aktuelle Dateiende und liest nur neue Zeilen. Truncate, Delete oder Recreate werden toleriert; nach Cleanup wird wieder ab Dateiende weitergelesen.
- `type: "process"` startet `cmd` mit optionalem `cwd` als Hintergrundprozess. `autoStart: false` macht den Stream on-demand startbar im Streams-Pane.
- Nur Streams, die im aktiven Preset genannt sind und laufen, schreiben in den globalen Buffer. Dadurch wirken Bottom-Query, globale Suche, Feldfilter, Highlighting und Detailansicht unveraendert auch auf Stream-Zeilen.

### Log View

- `timeFormat`: `time`, `date-short`, `relative`
  Bei `date-short` und `relative` wird keine separate Zeitspalte gerendert; die feste Zeitspalte ist nur fuer `time` sichtbar.
- `wrapMode`: `off`, `simple`, `field`
- `palette`: `default`, `contrast`, `ocean`, `forest`, `ember`
  In `default` wird die Zeit dunkelgrau, der logfmt-Key hellgrau und der Wert weiss gerendert.
- `visibleFields`: steuert die bevorzugte Reihenfolge erkannter logfmt-Felder; neue Felder werden automatisch angehaengt
- `hiddenFields`: blendet erkannte logfmt-Felder aus; das Feld-Menue schreibt diese Liste erst nach explizitem Speichern in die Config
- Root-`highlightRules` bleiben als Rueckwaertskompatibilitaets-Fallback aktiv, wenn ein Preset keine eigenen Regeln hat
