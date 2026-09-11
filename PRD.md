# Product Requirements Document — llama-swap TUI

## Overview

A terminal-based user interface (TUI) for monitoring and controlling a llama-swap instance running at `http://localhost:8080`. Replaces the browser-based web UI with a lightweight, keyboard-driven terminal experience.

## Problem

llama-swap ships with a web UI for monitoring model states, activity logs, in-flight requests, and hardware stats. Users managing local LLM inference from the terminal need to open a browser, which is friction — especially on headless servers or when working exclusively in tmux/terminal workflows.

## Goals

- Provide real-time visibility into llama-swap activity from the terminal
- Allow model load/unload and request cancellation without leaving the terminal
- Mirror the core value of the web UI: see what's happening, right now
- Stay lightweight — no browser, no build step, one binary

## Non-Goals

- Chat/playground interface (sending prompts to models)
- Image generation, audio, or other media workflows
- Config editing (profile management, model config)
- Mobile or remote access (terminal-bound)

## Target User

A developer or researcher running llama-swap locally who lives in the terminal and wants quick situational awareness of their model swap proxy.

---

## Functional Requirements

### FR-1: Connection Management

- Connect to a llama-swap instance at a configurable host (default `http://localhost:8080`)
- Accept host via CLI flag (`--host`), env var (`LLAMA_SWAP_URL`), or config file
- Maintain a persistent SSE connection to `/api/events` for real-time updates
- Auto-reconnect with exponential backoff on SSE disconnect (max 5s delay)
- Display connection status indicator in the status bar (connected / connecting / disconnected)
- Fall back to polling REST endpoints if SSE cannot be established

### FR-2: Activity Tab

- Display recent API requests in a sortable, filterable table
- Columns: ID, Time, Model, Path, Status, Input Tokens, Output Tokens, Cache Tokens, Prompt Speed, Gen Speed, Duration
- Auto-refresh on `activity` SSE events (new row appended)
- Paginated loading via `GET /api/metrics/activity` (default 50 rows, newest first)
- Filter by model name
- Filter by time range
- Sort by any column (click or key binding)
- Click a row to see request details (captured request/response if available)
- Aggregate stats header: total requests, total input/output/cache tokens

### FR-3: Models Tab

- Display all configured models with their current state
- Columns: Name, ID, State, Description, Peer (if applicable)
- Color-coded state indicator: green (ready), yellow (starting/stopping), gray (stopped)
- Load a model: `POST /api/models/unload` triggers state change
- Unload a model: `POST /api/models/unload/{model}`
- Cancel in-flight request: `POST /api/inflight/{id}/cancel`
- Real-time state updates via `modelStatus` SSE events
- Show selector models (strategy, targets, spillover) and peer models

### FR-4: Hardware Tab

- Display current CPU, memory, GPU stats from `GET /api/hardware`
- Show: CPU model, cores, memory total/used, GPU model, VRAM used/total, GPU util%, temperature, power draw
- Refresh on `perfsys` and `perfgpu` SSE events (or poll every 5s)
- Simple text-based bar charts for utilization percentages

### FR-5: Logs Tab

- Stream proxy and upstream logs via `GET /logs/stream`
- Filter by source: all, proxy, upstream, or specific model
- Color-coded log levels (info, warn, error)
- Live scrolling as new lines arrive
- Search/filter text within logs

### FR-6: Profiles Tab

- List all configured profiles via `GET /api/profiles`
- Show currently active profile
- Switch active profile via `PUT /api/profiles/active`
- Real-time update on `profileChanged` SSE event

### FR-7: Navigation & Controls

- Tab-based navigation between views (Activity, Models, Hardware, Logs, Profiles)
- Status bar at bottom showing: connection status, version, active profile
- Keyboard shortcuts:
  - `q` / `Ctrl+C` — quit
  - `1-5` — switch tabs
  - `r` — refresh current view
  - `/` — search/filter
  - `?` — show help overlay

---

## Technical Requirements

### TR-1: Language & Framework

- **Language**: Go 1.22+
- **TUI Framework**: [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) — MVU pattern, widely adopted, excellent text rendering
- **HTTP Client**: Standard library `net/http` with `EventSource`-like SSE handling
- **Dependency Management**: Go modules (`go.mod`)

### TR-2: Architecture

```
main.go                    # Entry point, CLI flags
├── api/
│   ├── client.go          # SSE connection manager + REST client
│   └── types.go           # Request/response type definitions
├── tui/
│   ├── app.go             # Main Bubbletea model, tab nav, status bar
│   ├── activity.go        # Activity table view
│   ├── models.go          # Models list view
│   ├── hardware.go        # Hardware stats view
│   ├── logs.go            # Log stream view
│   └── profiles.go        # Profile management view
└── go.mod
```

### TR-3: SSE Client

- Implement SSE reader using `EventSource` protocol (text/event-stream)
- Parse `APIEventEnvelope` with type dispatch:
  - `modelStatus` → update models list
  - `logData` → append to log buffer
  - `activity` → trigger activity refresh
  - `inflight` → update in-flight request tracker
  - `uiConfig` → update UI config
  - `profileChanged` → update active profile
  - `perfsys` / `perfgpu` → update hardware stats
- Maintain connection lifecycle: connect, reconnect, close

### TR-4: REST API Coverage

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/events` | GET (SSE) | Real-time event stream |
| `/api/metrics/activity` | GET | Paginated activity log |
| `/api/metrics/stats` | GET | Aggregate statistics |
| `/api/hardware` | GET | Hardware snapshot |
| `/api/performance` | GET | Historical performance |
| `/api/profiles` | GET | Profile list |
| `/api/profiles/active` | PUT | Switch profile |
| `/api/version` | GET | Build info |
| `/api/models/unload` | POST | Unload all models |
| `/api/models/unload/{model}` | POST | Unload specific model |
| `/api/inflight/{id}/cancel` | POST | Cancel request |
| `/v1/models` | GET | OpenAI-compatible model list |
| `/logs/stream` | GET (SSE) | Live log stream |

### TR-5: Data Types (Go equivalents of TS types)

- `Model` — id, state, name, description, unlisted, peerID, capabilities, aliases, strategy, targets, spillover
- `ActivityLogEntry` — id, timestamp, src, model, req_path, resp_content_type, resp_status_code, tokens, duration_ms, has_capture, error_msg, metadata
- `TokenMetrics` — cache_tokens, draft_tokens, draft_acc_tokens, input_tokens, output_tokens, prompt_per_second, tokens_per_second
- `InflightRequestEntry` — id, timestamp, model, req_path, method, req_headers, remote_ip, resp_headers, resp_bytes, elapsed_ms
- `HardwareSnapshot` — cpu, memory, accelerators (GPU), architecture, os, environment
- `ActivityStats` — total_requests, total_input/output/cache_tokens, histograms
- `Profile` / `ProfileState` — active, profiles list

---

## Acceptance Criteria

1. User can run `llama-swap-tui --host http://localhost:8080` and see the Activity tab populated with recent requests
2. Model states update in real-time as swaps happen (no manual refresh needed)
3. User can load and unload models from the Models tab
4. User can cancel in-flight requests from the Models tab
5. Hardware tab shows current GPU/CPU stats with visual utilization bars
6. Logs tab streams live proxy/upstream logs with source filtering
7. Profiles tab shows and allows switching active profiles
8. Connection status is always visible; reconnection works after network blips
9. The TUI quits cleanly on `q` or `Ctrl+C`, closing SSE connections gracefully
