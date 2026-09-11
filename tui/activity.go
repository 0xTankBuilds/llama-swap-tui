package tui

import (
	"fmt"
	"strings"

	"llama-swap-tui/api"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Activity view state
// ---------------------------------------------------------------------------

type sortColumn int

const (
	sortID sortColumn = iota
	sortTime
	sortModel
	sortStatus
	sortPath
)

type activityView struct {
	page      api.ActivityPage
	stats     *api.ActivityStats
	sortCol   sortColumn
	sortOrder string // "asc" or "desc"
	filter    string // model filter
	pageNum   int
	totalPages int
	selected  int // selected row index
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderActivityView() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f44")).
			Render("Error: " + m.errMsg)
	}

	// Render content and apply horizontal scroll
	raw := m.renderActivityContent()
	return applyHScroll(raw, m.hScrollOffset, m.vp.Width)
}

func (m *Model) renderActivityContent() string {
	var b strings.Builder

	// In-flight requests section (at top)
	if len(m.inflight) > 0 {
		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F80")).
			Render("  In-flight Requests:") + "\n")
		for _, req := range m.inflight {
			agent := "-"
			if req.ReqHeaders != nil {
				if ua, ok := req.ReqHeaders["User-Agent"]; ok {
					agent = ua
				} else if ua, ok := req.ReqHeaders["user-agent"]; ok {
					agent = ua
				}
			}
			// Render full-width (no truncation) - scrolling handles visibility
			b.WriteString(fmt.Sprintf("  %-13s %-15s %-18s %-13s %s\n",
				req.ID,
				req.Model,
				agent,
				req.Method,
				req.ReqPath,
			))
		}
		b.WriteString("\n")
	}

	// Stats header
	if m.activityStats != nil {
		b.WriteString(m.renderActivityStats())
		b.WriteString("\n")
	}

	// Table header
	b.WriteString(m.renderActivityHeader())

	// Table rows — most recent at the top (Data is returned in desc order)
	if len(m.activityPage.Data) == 0 {
		b.WriteString("  No activity yet.\n")
	} else {
		for i := 0; i < len(m.activityPage.Data); i++ {
			entry := m.activityPage.Data[i]
			line := m.renderActivityRow(entry, i)
			if i == m.selected {
				line = lipgloss.NewStyle().
					Bold(true).
					Background(lipgloss.Color("#444")).
					Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	// Pagination footer
	if m.totalPages > 1 {
		pager := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Render(fmt.Sprintf("  Page %d/%d  (j/k: scroll, /: filter, ←/→: h-scroll)", m.pageNum, m.totalPages))
		b.WriteString(pager)
	}

	return b.String()
}

// applyHScroll applies horizontal scrolling to multi-line text
func applyHScroll(content string, offset, width int) string {
	if offset <= 0 && width > 0 {
		// No scroll, but still constrain to viewport width
		var b strings.Builder
		for _, line := range strings.Split(content, "\n") {
			if len(line) > width {
				b.WriteString(line[:width])
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if len(line) > offset {
			line = line[offset:]
			if width > 0 && len(line) > width {
				line = line[:width]
			}
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) renderActivityStats() string {
	s := m.activityStats
	if s == nil {
		return "  Waiting for activity data..."
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#aaa")).
		Render(fmt.Sprintf(
			"  Requests: %-8d  Input: %-10d  Output: %-10d  Cache: %-10d",
			s.TotalRequests,
			s.TotalInputTokens,
			s.TotalOutputTokens,
			s.TotalCacheTokens,
		))
}

func (m *Model) renderActivityHeader() string {
	// Render full-width header (no truncation) - scrolling handles visibility
	header := "  ID       Time       Model                  Status Cached    In        Out       P/s      D/s      Duration Path"
	sepW := len(header)
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
		Render(strings.Repeat("-", sepW-2))
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#0BB")).
		Render(header) + "\n" + sep + "\n"
}

func (m *Model) renderActivityRow(entry api.ActivityLogEntry, idx int) string {
	statusColor := "#0F0"
	switch {
	case entry.RespStatusCode >= 500:
		statusColor = "#F44"
	case entry.RespStatusCode >= 400:
		statusColor = "#F80"
	case entry.RespStatusCode >= 300:
		statusColor = "#FF0"
	}

	statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%d", entry.RespStatusCode))
	cachedStr := humanTokens(entry.Tokens.CachedTokens)
	inStr := humanTokens(entry.Tokens.InputTokens)
	outStr := humanTokens(entry.Tokens.OutputTokens)
	pStr := formatRate(entry.Tokens.PromptPerSecond)
	dStr := formatRate(entry.Tokens.TokensPerSecond)
	durStr := formatDuration(entry.DurationMs)

	// Render full-width (no truncation) - scrolling handles visibility
	return fmt.Sprintf("  %-7d %-9s %-28s %-6s %-9s %-9s %-9s %-8s %-8s %-10s %s",
		entry.ID,
		entry.Timestamp.Format("15:04:05"),
		entry.Model,
		statusStr,
		cachedStr,
		inStr,
		outStr,
		pStr,
		dStr,
		durStr,
		entry.ReqPath,
	)
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m *Model) activityScrollDown() {
	if m.selected < len(m.activityPage.Data)-1 {
		m.selected++
		m.vp.LineDown(1)
	}
}

func (m *Model) activityScrollUp() {
	if m.selected > 0 {
		m.selected--
		m.vp.LineUp(1)
	}
}

func (m *Model) activityNextPage() tea.Cmd {
	if m.pageNum < m.totalPages {
		m.pageNum++
		m.selected = 0
		return m.fetchActivity()
	}
	return nil
}

func (m *Model) activityPrevPage() tea.Cmd {
	if m.pageNum > 1 {
		m.pageNum--
		m.selected = 0
		return m.fetchActivity()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func humanTokens(n int) string {
	const (
		_ = 1 << (iota * 10)
		K
		M
	)
	switch {
	case n >= M:
		return fmt.Sprintf("%.1fM", float64(n)/M)
	case n >= K:
		return fmt.Sprintf("%.1fK", float64(n)/K)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatRate(rate float64) string {
	if rate <= 0 {
		return "-"
	}
	if rate >= 1000 {
		return fmt.Sprintf("%.1fk", rate/1000)
	}
	return fmt.Sprintf("%.1f", rate)
}

func formatDuration(ms int) string {
	if ms <= 0 {
		return "-"
	}
	if ms >= 60000 {
		return fmt.Sprintf("%dm%ds", ms/60000, (ms%60000)/1000)
	}
	if ms >= 1000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%dms", ms)
}
