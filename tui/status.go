package tui

import (
	"fmt"
	"strings"
	"time"

	"llama-swap-tui/api"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Card chrome
// ---------------------------------------------------------------------------

// cardHeader renders a colored left-bar + bold title for a dashboard card.
func (m *Model) cardHeader(title, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("┃") +
		lipgloss.NewStyle().Bold(true).Render(" " + title) + "\n" +
		m.divider() + "\n"
}

// divider renders a wide section divider spanning the viewport.
func (m *Model) divider() string {
	w := m.vp.Width - 4
	if w < 20 {
		w = 20
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render(strings.Repeat("─", w))
}

// modelRate formats a tokens-per-second value; returns "-" for zero.
func modelRate(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f", v)
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func (m *Model) renderDashboard() string {
	var b strings.Builder

	b.WriteString(m.renderInflightCard())
	b.WriteString("\n")
	b.WriteString(m.renderLoadedModelsCard())
	b.WriteString("\n")
	b.WriteString(m.renderGPUCard())
	b.WriteString("\n")
	b.WriteString(m.renderActivityCard())

	return b.String()
}

// ---------------------------------------------------------------------------
// In-flight card
// ---------------------------------------------------------------------------

func (m *Model) renderInflightCard() string {
	var b strings.Builder

	b.WriteString(m.cardHeader("In-flight", colorStatusWarning))

	if len(m.inflight) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("  No requests in progress.") + "\n")
		return b.String()
	}

	// Summary line
	longest := int64(0)
	totalElapsed := int64(0)
	for _, r := range m.inflight {
		totalElapsed += r.ElapsedMs
		if r.ElapsedMs > longest {
			longest = r.ElapsedMs
		}
	}
	avgElapsed := totalElapsed / int64(len(m.inflight))

	b.WriteString("  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorStatusError)).Bold(true).Render(fmt.Sprintf("%d active", len(m.inflight))) +
		"  │  Longest: " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextPrimary)).Render(humanDuration(longest)) +
		"  │  Avg: " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary)).Render(humanDuration(avgElapsed)) + "\n")

	// Request rows (cap at 4 visible)
	maxRows := 4
	if len(m.inflight) < maxRows {
		maxRows = len(m.inflight)
	}
	for i := 0; i < maxRows; i++ {
		r := m.inflight[i]
		b.WriteString(m.renderInflightRow(r))
	}
	if len(m.inflight) > maxRows {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextTertiary)).
			Render(fmt.Sprintf("... %d more (see Models tab)", len(m.inflight)-maxRows)) + "\n")
	}

	return b.String()
}

func (m *Model) renderInflightRow(r api.InflightRequestEntry) string {
	// Elapsed bar (max 12 chars)
	barW := 12
	elapsed := r.ElapsedMs
	scaled := elapsed
	if scaled > 30000 {
		scaled = 30000
	}
	bars := int(float64(barW) * float64(scaled) / 30000.0)
	if bars < 1 {
		bars = 1
	}
	if bars > barW {
		bars = barW
	}

	// Color by duration
	color := colorStatusReady
	if elapsed > 10000 {
		color = colorStatusError
	} else if elapsed > 5000 {
		color = colorStatusWarning
	}

	elapsedStr := humanDuration(elapsed)
	const elapsedW = 5

	// Model name fills the remaining width
	modelW := m.vp.Width - (2 + 1 + barW + 1 + elapsedW + 1 + 2)
	if modelW < 10 {
		modelW = 10
	}

	return "  [" +
		lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", bars)) +
		strings.Repeat(" ", barW-bars) +
		" " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(padPlain(elapsedStr, elapsedW)) +
		"]  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).
			Render(padPlain(truncate(r.Model, modelW), modelW)) +
		"\n"
}

// ---------------------------------------------------------------------------
// Loaded Models card
// ---------------------------------------------------------------------------

