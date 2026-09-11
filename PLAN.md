# Implementation Plan — llama-swap TUI

## Phase 1: Project Bootstrap

- [ ] **1.1** Initialize Go module (`go mod init llama-swap-tui`)
- [ ] **1.2** Add bubbletea dependency (`go get github.com/charmbracelet/bubbletea`)
- [ ] **1.3** Create directory structure: `api/`, `tui/`
- [ ] **1.4** Create `api/types.go` — Go type definitions mirroring the TS types from the frontend
- [ ] **1.5** Create `api/client.go` — SSE connection manager + REST client

## Phase 2: Core TUI Shell

- [ ] **2.1** Create `main.go` — CLI flag parsing (`--host`, `--help`), env var fallback, entry point
- [ ] **2.2** Create `tui/app.go` — Main Bubbletea model with tab navigation, status bar, keyboard shortcuts
- [ ] **2.3** Wire up the SSE client to the app's update loop
- [ ] **2.4** Implement connection status indicator (connected/connecting/disconnected)

## Phase 3: Activity Tab

- [ ] **3.1** Create `tui/activity.go` — Activity table view
- [ ] **3.2** Fetch paginated activity from `GET /api/metrics/activity`
- [ ] **3.3** Display columns: ID, Time, Model, Path, Status, Tokens In/Out, Cache, Speeds, Duration
- [ ] **3.4** Auto-refresh on `activity` SSE events
- [ ] **3.5** Model filter and sort-by-column support
- [ ] **3.6** Aggregate stats header from `GET /api/metrics/stats`

## Phase 4: Models Tab

- [ ] **4.1** Create `tui/models.go` — Models list view
- [ ] **4.2** Display models from `modelStatus` SSE events
- [ ] **4.3** Color-coded state indicators (ready/starting/stopping/stopped)
- [ ] **4.4** Load model action (`GET /upstream/{model}`)
- [ ] **4.5** Unload model action (`POST /api/models/unload/{model}`)
- [ ] **4.6** Cancel in-flight request action (`POST /api/inflight/{id}/cancel`)
- [ ] **4.7** Show in-flight requests from `inflight` SSE events

## Phase 5: Hardware Tab

- [ ] **5.1** Create `tui/hardware.go` — Hardware stats view
- [ ] **5.2** Fetch from `GET /api/hardware`
- [ ] **5.3** Display CPU, memory, GPU stats
- [ ] **5.4** Text-based utilization bar charts
- [ ] **5.5** Refresh from `perfsys`/`perfgpu` SSE events (poll fallback every 5s)

## Phase 6: Logs Tab

- [ ] **6.1** Create `tui/logs.go` — Log stream view
- [ ] **6.2** Connect to `GET /logs/stream` for live log streaming
- [ ] **6.3** Filter by source (proxy/upstream/model)
- [ ] **6.4** Color-coded log levels
- [ ] **6.5** Live scrolling buffer (max ~500 lines, discard old)

## Phase 7: Profiles Tab

- [ ] **7.1** Create `tui/profiles.go` — Profile management view
- [ ] **7.2** Fetch profiles from `GET /api/profiles`
- [ ] **7.3** Switch profile via `PUT /api/profiles/active`
- [ ] **7.4** Update from `profileChanged` SSE events

## Phase 8: Polish

- [ ] **8.1** Help overlay (`?` key) showing all keyboard shortcuts
- [ ] **8.2** Version display in status bar from `GET /api/version`
- [ ] **8.3** Graceful shutdown (close SSE, cancel HTTP requests)
- [ ] **8.4** Build test: `go build -o llama-swap-tui .`
