# PROJECT KNOWLEDGE BASE

**Generated:** 2026-09-26

## OVERVIEW
Project: **llama-swap-tui**
Stack: Go 1.24.2 · Bubbletea (Charm) · Lipgloss · Bubbles

A terminal UI (TUI) for [llama-swap](https://github.com/binfelipe/llama-swap) — monitor model swaps, request activity, hardware stats, in-flight requests, GPU metrics, and more from your terminal.

## STRUCTURE
```
llama-swap-tui/
├── main.go              # Entry point, CLI flags, signal handling
├── api/
│   ├── types.go         # Go type definitions for llama-swap API
│   └── client.go        # SSE client + REST client
├── tui/
│   ├── app.go           # Main Bubbletea model, view, key handling
│   ├── activity.go      # Activity tab rendering
│   ├── models.go        # Models tab rendering
│   ├── hardware.go      # Hardware tab rendering
│   ├── logs.go          # Logs tab rendering
│   ├── profiles.go      # Profiles tab rendering
│   ├── status.go        # Status dashboard tab (In-flight, Loaded Models, GPU, Recent Activity)
│   ├── style.go         # Lipgloss styling definitions
│   ├── hscroll_test.go  # Horizontal scroll unit tests
│   └── performance_test.go  # Live integration test
├── docs/screenshots/    # Screenshot images for README
├── scripts/
│   └── mkshot.py        # Screenshot renderer (tmux ANSI capture → PNG)
├── .tmp/web/            # Transient web state (ignored)
└── llama-swap-tui       # Built binary (ignored)
```

## COMMANDS
| Action | Command |
|--------|---------|
| Install | `go mod download` |
| Test    | `go test ./...` |
| Build   | `go build -o llama-swap-tui .` |
| Run     | `./llama-swap-tui` or `LLAMA_SWAP_URL=http://host:8080 ./llama-swap-tui` |

## CODING STANDARDS
*   **Language**: Go 1.24.2. Standard library imports first, then third-party, then project-local (`llama-swap-tui/api`, `llama-swap-tui/tui`).
*   **Style**: Bubbletea MVU pattern (Model-View-Update). Each tab is a separate file. Messages (types implementing `bubbletea.Message`) carry data between goroutines and the update loop.
*   **Formatting**: Standard `go fmt` / `gofmt`. No external linter/formatter configured.
*   **Naming**: `NewModel`, `renderActivityView`, `fetchPerformance` — verb-noun for functions; PascalCase for exported types/functions; camelCase for unexported.
*   **Error handling**: Errors returned as values from `Cmd` functions; logged via `log.SetOutput(os.Stderr)` in main.
*   **Commit convention**: Use Conventional Commits format — `type(scope): description`. Types: `fix`, `feat`, `docs`, `chore`. Scopes: `tui`, `api`, `activity`, `models`, `hardware`, `status`. Always use the project's existing scope names (e.g. `tui` for general TUI changes, not `status` or `dashboard`).

## TESTS
*   `tui/hscroll_test.go` — Unit tests for horizontal scrolling with ANSI escape sequences, offset clamping, and adaptive column rendering.
*   `tui/performance_test.go` — Live integration test that hits a running llama-swap instance at `localhost:8080`. Use `t.Skipf` when the backend is unreachable.

## SCREENSHOTS
Tab screenshots in `docs/screenshots/` are captured from a **live** TUI session: tmux → ANSI capture → Pillow render via `scripts/mkshot.py`. The TUI requires a real TTY (`/dev/tty`), so it cannot run headless in a plain subprocess.

1. Detached tmux session + fixed pane size. `new-session -x/-y` is ignored for detached sessions; only `resize-window` sizes the pane:
   `tmux new-session -c <repo> -d -s tui && tmux resize-window -t tui -x 128 -y 32 && tmux send-keys -t tui './llama-swap-tui' Enter`
2. Wait a few seconds for the initial SSE/REST fetch, then switch tabs with number keys — `1`=Status, `2`=Activity, `3`=Models, `4`=Hardware, `5`=Logs, `6`=Profiles — and wait ~2s (the 1s auto-refresh tick keeps redrawing the pane).
3. Capture each tab with ANSI colors (tmux 3.4 has no file argument; `-p` writes to stdout → redirect):
   `tmux capture-pane -t tui -e -p > /tmp/screenshots/<name>.ansi`
4. Render (DejaVu Sans Mono, 128 cols → 1280 px; reads `/tmp/screenshots/` by default):
   `python3 scripts/mkshot.py docs/screenshots <name>,...`
5. Verify by reading the PNG back with PIL: the title row `llama-swap-tui [host]` must be visible. The app fits output exactly into `winH` rows, so a tab that overflows loses its title to scroll (the Status tab had exactly this bug).

Note: the Logs tab tails `/logs/stream` live — if it's empty ("No logs yet"), trigger a model load/unload from the Models tab (`L` = load prompt) before capturing. Quit the TUI with `q` before killing the tmux session.

## ARCHITECTURE NOTES
*   **Two communication channels** to llama-swap:
    *   **SSE** (`/api/events`) — Real-time stream with 8 event types: `modelStatus`, `logData`, `activity`, `inflight`, `uiConfig`, `profileChanged`, `perfsys`, `perfgpu`.
    *   **REST** (`/api/...`) — HTTP GET/POST/PUT for state fetch and mutation.
*   **Goroutine-to-update-loop bridge**: `model.SetProgram(program)` wires the Bubbletea program reference into the model so SSE reader and log stream goroutines can send messages via `program.Send()`.
*   **Signal handling**: `SIGINT`/`SIGTERM` → `ShutdownMsg{}` sent into the update loop for graceful shutdown.
*   **Horizontal scrolling**: `applyHScroll(content, offset, width)` handles ANSI-colored text width computation via `github.com/charmbracelet/x/ansi`.
*   **Auto-refresh**: 1s tick via `time.Time` message in update loop. Activity tab fetches `fetchActivity()`, Status dashboard tab fetches `fetchActivityPage()`, Hardware tab fetches `fetchHardware()`, GPU/system always fetches `fetchPerformance()`.
*   **Timezone**: All timestamps use `.Local()` on `time.Time` to reflect system timezone (not UTC).

## WHERE TO LOOK
*   **Source**: `tui/` (UI logic), `api/` (HTTP/SSE client)
*   **Tests**: `tui/*_test.go`
*   **Entry point**: `main.go`
*   **Style**: `tui/style.go`

## NOTES
*   Version is embedded at build time: `const version = "v1.3.0-auto-refresh"` in `main.go`.
*   **Status Dashboard** (PR #3): New tab with 4 cards — In-flight requests (with elapsed bars), Loaded Models (state breakdown), GPU (VRAM/util/power bars), Recent Activity (last 10 requests, auto-refreshing every 1s).
*   The binary `llama-swap-tui` is pre-built and tracked in git (10MB). It's ignored by `.gitignore` on re-push.
*   `.tmp/web/` is a transient scratch directory (ignored).
*   The project is MIT licensed.
*   No `.env` files or secrets are used — connection URL comes from `--host` flag or `LLAMA_SWAP_URL` env var.