func (m *Model) renderLoadedModelsCard() string {
	var b strings.Builder

	b.WriteString(m.cardHeader("Loaded Models", colorStatusReady))

	if len(m.models) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("  No models configured.") + "\n")
		return b.String()
	}

	loaded := 0
	stopping := 0
	stopped := 0
	loading := false
	for _, mod := range m.models {
		switch mod.State {
		case api.ModelReady:
			loaded++
		case api.ModelStarting:
			loading = true
		case api.ModelStopping:
			stopping++
		case api.ModelStopped:
			stopped++
		}
	}

	b.WriteString("  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorStatusReady)).Render(fmt.Sprintf("%d loaded", loaded)) +
		"  │  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorStatusStopping)).Render(fmt.Sprintf("%d stopping", stopping)) +
		"  │  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorStatusStopped)).Render(fmt.Sprintf("%d stopped", stopped)))
	if loading {
		b.WriteString("  │  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorLoading)).Render("1 loading"))
	}
	b.WriteString("\n")

	// List loaded models
	var loadedNames []string
	for _, mod := range m.models {
		if mod.State == api.ModelReady {
			loadedNames = append(loadedNames, modelName(mod))
		}
	}
	if len(loadedNames) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAccent)).
			Render(strings.Join(loadedNames, "  │  ")) + "\n")
	}

	// List stopping models
	var stoppingNames []string
	for _, mod := range m.models {
		if mod.State == api.ModelStopping {
			stoppingNames = append(stoppingNames, modelName(mod))
		}
	}
	if len(stoppingNames) > 0 {
		b.WriteString("  Stopping: " + lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusStopping)).
			Render(strings.Join(stoppingNames, "  │  ")) + "\n")
	}

	// Loading indicator
	if loading {
		b.WriteString("  Loading: " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorLoading)).Render("● loading...") + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// GPU card (simplified, color-coded bars)
// ---------------------------------------------------------------------------

func (m *Model) renderGPUCard() string {
	var b strings.Builder

	b.WriteString(m.cardHeader("GPU", colorInfo))

	if len(m.gpuStats) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("  Waiting for GPU performance data...") + "\n")
		return b.String()
	}

	// Sort GPU IDs for consistent display
	ids := make([]int, 0, len(m.gpuStats))
	for id := range m.gpuStats {
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[i] > ids[j] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}

	barW := 20

	for _, id := range ids {
		b.WriteString(m.renderGpuRow(m.gpuStats[id], barW))
	}

	return b.String()
}

func (m *Model) renderGpuRow(gs *api.GpuStat, barW int) string {
	var b strings.Builder

	memPct := 0.0
	if gs.MemTotalMB > 0 {
		memPct = float64(gs.MemUsedMB) / float64(gs.MemTotalMB) * 100
	}

	// GPU name + temp
	tempColor := colorTextPrimary
	if gs.TempC > 85 {
		tempColor = colorStatusError
	} else if gs.TempC > 70 {
		tempColor = colorStatusStarting
	}

	b.WriteString("  " +
		lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("GPU %d:", gs.ID)) +
		" " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Render(gs.Name) +
		"   " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(tempColor)).Render(fmt.Sprintf("%d°C", gs.TempC)) + "\n")

	// VRAM bar (color-coded)
	memBars := int(memPct / 100.0 * float64(barW))
	memColor := colorStatusReady
	if memPct > 90 {
		memColor = colorStatusError
	} else if memPct > 70 {
		memColor = colorStatusStarting
	}
	b.WriteString(fmt.Sprintf("    VRAM: [%s%s] %d/%d MB (%.0f%%)\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color(memColor)).Render(strings.Repeat("█", memBars)),
		strings.Repeat(" ", barW-memBars),
		gs.MemUsedMB, gs.MemTotalMB, memPct))

	// GPU util bar (color-coded)
	gutilBars := int(gs.GpuUtilPct / 100.0 * float64(barW))
	gutilColor := colorStatusReady
	if gs.GpuUtilPct > 80 {
		gutilColor = colorStatusError
	} else if gs.GpuUtilPct > 50 {
		gutilColor = colorStatusStarting
	}

	powerStr := ""
	if gs.PowerDrawW > 0 {
		if limit, ok := m.gpuPowerLimit(gs.ID); ok {
			powerStr = fmt.Sprintf("%.0fW (%.0f%%)", gs.PowerDrawW, gs.PowerDrawW/float64(limit)*100)
		} else {
			powerStr = fmt.Sprintf("%.0fW", gs.PowerDrawW)
		}
	}

	b.WriteString(fmt.Sprintf("    GPU:  [%s%s] %s  %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color(gutilColor)).Render(strings.Repeat("█", gutilBars)),
		strings.Repeat(" ", barW-gutilBars),
		fmt.Sprintf("%.0f%%", gs.GpuUtilPct),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary)).Render(powerStr)))

	return b.String()
}

