#!/usr/bin/env bash
# Smoke test: boot the real llama-swap-tui binary against the mock server
# and prove it renders, stays connected, switches tabs, and quits cleanly.
#
# Usage:
#   ./scripts/smoke-test.sh [--binary PATH] [--mock PATH]
#                           [--port N] [--expect-version STR]
#
# With no --binary/--mock it builds both from source into a temp dir.
# --expect-version defaults to "dev" (what a local build reports); the
# post-release workflow passes the release tag instead.
#
# The TUI needs a real TTY, so it runs inside tmux (installed by the CI
# steps that call this script) at 128x32, the same size the screenshot
# script uses. Assertions are plain-text greps over captured panes.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT=18080
EXPECT_VERSION="dev"
BIN=""
MOCK=""

while [[ $# -gt 0 ]]; do
	case "$1" in
		--binary) BIN="$2"; shift 2 ;;
		--mock) MOCK="$2"; shift 2 ;;
		--port) PORT="$2"; shift 2 ;;
		--expect-version) EXPECT_VERSION="$2"; shift 2 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

HOST="http://127.0.0.1:${PORT}"
SESSION="smoke-$$"
WORKDIR="$(mktemp -d)"
MOCK_PID=""
FAILURES=0

cleanup() {
	tmux kill-session -t "$SESSION" 2>/dev/null || true
	if [[ -n "$MOCK_PID" ]]; then
		kill "$MOCK_PID" 2>/dev/null || true
	fi
	rm -rf "$WORKDIR"
}
trap cleanup EXIT

check() {
	local name="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		echo "PASS: $name"
	else
		echo "FAIL: $name"
		FAILURES=$((FAILURES + 1))
	fi
}

capture_contains() {
	# capture_contains <needle> — true when the current pane contains it.
	tmux capture-pane -t "$SESSION" -p 2>/dev/null | grep -qF "$1"
}

capture_lacks() {
	# capture_lacks <needle> — true when the current pane does not contain it.
	! tmux capture-pane -t "$SESSION" -p 2>/dev/null | grep -qF "$1"
}

# --- Build what was not supplied -------------------------------------------
cd "$REPO_ROOT"
if [[ -z "$BIN" ]]; then
	BIN="$WORKDIR/llama-swap-tui"
	echo "building llama-swap-tui..."
	go build -o "$BIN" .
fi
if [[ -z "$MOCK" ]]; then
	MOCK="$WORKDIR/mock-llama-swap"
	echo "building mock-llama-swap..."
	go build -o "$MOCK" ./cmd/mock-llama-swap
fi
[[ -x "$BIN" ]] || { echo "FAIL: binary is not executable: $BIN"; exit 1; }
[[ -x "$MOCK" ]] || { echo "FAIL: mock is not executable: $MOCK"; exit 1; }

# --- Start the mock, wait until it answers ---------------------------------
"$MOCK" -addr "127.0.0.1:${PORT}" >"$WORKDIR/mock.log" 2>&1 &
MOCK_PID=$!
READY=0
for _ in $(seq 1 20); do
	if curl -fsS "$HOST/api/version" >/dev/null 2>&1; then
		READY=1
		break
	fi
	sleep 0.5
done
if [[ "$READY" -ne 1 ]]; then
	echo "FAIL: mock server did not become ready"
	cat "$WORKDIR/mock.log" >&2 || true
	exit 1
fi
echo "mock ready at $HOST"

# --- Boot the TUI inside tmux ----------------------------------------------
# Detached new-session ignores -x/-y, so size the pane with resize-window,
# the same way scripts/mkshot.py does it.
tmux new-session -d -s "$SESSION" "$BIN --host $HOST"
tmux resize-window -t "$SESSION" -x 128 -y 32
# Wait for the TUI to reach the connected state, polling the pane the same
# way the mock-ready loop polls the mock: a fixed sleep here is the first
# thing to go flaky on a slow runner. Note "connected" is a substring of
# "disconnected", so the loop needs the lacks check too.
CONNECTED=0
for _ in $(seq 1 40); do
	if capture_contains "connected" && capture_lacks "disconnected" && capture_contains "llama-swap-tui [$HOST]"; then
		CONNECTED=1
		break
	fi
	sleep 0.5
done
if [[ "$CONNECTED" -ne 1 ]]; then
	echo "FAIL: TUI did not reach connected state after boot"
	tmux capture-pane -t "$SESSION" -p >&2 || true
	exit 1
fi

check "process stays alive after boot" tmux has-session -t "$SESSION"
check "title row shows app and host" capture_contains "llama-swap-tui [$HOST]"
check "status bar shows connected" capture_contains "connected"
check "status bar shows no disconnected state" capture_lacks "disconnected"
check "no error line shown" capture_lacks "Error:"
check "status bar shows mock API version" capture_contains "API:1.0.0-mock"
check "status bar shows binary version $EXPECT_VERSION" capture_contains "|  $EXPECT_VERSION  |"

# --- Walk the tabs -----------------------------------------------------------
# 1 Status, 2 Activity, 3 Models, 4 Hardware, 5 Logs, 6 Profiles.
for key in 1 2 3 4 5 6; do
	tmux send-keys -t "$SESSION" "$key"
	sleep 1
	check "tab $key renders title row" capture_contains "llama-swap-tui [$HOST]"
done
tmux send-keys -t "$SESSION" "3"
sleep 1
check "Models tab lists mock model" capture_contains "smoke-gemma"
tmux send-keys -t "$SESSION" "5"
sleep 1
check "Logs tab shows mock log line" capture_contains "smoke log line 1"

# --- Activity capture detail ------------------------------------------------
tmux send-keys -t "$SESSION" "2"
sleep 1
check "Activity tab lists mock model row" capture_contains "smoke-gemma"
tmux send-keys -t "$SESSION" "Enter"
sleep 2
check "detail shows input prompt" capture_contains "smoke test prompt"
check "detail shows output" capture_contains "smoke test reply"
check "detail shows section labels" capture_contains "Input prompt"
tmux send-keys -t "$SESSION" "Escape"
sleep 1
check "detail closes on Esc" capture_lacks "smoke test prompt"

# --- Clean quit ---------------------------------------------------------------
tmux send-keys -t "$SESSION" "q"
QUIT=0
for _ in $(seq 1 10); do
	if ! tmux has-session -t "$SESSION" 2>/dev/null; then
		QUIT=1
		break
	fi
	sleep 0.5
done
if [[ "$QUIT" -eq 1 ]]; then
	echo "PASS: quits cleanly on q"
else
	echo "FAIL: did not quit on q"
	FAILURES=$((FAILURES + 1))
fi

if [[ "$FAILURES" -gt 0 ]]; then
	echo "SMOKE TEST FAILED ($FAILURES check(s))"
	exit 1
fi
echo "SMOKE TEST PASSED"
