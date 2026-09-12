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
	// Run the Cmd and check its result message.
	if msg := m.fetchPerformance()(); msg != nil {
		if err, ok := msg.(error); ok {
			t.Skipf("llama-swap not reachable: %v", err)
		}
	}

	if m.sysStat == nil {
		t.Fatal("sysStat not seeded")
	}
	if len(m.gpuStats) == 0 {
		t.Fatal("gpuStats not seeded")
	}

	view := m.renderHardwareView()
	for _, want := range []string{"GPU 0", "GPU Util", "VRAM", "Live GPU Performance"} {
		if !strings.Contains(view, want) {
			t.Errorf("hardware view missing %q", want)
		}
	}

	// A second poll with the cursor set should succeed and keep stats fresh.
	if msg := m.fetchPerformance()(); msg != nil {
		if err, ok := msg.(error); ok {
			t.Fatalf("second fetchPerformance: %v", err)
		}
	}
}
