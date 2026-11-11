package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llama-swap-tui/api"
)

// TestPerformanceCursorIsEscaped guards the bug that made every poll after the
// first fail with "HTTP 400": the cursor was concatenated into the query string
// unescaped, so the "+" of a "+01:00" UTC offset decoded to a space and the
// server could no longer parse it.
func TestPerformanceCursorIsEscaped(t *testing.T) {
	stamp := "2026-11-11T07:24:03.409735855+01:00"
	var seen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/performance", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("after"))
		if a := r.URL.Query().Get("after"); a != "" {
			if _, err := time.Parse(time.RFC3339, a); err != nil {
				http.Error(w, "invalid 'after' timestamp, use RFC3339 format", http.StatusBadRequest)
				return
			}
		}
		fmt.Fprintf(w, `{"enabled":true,"sys_stats":[{"timestamp":%q,"mem_total_mb":100}],`+
			`"gpu_stats":[{"timestamp":%q,"id":0,"name":"T"}]}`, stamp, stamp)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m := NewModel(api.NewClient(srv.URL, http.Header{}), "test")

	for poll := 1; poll <= 3; poll++ {
		msg := m.fetchPerformance()()
		if err, ok := msg.(error); ok {
			t.Fatalf("poll %d failed: %v", poll, err)
		}
		next, _ := m.Update(msg)
		m = next.(*Model)
	}

	if len(seen) != 3 {
		t.Fatalf("server saw %d requests, want 3", len(seen))
	}
	if seen[0] != "" {
		t.Errorf("first poll sent after=%q, want empty", seen[0])
	}
	for _, a := range seen[1:] {
		if _, err := time.Parse(time.RFC3339, a); err != nil {
			t.Errorf("cursor %q rejected by server: %v", a, err)
		}
	}
	if m.perfCursor == nil {
		t.Fatal("perfCursor not set")
	}
}

// TestPerformanceCursorTracksInstantAcrossOffsets checks that the cursor is
// compared as an instant, never as raw text. A "+01:00" stamp and a "Z" stamp
// for the same moment sort differently lexicographically; a string compare would
// stall or rewind the cursor and either re-fetch everything or drop entries.
func TestPerformanceCursorTracksInstantAcrossOffsets(t *testing.T) {
	// 08:00+01:00 is the same instant as 07:00Z; lexicographically "07:05Z"
	// sorts before "08:00+01:00", so a string compare would not advance.
	m := NewModel(api.NewClient("http://unused", http.Header{}), "test")
	m.applyPerformance(api.PerformanceResponse{
		SysStats: []api.SysStat{{Timestamp: "2026-11-11T08:00:00+01:00"}},
	})
	if m.perfCursor == nil {
		t.Fatal("perfCursor not set")
	}
	first := *m.perfCursor

	m.applyPerformance(api.PerformanceResponse{
		SysStats: []api.SysStat{{Timestamp: "2026-11-11T07:05:00Z"}},
	})
	if !m.perfCursor.After(first) {
		t.Fatalf("cursor %v did not advance past %v for a later instant", *m.perfCursor, first)
	}

	// A stale or unparseable entry must never rewind the cursor, otherwise
	// every later poll re-fetches the entire ring buffer.
	current := *m.perfCursor
	m.applyPerformance(api.PerformanceResponse{
		SysStats: []api.SysStat{{Timestamp: "not-a-time"}},
	})
	if got := *m.perfCursor; !got.Equal(current) {
		t.Fatalf("cursor = %v after unparseable stamp, want %v", got, current)
	}
}

// TestPerformanceEmptyResponseHoldsCursor verifies a poll that returns nothing
// new (server still buffering, or the window already consumed) keeps the cursor
// where it was instead of resetting it and refetching the whole buffer.
func TestPerformanceEmptyResponseHoldsCursor(t *testing.T) {
	m := NewModel(api.NewClient("http://unused", http.Header{}), "test")
	m.applyPerformance(api.PerformanceResponse{
		SysStats: []api.SysStat{{Timestamp: "2026-11-11T08:00:00+01:00"}},
	})
	before := *m.perfCursor
	m.applyPerformance(api.PerformanceResponse{})
	if got := *m.perfCursor; !got.Equal(before) {
		t.Fatalf("cursor = %v after empty response, want %v", got, before)
	}
}

// TestFetchPerformanceLive polls the local llama-swap instance and verifies
// that the live GPU/system stats get seeded and render.
func TestFetchPerformanceLive(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})
	m := NewModel(client, "test")
	// Run the Cmd; fetched data arrives as a message.
	msg := m.fetchPerformance()()
	if msg != nil {
		if err, ok := msg.(error); ok {
			t.Skipf("llama-swap not reachable: %v", err)
		}
	}
	next, _ := m.Update(msg)
	m = next.(*Model)

	if m.sysStat == nil {
		t.Fatal("sysStat not seeded")
	}
	if len(m.gpuStats) == 0 {
		t.Fatal("gpuStats not seeded")
	}

	m.winW, m.winH = 100, 30
	m.recalcVP(1)
	view := m.renderHardwareBody()
	for _, want := range []string{"GPU 0", "GPU Util", "VRAM", "Live GPU Performance"} {
		if !strings.Contains(view, want) {
			t.Errorf("hardware view missing %q", want)
		}
	}

	// A second poll with the cursor set should succeed and keep stats fresh.
	msg = m.fetchPerformance()()
	if msg != nil {
		if err, ok := msg.(error); ok {
			t.Fatalf("second fetchPerformance: %v", err)
		}
	}
	next, _ = m.Update(msg)
	m = next.(*Model)
}
