package tui

import (
	"fmt"
	"strings"

	"llama-swap-tui/api"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
			Foreground(lipgloss.Color(colorStatusError)).
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
			Foreground(lipgloss.Color(colorAccentMuted)).
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
					Background(lipgloss.Color(colorHighlight)).
					Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	// Pagination footer
	if m.totalPages > 1 {
		pager := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextSecondary)).
			Render(fmt.Sprintf("  Page %d/%d  (j/k: scroll, /: filter, ←/→: h-scroll)", m.pageNum, m.totalPages))
		b.WriteString(pager)
	}

	return b.String()
}

// applyHScroll clips content to the viewport with horizontal scrolling.
// Slicing is grapheme/ANSI-aware (like the bubbles viewport) so colored rows
// and UTF-8 text stay intact, and the offset is clamped to the longest line
// so over-scrolling can never blank the view.
func applyHScroll(content string, offset, width int) string {
	lines := strings.Split(content, "\n")

	if offset < 0 {
		offset = 0
	}
	maxLen := 0
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > maxLen {
			maxLen = w
		}
	}
	if maxOffset := max(0, maxLen-width); offset > maxOffset {
		offset = maxOffset
	}

	var b strings.Builder
	for _, line := range lines {
		if width > 0 {
			b.WriteString(ansi.Cut(line, offset, offset+width))
		} else {
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
		Foreground(lipgloss.Color(colorTextTertiary)).
		Render(fmt.Sprintf(
			"  Requests: %-8d  Input: %-10d  Output: %-10d  Cache: %-10d",
			s.TotalRequests,
			s.TotalInputTokens,
			s.TotalOutputTokens,
			s.TotalCacheTokens,
		))
}

// Table column widths for the activity view.
const (
	colID      = 7
	colTime    = 9
	colModel   = 28
	colStatus  = 6
	colCached  = 9
	colIn      = 9
	colOut     = 9
	colPrefill = 8 // P/s — prompt tokens/sec (prefill)
	colDecode  = 8 // D/s — generated tokens/sec (decode)
	colDur     = 10
)

// activityFixedWidth returns the total cell width of the always-shown
// columns plus separators, optionally including the token-count columns.
func activityFixedWidth(withTokens bool) int {
	w := 2 + // indent
		colID+1 + colTime+1 + colModel+1 + colStatus+1 +
		colPrefill+1 + colDecode+1 + colDur+1
	if withTokens {
		w += colCached + colIn + colOut + 3 // + 3 separators
	}
	return w
}

// useTokenColumns reports whether the viewport is wide enough to also show
// the Cached/In/Out token columns. Prefill/decode (P/s/D/s) are always shown;
// token counts are the first columns dropped on narrow terminals.
func (m *Model) useTokenColumns() bool {
	const minPathWidth = 12 // keep the path column readable
	return m.vp.Width >= activityFixedWidth(true) + minPathWidth
}

func (m *Model) renderActivityHeader() string {
	// Render full-width header (no truncation) - scrolling handles visibility
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s",
		colID, "ID", colTime, "Time", colModel, "Model", colStatus, "Status")
	if m.useTokenColumns() {
		header += fmt.Sprintf(" %-*s %-*s %-*s", colCached, "Cached", colIn, "In", colOut, "Out")
	}
	header += fmt.Sprintf(" %-*s %-*s %-*s %s",
		colPrefill, "P/s", colDecode, "D/s", colDur, "Duration", "Path")
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render(strings.Repeat("-", ansi.StringWidth(header)))
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render(header) + "\n" + sep + "\n"
}

func (m *Model) renderActivityRow(entry api.ActivityLogEntry, idx int) string {
	statusColor := colorStatusReady
	switch {
	case entry.RespStatusCode >= 500:
		statusColor = colorStatusError
	case entry.RespStatusCode >= 400:
		statusColor = colorStatusWarning
	case entry.RespStatusCode >= 300:
		statusColor = colorStatusStarting
	}

	statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%d", entry.RespStatusCode))
	cachedStr := humanTokens(entry.Tokens.CachedTokens)
	inStr := humanTokens(entry.Tokens.InputTokens)
	outStr := humanTokens(entry.Tokens.OutputTokens)
	pStr := formatRate(entry.Tokens.PromptPerSecond)
	dStr := formatRate(entry.Tokens.TokensPerSecond)
	durStr := formatDuration(entry.DurationMs)

	// Render full-width (no truncation) - scrolling handles visibility
	line := fmt.Sprintf("  %-*d %-*s %-*s %-*s",
		colID, entry.ID,
		colTime, entry.Timestamp.Format("15:04:05"),
		colModel, entry.Model,
		colStatus, statusStr)
	if m.useTokenColumns() {
		line += fmt.Sprintf(" %-*s %-*s %-*s", colCached, cachedStr, colIn, inStr, colOut, outStr)
	}
	line += fmt.Sprintf(" %-*s %-*s %-*s %s",
		colPrefill, pStr,
		colDecode, dStr,
		colDur, durStr,
		entry.ReqPath)
	return line
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m *Model) activityScrollDown() {
	if len(m.activityPage.Data) == 0 || m.selected < 0 || m.selected >= len(m.activityPage.Data)-1 {
		return
	}
	m.selected++
	m.vp.LineDown(1)
}

func (m *Model) activityScrollUp() {
	if len(m.activityPage.Data) == 0 || m.selected < 1 {
		return
	}
	m.selected--
	m.vp.LineUp(1)
}

func (m *Model) activityNextPage() tea.Cmd {
	if m.pageNum < m.totalPages {
		m.pageNum++
		m.selected = 0
		m.hScrollOffset = 0
		return m.fetchActivity()
	}
	return nil
}

func (m *Model) activityPrevPage() tea.Cmd {
	if m.pageNum > 1 {
		m.pageNum--
		m.selected = 0
		m.hScrollOffset = 0
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
