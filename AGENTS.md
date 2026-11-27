# PROJECT KNOWLEDGE BASE

**Generated:** 2026-09-26 (updated 2026-11-11)

## OVERVIEW
Project: **llama-swap-tui**
Stack: Go 1.24.2 · Bubbletea (Charm) · Lipgloss · Bubbles

A terminal UI (TUI) for [llama-swap](https://github.com/mostlygeek/llama-swap) — monitor model swaps, request activity, hardware stats, in-flight requests, GPU metrics, and more from your terminal.

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
│   ├── performance_test.go  # Perf cursor unit tests + live integration test
│   ├── models_layout_test.go # Models tab layout/clipping, fetch-cmd race, line budgets
│   ├── tick_test.go     # Auto-refresh tick delivery/rescheduling tests
│   └── header_conn_test.go   # Header conn dot / reconnect state tests
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
| Smoke test | `./scripts/smoke-test.sh` (builds the binary and the mock itself) |
| Build   | `go build -o llama-swap-tui .` |
| Vet     | `go vet ./...` |
| Lint    | `golangci-lint run ./...` (config: `.golangci.yml`, pinned to v2.15.0 in CI) |
| Run     | `./llama-swap-tui` or `LLAMA_SWAP_URL=http://host:8080 ./llama-swap-tui` |

CI runs on PRs to master via `.github/workflows/ci.yml` (format check, `go vet`, `go build`, `go test -v`, a smoke test that boots the binary against a mock server, and a golangci-lint job — see LINT below). Tagged releases (`v*.*.*`) publish binaries via `.github/workflows/release.yml` (GoReleaser). Weekly releases are tagged automatically by `.github/workflows/auto-tag.yml` (see AUTO-TAGGING below).

## CODING STANDARDS
*   **Language**: Go 1.24.2. Standard library imports first, then third-party, then project-local (`llama-swap-tui/api`, `llama-swap-tui/tui`).
*   **Style**: Bubbletea MVU pattern (Model-View-Update). Each tab is a separate file. Messages (types implementing `bubbletea.Message`) carry data between goroutines and the update loop.
*   **Formatting**: Standard `go fmt` / `gofmt`. No external linter/formatter configured.
*   **Naming**: `NewModel`, `renderActivityView`, `fetchPerformance` — verb-noun for functions; PascalCase for exported types/functions; camelCase for unexported.
*   **Error routing**: `Cmd` failures travel as two struct message types, never as a bare `error`.
    *   `SSEErrMsg{err error}` — sent only from `sseReader`; sets `conn = connDisconnected` and `errMsg`.
    *   `FetchErrMsg{err error}` — returned by every REST poll Cmd; deliberately **discarded** by `Update`. Polls keep the last good data, the 1s tick retries, and SSE alone decides connectivity.
    Both are structs on purpose: the old `type SSEErrMsg error` was an interface type, so `case SSEErrMsg:` matched *any* error value — a single REST 404 disconnected the app and blanked the Activity/Models tabs. Never send a bare `error` as a `tea.Msg`. `log.SetOutput(os.Stderr)` in `main.go` only covers startup/`--host` failures; poll errors are never logged, so a permanently failing endpoint renders as an empty tab with no message.
*   **Commit convention**: Use Conventional Commits format — `type(scope): description`. Types: `fix`, `feat`, `docs`, `chore`. Scopes: `tui`, `api`, `activity`, `models`, `hardware`, `status`, `ci`. Always use the project's existing scope names (e.g. `tui` for general TUI changes, not `status` or `dashboard`).

## TESTS
*   `tui/hscroll_test.go` — Unit tests for horizontal scrolling with ANSI escape sequences, offset clamping, and adaptive column rendering.
*   `tui/performance_test.go` — `httptest`-based cursor tests (query escaping, cross-offset instant ordering, no rewind on stale/unparseable stamps) plus `TestFetchPerformanceLive`, a live integration test that hits a running llama-swap at `localhost:8080` and uses `t.Skipf` when the backend is unreachable.
*   `tui/models_layout_test.go` — Models tab clipping/layout, fetch cmd race safety, tab line-count budgets, activity header alignment, window-height fit (incl. error state).
*   `tui/tick_test.go` — Auto-refresh tick delivers `time.Time` messages and reschedules itself.
*   `tui/header_conn_test.go` — Header connection dot color/rendering, stale error cleared on reconnect, and REST poll failures not marking the app disconnected.

## SMOKE TESTS
`scripts/smoke-test.sh` boots the real binary against `cmd/mock-llama-swap` (a fake llama-swap serving the boot-time endpoints with canned data: version, models, activity, stats, hardware, profiles, performance, tailcat, the SSE event stream, and the log stream) and asserts, over tmux pane captures: the title row shows the app and host, the status bar shows connected with the mock API version and the binary version, no `Error:` line appears, tab keys 1-6 all render (Models lists the mock model, Logs shows the mock log line), and `q` quits cleanly. Flags: `--binary`/`--mock` to skip building, `--port`, `--expect-version` (defaults to `dev`; the post-release workflow passes the release tag).

Runs in CI on every PR (the `smoke` job in `.github/workflows/ci.yml`) and after every published release (`.github/workflows/smoke-release.yml`, which downloads the Linux amd64 asset and re-runs the same script against it). The mock also serves one activity row with a capture (`capture_id: 26112716-a7Kq`) and a canned `/api/captures/` endpoint (any other ID 404s, like a pruned capture), so the script can exercise the Activity detail view: Enter shows the prompt and reply, Esc closes it. If the post-release smoke fails, the workflow's `rollback` job marks the release as a pre-release so it stops being the latest release, and opens a tracking issue; the tag and artifacts stay in place, and the fix ships as a new release. The TUI needs a real TTY, so the script runs it inside tmux at 128x32; install tmux explicitly in CI rather than assuming the runner image ships it.

## LINT
`golangci-lint run ./...` (config `.golangci.yml`, default linter set listed explicitly: errcheck, govet, ineffassign, staticcheck, unused). The CI `lint` job runs the same config via `golangci/golangci-lint-action@v9` pinned to v2.15.0. One deliberate exclusion: staticcheck QF1012 is off, because the codebase builds rendered output with `b.WriteString(fmt.Sprintf(...))` in ~30 places and rewriting them all buys nothing. Note golangci-lint caps identical-text issues at 3 per run (`max-same-issues`), so a first run can hide more of the same: fix what it reports, then re-run until it is clean.

## AUTO-TAGGING
`scripts/auto-tag.sh` tags the next release from merged PRs; `.github/workflows/auto-tag.yml` runs it every Sunday at 16:17 HKT (08:17 UTC), and it can also be run manually from the Actions tab (with a dry-run option). It reads the squash-merged PR titles since the last tag (`git log` subjects ending in ` (#N)`), bumps SemVer from them (`feat` -> minor, anything else -> patch, `type!` or `BREAKING CHANGE` -> major), and pushes an annotated tag, which triggers the release workflow. Weeks with no merged PRs tag nothing; if the computed tag already exists it exits quietly. With no tags at all the first tag is `v1.0.0`. Run it locally with `bash scripts/auto-tag.sh --dry-run`.

Setup: the workflow needs a repository secret named `RELEASE_PAT` holding a fine-grained PAT with Contents write. The tag must be pushed with that token, not the built-in `GITHUB_TOKEN`: pushes made with `GITHUB_TOKEN` do not trigger other workflows, so the release workflow would never fire. The workflow fails fast with a clear error when the secret is missing.

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
*   **Activity capture detail** (issue #26): Enter on a selected Activity row expands a detail panel below the table showing the input prompt and output from `GET /api/captures/{capture_id}` (the string capture ID, not the row's numeric ID: the server validates the ID shape). Esc closes it; j/k move the row selection on the Activity tab; moving selection, changing page, or leaving the tab closes the panel. The panel is keyed by entry ID so the 1s refresh keeps it on the same request. Parsing mirrors the web UI's CaptureDialog (chat messages become the input, SSE chunks reassemble into the output); see `tui/capture.go`.
*   **Horizontal scrolling**: `applyHScroll(content, offset, width)` handles ANSI-colored text width computation via `github.com/charmbracelet/x/ansi`.
*   **Auto-refresh**: 1s tick via `time.Time` message in update loop. Activity tab fetches `fetchActivity()`, Status dashboard tab fetches `fetchActivityPage()`, Hardware tab fetches `fetchHardware()`, GPU/system always fetches `fetchPerformance()`. Performance fetches are incremental: the last-seen perf timestamp is echoed back as an RFC3339 `?after=` cursor (`perfCursor` in the model), percent-encoded via `Client.GetPerformance(ctx, after)` — the server rejects malformed cursors with HTTP 400. `perfCursor` is a `*time.Time` formatted `RFC3339Nano` and compared as an **instant**, never as text (mixed `+01:00`/`Z` stamps sort wrong lexically), and it only ever advances — a stale or unparseable entry must not rewind it to the epoch and refetch the whole ring buffer.
*   **Connection status**: Header shows a colored `●` dot (green = connected, amber = disconnected/connecting). Only SSE disconnects mark the app disconnected — REST poll failures do NOT. Stale disconnect errors are cleared on reconnect so they don't linger in the status bar.
*   **Timezone**: All timestamps use `.Local()` on `time.Time` to reflect system timezone (not UTC).

## WHERE TO LOOK
*   **Source**: `tui/` (UI logic), `api/` (HTTP/SSE client)
*   **Tests**: `tui/*_test.go`
*   **Entry point**: `main.go`
*   **Style**: `tui/style.go`

## NOTES
*   Version is injected at build time: `var version = "dev"` in `main.go`, overridden by GoReleaser via `-X main.version=v{{.Version}}` on tagged releases (see `.goreleaser.yaml` and `.github/workflows/release.yml`).
*   **Status Dashboard** (PR #3): New tab with 4 cards — In-flight requests (with elapsed bars), Loaded Models (state breakdown), GPU (VRAM/util/power bars), Recent Activity (last 10 requests, auto-refreshing every 1s).
*   The binary `llama-swap-tui` is built locally from source; it is gitignored and **not** tracked (earlier revisions of this file wrongly claimed it was committed).
*   Known gap: `/api/performance` answers HTTP 503 `{"enabled": false}` when `performance.disabled: true`. `PerformanceResponse` has no `Enabled` field and `doJSON` rejects non-200, so monitoring-disabled servers show empty Hardware/GPU sections with no explanation — and silently, since `FetchErrMsg` is discarded.
*   `.tmp/web/` is a transient scratch directory (ignored).
*   The project is MIT licensed.
*   No `.env` files or secrets are used — connection URL comes from `--host` flag or `LLAMA_SWAP_URL` env var.
