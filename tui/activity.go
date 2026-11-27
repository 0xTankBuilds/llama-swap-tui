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
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderActivityView() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusError)).
			Render("Error: " + m.errMsg)
	}

	// Render body and apply horizontal scroll. The header is sticky above
	// the viewport but shares the same offset so columns stay aligned.
	// Only the table scrolls: the capture detail panel is appended
	// unscrolled (clipped to the viewport width) so h-scrolling never
	// clips its leading columns.
	raw := m.renderActivityBody()
	view := hScrollLines(raw, m.activityOffset(), m.vp.Width)
	if m.detail != nil {
		view += hScrollLines(m.renderActivityDetail(), 0, m.vp.Width)
	}
	return view
}

func (m *Model) renderActivityBody() string {
	var b strings.Builder

	// Currently loaded models section (at top)
	if len(m.models) > 0 {
		loaded := []string{}
		for _, model := range m.models {
			if model.State == api.ModelReady {
				// Use ID since the Name field is empty in both REST and SSE responses
				loaded = append(loaded, model.ID)
			}
		}
		if len(loaded) > 0 {
			modelsStr := strings.Join(loaded, ", ")
			b.WriteString(lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorAccentMuted)).
				Render("  Loaded Models: ") +
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorStatusReady)).
					Render(modelsStr))
			b.WriteString("\n")
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorTextTertiary)).
				Render("  No models loaded."))
			b.WriteString("\n")
		}
	} else if m.conn != connDisconnected {
		// Connected but no model data yet
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("  No models available."))
		b.WriteString("\n")
	}

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
			fmt.Fprintf(&b, "  %-13s %-15s %-18s %-13s %s\n",
				req.ID,
				req.Model,
				agent,
				req.Method,
				req.ReqPath,
			)
		}
		b.WriteString("\n")
	}

	// Stats header
	if m.activityStats != nil {
		b.WriteString(m.renderActivityStats())
		b.WriteString("\n")
	}

	// Table rows — most recent at the top (Data is returned in desc order)
	if len(m.activityPage.Data) == 0 {
		b.WriteString("  No activity yet.\n")
	} else {
		for i := 0; i < len(m.activityPage.Data); i++ {
			entry := m.activityPage.Data[i]
			line := m.renderActivityRow(entry, i)
			if i == m.activitySelected {
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
			Render(fmt.Sprintf("  Page %d/%d  (j/k: select row, enter: details, ←/→: h-scroll)", m.pageNum, m.totalPages))
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
	return hScrollLines(content, offset, width)
}

// hScrollLines clips each line to width starting at offset, with no offset
// clamping. Callers that share one offset across multiple blocks (header +
// body) must clamp it against the widest line of all blocks first (see
// activityOffset).
func hScrollLines(content string, offset, width int) string {
	if offset < 0 {
		offset = 0
	}
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
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
		colID + 1 + colTime + 1 + colModel + 1 + colStatus + 1 +
		colPrefill + 1 + colDecode + 1 + colDur + 1
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
	return m.vp.Width >= activityFixedWidth(true)+minPathWidth
}

// renderActivityHeaderRaw builds the two header lines (column row +
// separator) without styling or scrolling.
func (m *Model) renderActivityHeaderRaw() string {
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s",
		colID, "ID", colTime, "Time", colModel, "Model", colStatus, "Status")
	if m.useTokenColumns() {
		header += fmt.Sprintf(" %-*s %-*s %-*s", colCached, "Cached", colIn, "In", colOut, "Out")
	}
	header += fmt.Sprintf(" %-*s %-*s %-*s %s",
		colPrefill, "P/s", colDecode, "D/s", colDur, "Duration", "Path")
	return header + "\n" + strings.Repeat("-", ansi.StringWidth(header)) + "\n"
}

func (m *Model) renderActivityHeader() string {
	raw := strings.SplitN(m.renderActivityHeaderRaw(), "\n", 3)
	header := raw[0]
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render(raw[1])
	h := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render(header) + "\n" + sep + "\n"
	// H-scroll with the body's shared offset so columns stay aligned.
	return hScrollLines(h, m.activityOffset(), m.vp.Width)
}

// activityOffset is the horizontal scroll offset shared by the sticky
// header and the body rows, clamped against the widest line of either so
// the two can never scroll out of step.
func (m *Model) activityOffset() int {
	maxW := 0
	for _, l := range strings.Split(m.renderActivityHeaderRaw(), "\n") {
		if w := ansi.StringWidth(l); w > maxW {
			maxW = w
		}
	}
	for _, l := range strings.Split(m.renderActivityBody(), "\n") {
		if w := ansi.StringWidth(l); w > maxW {
			maxW = w
		}
	}
	off := max(0, m.hScrollOffset)
	if maxOff := max(0, maxW-m.vp.Width); off > maxOff {
		off = maxOff
	}
	return off
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

	statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%s%d", statusSymbol(entry.RespStatusCode), entry.RespStatusCode))
	cachedStr := humanTokens(entry.Tokens.CachedTokens)
	inStr := humanTokens(entry.Tokens.InputTokens)
	outStr := humanTokens(entry.Tokens.OutputTokens)
	pStr := formatRate(entry.Tokens.PromptPerSecond)
	dStr := formatRate(entry.Tokens.TokensPerSecond)
	durStr := formatDuration(entry.DurationMs)

	// Render full-width (no truncation) - scrolling handles visibility
	line := fmt.Sprintf("  %-*d %-*s %-*s %-*s",
		colID, entry.ID,
		colTime, entry.Timestamp.Local().Format("15:04:05"),
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

func (m *Model) activityNextPage() tea.Cmd {
	if m.pageNum < m.totalPages {
		m.pageNum++
		m.activitySelected = 0
		m.closeActivityDetail()
		m.hScrollOffset = 0
		return m.fetchActivity()
	}
	return nil
}

func (m *Model) activityPrevPage() tea.Cmd {
	if m.pageNum > 1 {
		m.pageNum--
		m.activitySelected = 0
		m.closeActivityDetail()
		m.hScrollOffset = 0
		return m.fetchActivity()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Capture detail (issue #26)
// ---------------------------------------------------------------------------

// activityDetail is the expanded capture view for one activity row. It is
// keyed by entry ID, not row index, so a page refresh that moves the row
// keeps the detail on the same request.
type activityDetail struct {
	entryID   int
	captureID string
	model     string
	time      string // HH:MM:SS, as shown in the table
	// generation identifies this open of the row: closing and reopening
	// the same row starts a new fetch with the same entry and capture
	// IDs, so results are matched to the open that requested them.
	generation int
	loading    bool
	noCapture  bool
	pruned     bool
	err        string
	prompts    captureDetail
}

// closeActivityDetail closes the expanded capture panel, if open.
func (m *Model) closeActivityDetail() {
	m.detail = nil
}

// toggleActivityDetail opens the expanded capture panel for the
// selected activity row, or closes it if it is already open for that
// row. Rows without a capture show a note instead of fetching.
func (m *Model) toggleActivityDetail() tea.Cmd {
	if len(m.activityPage.Data) == 0 || m.activitySelected < 0 || m.activitySelected >= len(m.activityPage.Data) {
		return nil
	}
	entry := m.activityPage.Data[m.activitySelected]
	if m.detail != nil && m.detail.entryID == entry.ID {
		m.detail = nil
		return nil
	}
	d := &activityDetail{
		entryID:   entry.ID,
		captureID: entry.CaptureID,
		model:     entry.Model,
		time:      entry.Timestamp.Local().Format("15:04:05"),
	}
	m.detail = d
	if !entry.HasCapture || entry.CaptureID == "" {
		d.noCapture = true
		return nil
	}
	d.loading = true
	m.captureGen++
	d.generation = m.captureGen
	return m.fetchCapture(entry, d.generation)
}

// activitySelectDown moves the activity row selection down one row,
// closing the detail: the detail belongs to the previously selected row.
func (m *Model) activitySelectDown() {
	m.closeActivityDetail()
	m.clampActivitySelected()
	if m.activitySelected < len(m.activityPage.Data)-1 {
		m.activitySelected++
	}
	m.ensureActivitySelectedVisible()
}

// activitySelectUp moves the activity row selection up one row,
// closing the detail: the detail belongs to the previously selected row.
func (m *Model) activitySelectUp() {
	m.closeActivityDetail()
	m.clampActivitySelected()
	if m.activitySelected > 0 {
		m.activitySelected--
	}
	m.ensureActivitySelectedVisible()
}

// activityTableStartLine is the viewport line index of the first table
// row: the sections rendered above the table in renderActivityBody.
func (m *Model) activityTableStartLine() int {
	lines := 0
	if len(m.models) > 0 || m.conn != connDisconnected {
		lines++
	}
	if len(m.inflight) > 0 {
		lines += len(m.inflight) + 2
	}
	if m.activityStats != nil {
		lines++
	}
	return lines
}

// ensureActivitySelectedVisible scrolls the viewport just enough to
// keep the selected row on screen after a selection move.
func (m *Model) ensureActivitySelectedVisible() {
	if len(m.activityPage.Data) == 0 || m.vp.Height <= 0 {
		return
	}
	rowLine := m.activityTableStartLine() + m.activitySelected
	if rowLine < m.vp.YOffset {
		m.vp.SetYOffset(rowLine)
	} else if rowLine >= m.vp.YOffset+m.vp.Height {
		m.vp.SetYOffset(rowLine - m.vp.Height + 1)
	}
}

// clampActivitySelected keeps the activity row selection inside the
// current page. The Models tab shares m.selected for its own list, so
// the Activity tab keeps a separate index.
func (m *Model) clampActivitySelected() {
	if len(m.activityPage.Data) == 0 {
		m.activitySelected = 0
	} else if m.activitySelected >= len(m.activityPage.Data) {
		m.activitySelected = len(m.activityPage.Data) - 1
	} else if m.activitySelected < 0 {
		m.activitySelected = 0
	}
}

// syncDetailWithPage keeps the open detail on the same request after the
// activity page is replaced: the row may have moved, or dropped off the
// page entirely (then the detail closes).
func (m *Model) syncDetailWithPage() {
	if m.detail == nil {
		return
	}
	for i, entry := range m.activityPage.Data {
		if entry.ID == m.detail.entryID {
			m.activitySelected = i
			return
		}
	}
	m.detail = nil
}

// renderActivityDetail renders the expanded capture panel for the
// selected activity row: input prompt and output, wrapped to the
// viewport width so long prompts stay readable without h-scrolling.
func (m *Model) renderActivityDetail() string {
	d := m.detail
	var b strings.Builder
	header := fmt.Sprintf("  ── Request #%d · %s · %s ──", d.entryID, d.model, d.time)
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccentMuted)).
		Render(header))
	b.WriteString("\n")
	switch {
	case d.noCapture:
		b.WriteString("  No capture available for this request.\n")
	case d.loading:
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorLoading)).
			Render("  Loading capture…"))
		b.WriteString("\n")
	case d.pruned:
		b.WriteString("  This capture is no longer available (it was pruned).\n")
	case d.err != "":
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusError)).
			Render("  Could not load capture: " + d.err))
		b.WriteString("\n")
	default:
		writeDetailSection(&b, "Input prompt", d.prompts.input, m.detailWidth())
		if d.prompts.reasoning != "" {
			writeDetailSection(&b, "Reasoning", d.prompts.reasoning, m.detailWidth())
		}
		writeDetailSection(&b, "Output", d.prompts.output, m.detailWidth())
	}
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextTertiary)).
		Render("  (esc to close)"))
	b.WriteString("\n")
	return b.String()
}

// stripControlChars removes C0 and C1 control bytes (and DEL) from s so
// captured text can never smuggle terminal escape sequences into the
// rendered view: a stored prompt could otherwise carry an OSC 52
// sequence that rewrites the operator's clipboard. Newline and tab are
// kept, wordWrap splits paragraphs on newlines and treats tabs as
// whitespace.
func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// writeDetailSection renders one labelled, wrapped block of the detail
// panel. Empty text reads "(empty)" rather than vanishing.
func writeDetailSection(b *strings.Builder, label, text string, width int) {
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccentMuted)).
		Render("  " + label + ":"))
	b.WriteString("\n")
	if strings.TrimSpace(text) == "" {
		b.WriteString("    (empty)\n\n")
		return
	}
	for _, line := range wordWrap(stripControlChars(text), width) {
		b.WriteString("    " + line + "\n")
	}
	b.WriteString("\n")
}

// detailWidth is the text width for wrapped detail lines: the viewport
// width less the 4-space indent and a small margin.
func (m *Model) detailWidth() int {
	w := m.vp.Width - 6
	if w < 10 {
		w = 10
	}
	return w
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
