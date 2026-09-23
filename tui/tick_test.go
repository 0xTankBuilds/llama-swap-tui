package tui

import (
	"net/http"
	"testing"
	"time"

	"llama-swap-tui/api"
)

// autoRefreshTick must deliver a time.Time message into the update loop.
//
// tea.Tick(d, fn) sends the RESULT of fn(t) (not the timestamp itself) as the
// message. The auto-refresh chain depends on `case time.Time` in Update to run
// the refresh and reschedule the next tick. A callback that returns nil (or any
// non-time type) silently drops the tick into the default case and kills the
// whole auto-refresh chain after the first tick — which was the original bug.
func TestAutoRefreshTickDeliversTimeMsg(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})
	m := NewModel(client, "test")

	cmd := m.autoRefreshTick()
	if cmd == nil {
		t.Fatal("autoRefreshTick returned nil Cmd")
	}

	// Execute the Cmd. tea.Tick blocks until the timer fires, then returns the
	// message that will be sent into the update loop.
	msg := cmd()
	if _, ok := msg.(time.Time); !ok {
		t.Fatalf("tick delivered %T, want time.Time (chain would die in default case)", msg)
	}
}

// When Update receives a time.Time it must schedule the next tick (and the
// per-tab refreshes). If the message type were not matched, Update would fall
// through to `default` and return a nil Cmd, ending the auto-refresh chain.
func TestTimeMsgReschedulesTick(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})
	m := NewModel(client, "test")

	// No network is touched here: the returned Cmd is constructed, not run.
	_, cmd := m.Update(time.Now())
	if cmd == nil {
		t.Fatal("Update(time.Time) returned nil Cmd; tick chain is not self-perpetuating")
	}
}
