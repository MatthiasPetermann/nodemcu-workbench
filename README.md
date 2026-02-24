# nodemcu-workbench

Kid-friendly "Midnight Commander"-ish TUI to upload Lua files to NodeMCU **via the REPL**.

## Keys

- `Tab` switch pane (Local / NodeMCU)
- `↑/↓` move selection
- `Enter` open directory (Local pane)
- `Backspace` go up (Local pane)
- `F2` refresh remote list
- `F5` upload selected local file to NodeMCU (overwrite)
- `F8` delete selected remote file
- `Ctrl+C` cancel ongoing op / quit
- `q` quit

## Build & Run

```bash
go mod download
go build ./cmd/nodemcu-mc
./nodemcu-mc -port /dev/ttyUSB0 -baud 115200 -dir .
```

Notes:
- This tool waits for the `>` prompt after each Lua line.
- Upload uses `file.open`, `file.write` in small chunks, then `file.close`.

## Future Ideas

- Builtin terminal (like picocom)
- Builtin Lua editor with syntax highlightning
- Version control / history
- making this the NodeMCU IDE I always was looking for...
