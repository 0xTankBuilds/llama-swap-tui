package tui

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"llama-swap-tui/api"
)

// The header connection indicator is green when the llama-swap server is
// connected and amber otherwise (matching the status bar convention).
func TestConnDotColor(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})

	tests := []struct {
		name string
		conn connState
		want string
	}{
		{"connected is green", connConnected, colorStatusReady},
		{"disconnected is amber", connDisconnected, colorStatusWarning},
		{"connecting is amber", connConnecting, colorStatusWarning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(client, "test")
			m.conn = tt.conn
			if got := m.connDotColor(); got != tt.want {
				t.Fatalf("connDotColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The connection dot must render at the top left of the header, before the
// title text.
func TestRenderTitleStartsWithConnDot(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})
	m := NewModel(client, "test")

	title := m.renderTitle()
	dotIdx := strings.Index(title, "●")
	nameIdx := strings.Index(title, "llama-swap-tui")
	if dotIdx < 0 {
		t.Fatalf("renderTitle missing connection dot: %q", title)
	}
	if nameIdx < 0 {
		t.Fatalf("renderTitle missing title text: %q", title)
	}
	if dotIdx > nameIdx {
		t.Fatalf("connection dot must come before the title text: %q", title)
	}
}

// A disconnect error must clear as soon as the stream reconnects: an SSE
// event arriving after SSEErrMsg proves the connection is live again.
// Regression test: the red "Error: ..." line at the bottom used to stay
// up forever after a reconnect, because errMsg was set on disconnect and
// never cleared anywhere.
func TestReconnectClearsStaleError(t *testing.T) {
	client := api.NewClient("http://localhost:8080", http.Header{})
	m := NewModel(client, "test")

	// Disconnect: the error is shown.
	updated, _ := m.Update(SSEErrMsg(fmt.Errorf("sse: request failed: connection refused")))
	m = updated.(*Model)
	if m.errMsg == "" {
		t.Fatal("SSEErrMsg did not set errMsg")
	}
	if m.conn != connDisconnected {
		t.Fatalf("conn = %v, want disconnected after SSEErrMsg", m.conn)
	}
	if !strings.Contains(m.View(), "Error: ") {
		t.Fatal("View() should show the disconnect error")
	}

	// Reconnect: the first event after the drop is the modelStatus
	// snapshot llama-swap sends on connect. The error must go away and
	// the connection must read connected again.
	updated, _ = m.Update(SSEMsg{Type: "modelStatus", Data: "[]"})
	m = updated.(*Model)
	if m.errMsg != "" {
		t.Fatalf("errMsg = %q after reconnect, want it cleared", m.errMsg)
	}
	if m.conn != connConnected {
		t.Fatalf("conn = %v after reconnect, want connected", m.conn)
	}
	if strings.Contains(m.View(), "Error: ") {
		t.Fatal("View() still shows the disconnect error after reconnect")
	}

	// A later disconnect must still surface its error.
	updated, _ = m.Update(SSEErrMsg(fmt.Errorf("sse: request failed: connection refused")))
	m = updated.(*Model)
	if m.errMsg == "" {
		t.Fatal("second SSEErrMsg did not set errMsg")
	}
}
