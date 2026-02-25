# NodeMCU Workbench

`nodemcu-workbench` ist eine terminalbasierte All-in-One-Oberfläche für NodeMCU/ESP-Geräte.
Die App kombiniert drei Arbeitsbereiche in einem TUI-Programm:

1. **Workbench** (Dateiverwaltung + Upload auf den NodeMCU-Dateisystembereich)
2. **Terminal** (interaktive REPL)
3. **Maintenance** (Geräteerkennung, Flash löschen, Firmware flashen)

Die Bedienung ist an klassische 2-Pane-Dateimanager angelehnt und auf schnelle Tastatur-Workflows ausgelegt.

---

## Funktionsumfang

### 1) Workbench-Modus

- Zweispaltige Ansicht:
  - **Links:** Lokales Dateisystem
  - **Rechts:** Dateien auf dem NodeMCU (`file.list()`)
- Lokale Dateiverwaltung:
  - Verzeichnisse öffnen / eine Ebene zurück
  - Neue Datei anlegen
  - Datei/Ordner umbenennen
  - Neues Verzeichnis erstellen
  - Datei/Ordner löschen (mit Bestätigung)
  - Datei in `nano` bearbeiten
- Upload:
  - Auswahl im linken Pane mit **F5** direkt auf den NodeMCU hochladen
  - Upload über REPL (`file.open`, `f:write`, `f:close`) in kleinen Chunks
- Remote-Löschen:
  - Datei im rechten Pane auswählen und mit **F8** löschen (mit Bestätigung)

### 2) Terminal-Modus

- Interaktive REPL-Konsole mit Eingabefeld und Scroll-Viewport
- Unterstützt Lua-Continuation (`>>`) automatisch
- **Ctrl+C** unterbricht laufende Eingaben
- **F2** leert die Anzeige
- **F5** synchronisiert/reconnectet die bestehende Session

### 3) Maintenance-Modus

- Kachelansicht für Service-Aktionen:
  - **Identify Device** (Chip + MAC auslesen)
  - **Erase Flash** (kompletten Flash löschen)
  - **Flash Firmware** (zwei Segmente flashen)
- Flashing nutzt eingebettete Firmware-Dateien:
  - `0x00000.bin` @ `0x00000000`
  - `0x10000.bin` @ `0x00010000`
- Fortschritt wird im Statusbereich live angezeigt
- Serielle Schnittstelle wird exklusiv genutzt, damit REPL und Flasher sich nicht in die Quere kommen

---

## Bedienung (global)

- **F9**: Zwischen Modi wechseln (`Workbench -> Terminal -> Maintenance`)
- **F10** oder **Ctrl+C**: Programm beenden

Die Funktionsleiste unten zeigt je Modus die verfügbaren Aktionen an.

---

## Tastenkürzel nach Modus

### Workbench

- `Tab` aktives Pane wechseln
- `↑/↓` Auswahl bewegen
- `Enter` Verzeichnis öffnen (nur links)
- `Backspace` Verzeichnis nach oben (nur links)
- `Ctrl+N` neue Datei anlegen (nur links)
- `F2` beide Seiten aktualisieren
- `F4` ausgewählte lokale Datei mit `nano` öffnen
- `F5` ausgewählte lokale Datei hochladen
- `F6` lokal umbenennen
- `F7` lokales Verzeichnis erstellen
- `F8` löschen (lokal oder remote, je nach aktivem Pane)

### Terminal

- `Enter` Zeile an REPL senden
- `Ctrl+C` laufende/mehrzeilige Eingabe abbrechen
- `F2` Log-Ausgabe leeren
- `F5` REPL neu synchronisieren
- `PgUp/PgDown`, `↑/↓` scrollen

### Maintenance

- `↑/↓` oder `←/→` Aktion wählen
- `Enter` / `F5` / `F8` gewählte Aktion starten
- Bei destruktiven Aktionen erfolgt eine Bestätigung

---

## Voraussetzungen

- Go **1.22+**
- Linux/macOS mit serieller NodeMCU/ESP-Verbindung
- Optional: `nano` (für `F4` im Workbench-Modus)

---

## Build

```bash
make build
```

Erzeugt das Binary unter:

```text
bin/nodemcu-workbench
```

Alternativ direkt mit Go:

```bash
go build -o bin/nodemcu-workbench
```

---

## Start

Standardmäßig wird der Port aus `NODEMCU_PORT` gelesen, ansonsten `/dev/ttyUSB0` verwendet.
Baudrate ist aktuell fest auf `115200`.

```bash
NODEMCU_PORT=/dev/ttyUSB0 ./bin/nodemcu-workbench
```

Wenn keine Verbindung aufgebaut werden kann, startet die UI trotzdem; Funktionen, die eine Session brauchen, melden dann entsprechend „not connected“.

---

## Firmware für Maintenance einbetten

Für den Flash-Modus müssen die Firmware-Dateien im Repository vorhanden sein, bevor du baust:

```text
modes/maintenance/embedded/0x00000.bin
modes/maintenance/embedded/0x10000.bin
```

Die Dateien werden beim Build eingebettet (`go:embed`).

Optional kannst du die Dateinamen/-pfade über Umgebungsvariablen umbiegen:

- `NODEMCU_BOOT_BIN` (Default: `0x00000.bin`)
- `NODEMCU_APP_BIN` (Default: `0x10000.bin`)

> Hinweis: Es wird immer nach dem **Basename** im eingebetteten `embedded/`-Verzeichnis gesucht.

---

## Projektstruktur (Kurzüberblick)

- `main.go` – App-Start, Layout, globales Routing, Moduswechsel
- `modes/workbench` – Dateimanager + Upload/Löschen
- `modes/terminal` – REPL-UI mit Continuation-Handling
- `modes/maintenance` – ESP-Bootloader-Aktionen (Identify/Erase/Flash)
- `repl/session.go` – serielle REPL-Session inkl. Prompt-Erkennung (`>`, `>>`)
- `ui/` – Theme, Header, Statusline, Keybar, Layout-Bausteine

---

## Sicherheitshinweise

- **Erase Flash** löscht den Gerätespeicher vollständig.
- **Flash Firmware** überschreibt Firmwarebereiche (`0x0`, `0x10000`).
- Vor Wartungsaktionen prüfen, dass der richtige Port gewählt ist.

---

## Lizenz

Siehe [`LICENSE`](./LICENSE).
