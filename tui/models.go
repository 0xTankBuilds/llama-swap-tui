package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llama-swap-tui/api"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Models view state
// ---------------------------------------------------------------------------

type modelsView struct {
	selected   int
	inflight   []api.InflightRequestEntry
	filter     string
	loading    map[string]bool // model name -> loading
	statusMsg  string
	statusTime time.Time
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderModelsView() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusError)).
			Render("Error: " + m.errMsg)
	}

	// Show input prompt when in load model mode
	if m.loadModelMode {
		return m.renderLoadModelInput()
	}

	// Split viewport: left 40% for model list, right 60% for details
	leftW := m.vp.Width / 2
	if leftW < 30 {
		leftW = 30
	}
	rightW := m.vp.Width - leftW - 2 // -2 for gap
	if rightW < 30 {
		rightW = 30
	}

	// Build left pane (model list)
	leftContent := m.renderModelList(leftW)

	// Build right pane (model details + activity)
	rightContent := m.renderModelDetail(rightW)

	// Join panes with gap
	var b strings.Builder
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")
	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	for i := 0; i < maxLines; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		rightLine := ""
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}
		b.WriteString(leftLine + strings.Repeat(" ", 2) + rightLine + "\n")
	}

	return b.String()
}

func (m *Model) renderLoadModelInput() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Load Model") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", 40)) + "\n")
	b.WriteString("\n")
	b.WriteString("  " + m.loadModelInput.View() + "\n")
	b.WriteString("\n")
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextTertiary)).
		Render("  Enter: load  Esc: cancel")
	b.WriteString(hint)

	return b.String()
}

func (m *Model) renderModelList(width int) string {
	var b strings.Builder

	// Status message
	if m.statusMsg != "" && time.Since(m.statusTime) < 5*time.Second {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorLoading)).
			Render(m.statusMsg) + "\n")
	}

	// Dynamic column widths for left pane
	nameW := width - 20 // reserve space for state, strategy, description
	if nameW < 15 {
		nameW = 15
	}
	stateW := 10
	stratW := 12
	descW := width - nameW - stateW - stratW - 10
	if descW < 15 {
		descW = 15
	}

	// Table header
	header := fmt.Sprintf("  %-*s %-*s %-*s %s", nameW, "Name", stateW, "State", stratW, "Strategy", strings.Repeat("-", descW))
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render(header) + "\n")

	// Model rows
	if len(m.models) == 0 {
		b.WriteString("  No models configured.\n")
	} else {
		for i, model := range m.models {
			line := m.renderModelRowCompact(model, i, nameW, stateW, stratW, descW)
			b.WriteString(line + "\n")
		}
	}

	// In-flight requests
	if len(m.inflight) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccentMuted)).
			Render("  In-flight:") + "\n")
		idW2 := 10
		modelW2 := 15
		methodW2 := 8
		pathW2 := width - idW2 - modelW2 - methodW2 - 8
		if pathW2 < 15 {
			pathW2 = 15
		}
		for _, req := range m.inflight {
			b.WriteString(fmt.Sprintf("  %-*s %-*s %-*s %s\n",
				idW2, truncate(req.ID, idW2-1),
				modelW2, truncate(req.Model, modelW2-1),
				methodW2, truncate(req.Method, methodW2-1),
				truncate(req.ReqPath, pathW2),
			))
		}
	}

	// Footer hint
	if len(m.models) > 0 {
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextTertiary)).
			Render("(j/k: nav  l:load  L:load-by-name  u:unload  x:cancel)")
		b.WriteString(hint)
	}

	return b.String()
}