// ---------------------------------------------------------------------------
// Activity card (last 10 requests)
// ---------------------------------------------------------------------------

const (
	actTimeW  = 8 // "15:43:59"
	actStatus = 5 // "200"
	actRateW  = 6 // "1234" tokens/s
	actDurW   = 6 // "1m6s"
)

// activityModelW computes the dynamic model-name column width so rows
// stretch to the full viewport width.
func (m *Model) activityModelW() int {
	w := m.vp.Width - (2 + actTimeW + 2 + actStatus + 2 + actRateW + 2 + actRateW + 2 + actDurW)
	if w < 12 {
		w = 12
	}
	return w
}

// padPlain right-pads a plain (non-styled) string to width w so that
// lipgloss styling applied afterwards does not break fmt column alignment.
func padPlain(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func (m *Model) renderActivityCard() string {
	var b strings.Builder

	b.WriteString(m.cardHeader("Recent Activity (last 10)", colorAccent))

	if m.activityPage.Data == nil || len(m.activityPage.Data) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("  No recent activity.") + "\n")
		return b.String()
	}

	mw := m.activityModelW()
	label := func(s string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary)).Render(s)
	}

	// Column headers
	b.WriteString("  " +
		label(padPlain("Time", actTimeW)) + "  " +
		label(padPlain("Model", mw)) + "  " +
		label(padPlain("Status", actStatus)) + "  " +
		label(padPlain("P/s", actRateW)) + "  " +
		label(padPlain("D/s", actRateW)) + "  " +
		label(padPlain("Dur", actDurW)) +
		"\n")

	// Separator
	b.WriteString("  " +
		strings.Repeat("─", actTimeW) + "  " +
		strings.Repeat("─", mw) + "  " +
		strings.Repeat("─", actStatus) + "  " +
		strings.Repeat("─", actRateW) + "  " +
		strings.Repeat("─", actRateW) + "  " +
		strings.Repeat("─", actDurW) +
		"\n")

	// Rows (up to 10)
	maxRows := 10
	if len(m.activityPage.Data) < maxRows {
		maxRows = len(m.activityPage.Data)
	}
	for i := 0; i < maxRows; i++ {
		entry := m.activityPage.Data[i]
		b.WriteString(m.renderDashboardActivityRow(entry, mw))
	}

	// Footer
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextTertiary)).
		Render(fmt.Sprintf("  Updated: %s ago", humanDuration(time.Since(m.lastActivityRefresh).Milliseconds()))) + "\n")

	return b.String()
}

func (m *Model) renderDashboardActivityRow(entry api.ActivityLogEntry, mw int) string {
	// Time
	t := entry.Timestamp
	timeStr := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())

	// Model (full name, truncated to column width, padded before styling)
	model := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Render(padPlain(truncate(entry.Model, mw), mw))

	// Status code (color-coded)
	statusColor := colorStatusReady
	if entry.RespStatusCode >= 500 {
		statusColor = colorStatusError
	} else if entry.RespStatusCode >= 400 {
		statusColor = colorStatusWarning
	}
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).
		Render(padPlain(fmt.Sprintf("%d", entry.RespStatusCode), actStatus))

	// Throughput rates (prompt/s, decode/s)
	ps := lipgloss.NewStyle().Foreground(lipgloss.Color(colorInfo)).
		Render(padPlain(modelRate(entry.Tokens.PromptPerSecond), actRateW))
	ds := lipgloss.NewStyle().Foreground(lipgloss.Color(colorInfo)).
		Render(padPlain(modelRate(entry.Tokens.TokensPerSecond), actRateW))

	// Duration (color-coded by slowness)
	durMs := int64(entry.DurationMs)
	durColor := colorTextPrimary
	if durMs > 10000 {
		durColor = colorStatusError
	} else if durMs > 5000 {
		durColor = colorStatusStarting
	}
	dur := lipgloss.NewStyle().Foreground(lipgloss.Color(durColor)).
		Render(padPlain(humanDuration(durMs), actDurW))

	return fmt.Sprintf("  %s  %s  %s  %s  %s  %s\n",
		timeStr, model, status, ps, ds, dur)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func humanDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	s := ms / 1000
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	return fmt.Sprintf("%dm%ds", m, s)
}
