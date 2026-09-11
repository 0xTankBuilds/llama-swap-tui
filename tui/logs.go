package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Logs view state
// ---------------------------------------------------------------------------

type logSourceFilter int

const (
	logSourceAll logSourceFilter = iota
	logSourceProxy
	logSourceUpstream
)

func (f logSourceFilter) String() string {
	switch f {
	case logSourceAll:
		return "All"
	case logSourceProxy:
		return "Proxy"
	case logSourceUpstream:
		return "Upstream"
	default:
		return "All"
	}
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderLogsContent() string {
	var b strings.Builder

	// Source filter indicator
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Logs") + "  " +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextSecondary)).
			Render(fmt.Sprintf("(filter: %s)", m.logFilter.String())))
	b.WriteString("\n")

	if len(m.logLines) == 0 {
		b.WriteString("  No logs yet.\n")
		b.WriteString("  Waiting for log data...\n")
		return b.String()
	}

	// Show last N lines
	maxLines := m.vp.Height
	if maxLines < 5 {
		maxLines = 5
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}

	for i := start; i < len(m.logLines); i++ {
		line := m.logLines[i]
		// Color-code by source prefix [proxy] or [upstream]
		sourceColor := colorTextSecondary
		if strings.HasPrefix(line, "[proxy]") {
			sourceColor = colorInfo
			line = "[proxy] " + strings.TrimPrefix(line, "[proxy] ")
		} else if strings.HasPrefix(line, "[upstream]") {
			sourceColor = colorStatusReady
			line = "[upstream] " + strings.TrimPrefix(line, "[upstream] ")
		}

		// Apply source filter
		switch m.logFilter {
		case logSourceProxy:
			if !strings.HasPrefix(line, "[proxy]") {
				continue
			}
		case logSourceUpstream:
			if !strings.HasPrefix(line, "[upstream]") {
				continue
			}
		}

		lineW := m.vp.Width - 4 // 4 = padding
		if lineW < 40 {
			lineW = 40
		}
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(sourceColor)).
			Render(truncate(line, lineW)) + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m *Model) cycleLogFilter() {
	switch m.logFilter {
	case logSourceAll:
		m.logFilter = logSourceProxy
	case logSourceProxy:
		m.logFilter = logSourceUpstream
	case logSourceUpstream:
		m.logFilter = logSourceAll
	}
}

func (m *Model) logScrollDown() {
	m.vp.LineDown(1)
}

func (m *Model) logScrollUp() {
	m.vp.LineUp(1)
}

func (m *Model) logScrollToBottom() {
	m.vp.GotoBottom()
}
