package tui

import (
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
