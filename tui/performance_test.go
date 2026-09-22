package tui

import (
	"net/http"
	"strings"
	"testing"

	"llama-swap-tui/api"
)

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
