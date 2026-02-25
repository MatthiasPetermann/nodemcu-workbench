# NodeMCU Workbench

`nodemcu-workbench` is a terminal-based, low-barrier all-in-one app for NodeMCU/ESP8266.

The app grew out of real-world work with children: the goal was to build a tool that enables **simple, integrated, and accessible development** — without tool sprawl, without complex setup, and with clear keyboard-driven workflows.

Design principles:

- **Standalone package** instead of many separate tools
- **Firmware embedded directly**, so flashing works out of the box
- **Minimal project management** via a file manager (local + on-device filesystem)
- **Integrated REPL** for fast experimentation in the same program

Context / background:

- Blog post: <https://www.petermann-digital.de/blog/nodemcu-from-scratch/>

---

## Overview

The app combines three modes in a single TUI program:

1. **Workbench** – local file manager + NodeMCU files + upload/delete
2. **Terminal** – interactive REPL with continuation handling
3. **Maintenance** – identify, erase, flash with embedded firmware

---

## Current feature set (code-aligned)

### 1) Workbench mode

- Two-pane view:
  - **Left:** local filesystem
  - **Right:** files on the NodeMCU filesystem (`file.list()`)
- Local file operations:
  - Enter directories / go one level up
  - Create new files
  - Rename files or folders
  - Create new directories
  - Delete files/folders (with confirmation)
  - Edit files in `nano`
- Upload:
  - Upload selected local file to the board
- Remote delete:
  - Delete selected file in the right pane (with confirmation)

### 2) Terminal mode

- Interactive REPL console with input line + scrollable viewport
- Automatic handling of Lua continuation (`>>`)
- `Ctrl+C` interrupts running multi-line input
- `Ctrl+L` clears the output
- `Ctrl+R` syncs/reconnects the session

### 3) Maintenance mode

- Actions:
  - **Identify Device** (chip + MAC)
  - **Erase Flash** (full flash erase)
  - **Flash Firmware** (2 segments)
- Embedded firmware segments:
  - `0x00000.bin` @ `0x00000000`
  - `0x10000.bin` @ `0x00010000`
- Live progress reporting in the status line
- Exclusive serial port access during maintenance actions

---

## Keyboard controls

> Note: the keybar in the UI displays shortcuts in the form `^X`.

### Global

- `Ctrl+W` – switch mode (`Workbench -> Terminal -> Maintenance`)
- `Ctrl+X` – quit the program

### Workbench

- `Tab` – switch active pane
- `↑ / ↓` – move selection
- `Enter` – open directory (left pane)
- `Backspace` – go up one level (left pane)
- `Ctrl+R` – refresh both sides
- `Ctrl+E` – open selected local file in `nano`
- `Ctrl+O` – upload selected local file
- `Ctrl+T` – rename locally
- `Ctrl+N` – create new file
- `Ctrl+G` – create new local directory
- `Ctrl+K` – delete (local or remote depending on active pane)

### Terminal

- `Enter` – send line to REPL
- `Ctrl+C` – interrupt multi-line input
- `Ctrl+L` – clear output
- `Ctrl+R` – sync/reconnect REPL
- `PgUp / PgDown / ↑ / ↓` – scroll

### Maintenance

- `← / → / ↑ / ↓` – select action
- `Enter` or `Ctrl+O` – run selected action
- Destructive actions require confirmation

---

## Requirements

- Go **1.22+**
- Linux/macOS with a serial NodeMCU/ESP connection
- Optional: `nano` (for editing in Workbench)

---

## Build

```bash
make build
```

Binary:

```text
bin/nodemcu-workbench
```

Alternative:

```bash
go build -o bin/nodemcu-workbench
```

---

## Run

Default port: `NODEMCU_PORT`, fallback `/dev/ttyUSB0`.
Baud rate: `115200`.

```bash
NODEMCU_PORT=/dev/ttyUSB0 ./bin/nodemcu-workbench
```

If a connection cannot be established, the UI still starts; affected functions then report `not connected`.

---

## Embedding firmware (Maintenance)

Before building, the firmware files must exist:

```text
modes/maintenance/embedded/0x00000.bin
modes/maintenance/embedded/0x10000.bin
```

These files are embedded into the binary via `go:embed`.

Optional configuration:

- `NODEMCU_BOOT_BIN` (default: `0x00000.bin`)
- `NODEMCU_APP_BIN` (default: `0x10000.bin`)

> The app always resolves the **basename** within the embedded `embedded/` directory.

---

## Project structure

- `main.go` – app startup, global routing, mode switching, keybar
- `modes/workbench` – file manager + upload/delete
- `modes/terminal` – REPL UI with continuation handling
- `modes/maintenance` – identify/erase/flash
- `repl/session.go` – serial session, prompt detection (`>`, `>>`)
- `ui/` – theme, header, status line, layout components

---

## Safety

- **Erase Flash** wipes the entire device flash.
- **Flash Firmware** overwrites firmware regions (`0x0`, `0x10000`).
- Verify port/board before running maintenance actions.

---

## License

See [`LICENSE`](./LICENSE).