func (m *Model) renderModelDetail(width int) string {
	var b strings.Builder

	// Safety: clamp selected before any access
	if len(m.models) > 0 && m.selected >= len(m.models) {
		m.selected = len(m.models) - 1
	}
	if len(m.models) == 0 || m.selected < 0 || m.selected >= len(m.models) {
		b.WriteString("  Select a model to see details\n")
		return b.String()
	}

	model := m.models[m.selected]

	// Model info header
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  " + modelName(model)) + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", width-4)) + "\n")

	// Model details
	stateColor := colorStatusStopped
	switch model.State {
	case api.ModelReady:
		stateColor = colorStatusReady
	case api.ModelStarting:
		stateColor = colorStatusStarting
	case api.ModelStopping:
		stateColor = colorStatusStopping
	case api.ModelStopped:
		stateColor = colorStatusStopped
	case api.ModelShutdown:
		stateColor = colorStatusError
	}

	b.WriteString(fmt.Sprintf("  State:   %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Render(string(model.State))))
	if model.Strategy != "" {
		b.WriteString(fmt.Sprintf("  Strategy: %s\n", model.Strategy))
	}
	if model.ContextLength > 0 {
		b.WriteString(fmt.Sprintf("  Context:  %d tokens\n", model.ContextLength))
	}
	if model.Description != "" {
		b.WriteString(fmt.Sprintf("  Desc:     %s\n", truncate(model.Description, width-14)))
	}
	if len(model.Aliases) > 0 {
		b.WriteString(fmt.Sprintf("  Aliases:  %s\n", truncate(strings.Join(model.Aliases, ", "), width-14)))
	}
	if len(model.Targets) > 0 {
		b.WriteString(fmt.Sprintf("  Targets:  %s\n", truncate(strings.Join(model.Targets, ", "), width-14)))
	}

	// Model activity
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Activity") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", width-4)) + "\n")

	// Stats
	if m.modelActivityStats != nil {
		b.WriteString(fmt.Sprintf("  Requests: %d  Input: %s  Output: %s  Cache: %s\n",
			m.modelActivityStats.TotalRequests,
			humanTokens(m.modelActivityStats.TotalInputTokens),
			humanTokens(m.modelActivityStats.TotalOutputTokens),
			humanTokens(m.modelActivityStats.TotalCacheTokens),
		))
	}

	// Activity rows
	if m.modelActivity.Data == nil || len(m.modelActivity.Data) == 0 {
		b.WriteString("  No activity for this model.\n")
	} else {
		for i := len(m.modelActivity.Data) - 1; i >= 0; i-- {
			if i < 0 || i >= len(m.modelActivity.Data) {
				break
			}
			entry := m.modelActivity.Data[i]
			line := m.renderModelActivityRow(entry, width)
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

func (m *Model) renderModelRow(model api.Model, idx int) string {
	stateColor := colorStatusStopped
	switch model.State {
	case api.ModelReady:
		stateColor = colorStatusReady
	case api.ModelStarting:
		stateColor = colorStatusStarting
	case api.ModelStopping:
		stateColor = colorStatusStopping
	case api.ModelStopped:
		stateColor = colorStatusStopped
	case api.ModelShutdown:
		stateColor = colorStatusError
	}

	stateStr := string(model.State)
	if m.loading[modelName(model)] {
		stateStr = "loading..."
		stateColor = colorLoading
	}

	strategy := "-"
	if model.Strategy != "" {
		strategy = model.Strategy
	}

	nameW := 28
	stateW := 10
	stratW := 14
	descW := m.vp.Width - nameW - stateW - stratW - 14
	if descW < 20 {
		descW = 20
	}

	return fmt.Sprintf("  %-*s %-*s %-*s %s",
		nameW, truncate(modelName(model), nameW-1),
		stateW, lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Render(stateStr),
		stratW, truncate(strategy, stratW-1),
		truncate(model.Description, descW),
	)
}

func (m *Model) renderModelRowCompact(model api.Model, idx int, nameW, stateW, stratW, descW int) string {
	stateColor := colorStatusStopped
	switch model.State {
	case api.ModelReady:
		stateColor = colorStatusReady
	case api.ModelStarting:
		stateColor = colorStatusStarting
	case api.ModelStopping:
		stateColor = colorStatusStopping
	case api.ModelStopped:
		stateColor = colorStatusStopped
	case api.ModelShutdown:
		stateColor = colorStatusError
	}

	stateStr := string(model.State)
	if m.loading[modelName(model)] {
		stateStr = "loading..."
		stateColor = colorLoading
	}

	strategy := "-"
	if model.Strategy != "" {
		strategy = model.Strategy
	}

	line := fmt.Sprintf("  %-*s %-*s %-*s %s",
		nameW, truncate(modelName(model), nameW-1),
		stateW, lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Render(stateStr),
		stratW, truncate(strategy, stratW-1),
		truncate(model.Description, descW),
	)
	if idx == m.selected {
		line = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color(colorHighlight)).
			Render(line)
	}
	return line
}

func (m *Model) renderModelActivityRow(entry api.ActivityLogEntry, width int) string {
	statusColor := colorStatusReady
	switch {
	case entry.RespStatusCode >= 500:
		statusColor = colorStatusError
	case entry.RespStatusCode >= 400:
		statusColor = colorStatusWarning
	case entry.RespStatusCode >= 300:
		statusColor = colorStatusStarting
	}

	// Compact layout for right pane
	timeW := 8
	statusW := 5
	inW := 7
	outW := 7
	durW := 8
	pathW := width - timeW - statusW - inW - outW - durW - 12
	if pathW < 15 {
		pathW = 15
	}

	return fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %-s",
		timeW, entry.Timestamp.Local().Format("15:04:05"),
		statusW, lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%d", entry.RespStatusCode)),
		inW, humanTokens(entry.Tokens.InputTokens),
		outW, humanTokens(entry.Tokens.OutputTokens),
		durW, formatDuration(entry.DurationMs),
		truncate(entry.ReqPath, pathW),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func modelName(m api.Model) string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m *Model) modelScrollDown() tea.Cmd {
	if len(m.models) == 0 || m.selected < 0 || m.selected >= len(m.models)-1 {
		return nil
	}
	m.selected++
	m.selectedModel = modelName(m.models[m.selected])
	m.vp.LineDown(1)
	return m.fetchModelActivity(m.selectedModel)
}

func (m *Model) modelScrollUp() tea.Cmd {
	if len(m.models) == 0 || m.selected < 1 {
		return nil
	}
	m.selected--
	m.selectedModel = modelName(m.models[m.selected])
	m.vp.LineUp(1)
	return m.fetchModelActivity(m.selectedModel)
}

func (m *Model) loadSelectedModel() tea.Cmd {
	if len(m.models) == 0 || m.selected < 0 || m.selected >= len(m.models) {
		return nil
	}
	model := m.models[m.selected]
	m.loading[modelName(model)] = true
	m.statusMsg = fmt.Sprintf("Loading model %s...", modelName(model))
	m.statusTime = time.Now()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.LoadModel(ctx, modelName(model))
		delete(m.loading, modelName(model))
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to load %s: %v", modelName(model), err)
		} else {
			m.statusMsg = fmt.Sprintf("Loading %s initiated", modelName(model))
		}
		m.statusTime = time.Now()
		return ModelsRefreshMsg{}
	}
}

func (m *Model) unloadSelectedModel() tea.Cmd {
	if len(m.models) == 0 || m.selected < 0 || m.selected >= len(m.models) {
		return nil
	}
	model := m.models[m.selected]
	m.statusMsg = fmt.Sprintf("Unloading model %s...", modelName(model))
	m.statusTime = time.Now()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.UnloadModel(ctx, modelName(model))
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to unload %s: %v", modelName(model), err)
		} else {
			m.statusMsg = fmt.Sprintf("Unloaded %s", modelName(model))
		}
		m.statusTime = time.Now()
		return ModelsRefreshMsg{}
	}
}

func (m *Model) cancelSelectedRequest() tea.Cmd {
	if len(m.inflight) == 0 || m.selected < 0 || m.selected >= len(m.inflight) {
		return nil
	}
	req := m.inflight[m.selected]
	m.statusMsg = fmt.Sprintf("Cancelling request %s...", req.ID)
	m.statusTime = time.Now()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := m.client.CancelInflightRequest(ctx, req.ID)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Failed to cancel: %v", err)
		} else {
			m.statusMsg = fmt.Sprintf("Cancelled request %s", req.ID)
		}
		m.statusTime = time.Now()
		return ModelsRefreshMsg{}
	}
}
