// Package tui provides the Bubbletea-based terminal UI.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llama-swap-tui/api"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Tab types
// ---------------------------------------------------------------------------

type tab int

const (
	tabActivity tab = iota
	tabModels
	tabHardware
	tabLogs
	tabProfiles
	tabCount
)

func (t tab) String() string {
	switch t {
	case tabActivity:
		return "Activity"
	case tabModels:
		return "Models"
	case tabHardware:
		return "Hardware"
	case tabLogs:
		return "Logs"
	case tabProfiles:
		return "Profiles"
	default:
		return "Unknown"
	}
}

// ---------------------------------------------------------------------------
// Connection state
// ---------------------------------------------------------------------------

type connState int

const (
	connDisconnected connState = iota
	connConnecting
	connConnected
)

func (s connState) String() string {
	switch s {
	case connDisconnected:
		return "● disconnected"
	case connConnecting:
		return "● connecting"
	case connConnected:
		return "● connected"
	default:
		return "● ?"
	}
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// SSEMsg is a message from the SSE event stream.
type SSEMsg struct {
	Type string
	Data string
}

// SSEErrMsg is a connection error from the SSE stream.
type SSEErrMsg error

// ShutdownMsg triggers graceful shutdown.
type ShutdownMsg struct{}

// ActivityRefreshMsg triggers an activity refresh.
type ActivityRefreshMsg struct{}

// HardwareRefreshMsg triggers a hardware fetch.
type HardwareRefreshMsg struct{}

// ModelsRefreshMsg triggers a models fetch.
type ModelsRefreshMsg struct{}

// ProfilesRefreshMsg triggers a profiles fetch.
type ProfilesRefreshMsg struct{}

// LogLineMsg is a line from the log stream.
type LogLineMsg string

// ---------------------------------------------------------------------------
// Key bindings
// ---------------------------------------------------------------------------

type keyMap struct {
	Quit          key.Binding
	Refresh       key.Binding
	Help          key.Binding
	NextTab       key.Binding
	PrevTab       key.Binding
	LoadModel     key.Binding
	LoadModelByName key.Binding
	UnloadModel   key.Binding
	CancelReq     key.Binding
	SwitchProfile key.Binding
	ScrollDown    key.Binding
	ScrollUp      key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Refresh, k.Help}
}

func (k keyMap) FullHelp() []key.Binding {
	return []key.Binding{
		k.NextTab, k.PrevTab,
		k.LoadModel, k.LoadModelByName, k.UnloadModel, k.CancelReq, k.SwitchProfile,
		k.ScrollDown, k.ScrollUp,
		k.Quit, k.Refresh, k.Help,
	}
}

var keys = keyMap{
	Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Refresh:       key.NewBinding(key.WithKeys("r")),
	Help:          key.NewBinding(key.WithKeys("?")),
	NextTab:       key.NewBinding(key.WithKeys("tab", "shift+tab", "n", "1", "2", "3", "4", "5")),
	PrevTab:       key.NewBinding(key.WithKeys("h", "p")),
	LoadModel:     key.NewBinding(key.WithKeys("l")),
	LoadModelByName: key.NewBinding(key.WithKeys("L")),
	UnloadModel:   key.NewBinding(key.WithKeys("u")),
	CancelReq:     key.NewBinding(key.WithKeys("x")),
	SwitchProfile: key.NewBinding(key.WithKeys("enter")),
	ScrollDown:    key.NewBinding(key.WithKeys("j", "down", "pgdown")),
	ScrollUp:      key.NewBinding(key.WithKeys("k", "up", "pgup")),
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the main Bubbletea model.
type Model struct {
	client        *api.Client
	sse           *api.SSEClient
	logStream     <-chan string
	program       *tea.Program
	tab           tab
	conn          connState
	version       api.VersionInfo
	activeProfile string
	hardware      api.HardwareSnapshot
	models        []api.Model
	activityPage  api.ActivityPage
	activityStats *api.ActivityStats
	selected      int
	pageNum       int
	totalPages    int

	// Models
	inflight        []api.InflightRequestEntry
	loading         map[string]bool
	statusMsg       string
	statusTime      time.Time
	selectedModel   string // name of selected model
	modelActivity   api.ActivityPage
	modelActivityStats *api.ActivityStats

	// Live performance (SSE perfsys / perfgpu + /api/performance polling)
	sysStat    *api.SysStat
	gpuStats   map[int]*api.GpuStat // latest per-GPU stats, keyed by GPU ID
	perfCursor string               // last-seen performance timestamp (?after=)

	// Log source filter
	logFilter logSourceFilter

	// Profiles
	profiles []api.Profile

	// Auto-refresh
	lastActivityRefresh time.Time

	// App version
	appVersion string

	// Horizontal scroll
	hScrollOffset int

	// Viewport for scrollable content
	vp viewport.Model

	// Log buffer
	logLines []string
	logMax   int // max lines to keep

	// Help
	help   help.Model
	helpShow bool

	// Error display
	errMsg string

	// Model load input prompt
	loadModelInput textinput.Model
	loadModelMode  bool // true when input prompt is active

	// Done channel for SSE goroutine
	done chan struct{}
}

// NewModel creates a new TUI model.
func NewModel(client *api.Client, version string) *Model {
	m := &Model{
		client:   client,
		tab:      tabActivity,
		conn:     connDisconnected,
		logMax:   500,
		help:     help.New(),
		done:     make(chan struct{}),
		appVersion: version,
	}
	m.vp = viewport.New(80, 24)
	m.loading = make(map[string]bool)
	m.loadModelInput = textinput.New()
	m.loadModelInput.Placeholder = "model name"
	m.loadModelInput.Prompt = "> "
	m.loadModelInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextTertiary))
	m.loadModelInput.Focus()
	return m
}

// SetProgram sets the program reference used to send messages from goroutines.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	m.startSSE()
	// Fetch initial data so content is visible as soon as the TUI opens.
	return tea.Batch(
		m.fetchVersion(),
		m.fetchModels(),
		m.fetchActivity(),
		m.fetchProfiles(),
		m.fetchHardware(),
		m.fetchPerformance(),
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleResize(msg)
		// Start auto-refresh timer on first update
		if m.lastActivityRefresh.IsZero() {
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return nil })
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		// Forward mouse wheel to the shared viewport so content scrolls.
		updatedVP, cmd := m.vp.Update(msg)
		m.vp = updatedVP
		return m, cmd

	case time.Time:
		// Auto-refresh every 5s: activity on the activity tab, and GPU/system
		// performance always (the SSE stream does not push perfsys/perfgpu
		// events, so /api/performance must be polled to keep stats live).
		var cmds []tea.Cmd
		if m.tab == tabActivity {
			cmds = append(cmds, m.fetchActivity())
		}
		cmds = append(cmds, m.fetchPerformance())
		cmds = append(cmds, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return nil }))
		return m, tea.Batch(cmds...)

	case ShutdownMsg:
		m.closeSSE()
		return m, tea.Quit

	case SSEErrMsg:
		m.conn = connDisconnected
		m.errMsg = msg.Error()
		return m, nil

	case SSEMsg:
		return m.handleSSE(msg)

	case ActivityRefreshMsg:
		m.lastActivityRefresh = time.Now()
		return m, nil

	case HardwareRefreshMsg:
		return m, nil

	case ModelsRefreshMsg:
		return m, nil

	case ProfilesRefreshMsg:
		return m, nil

	case LogLineMsg:
		return m.handleLogLine(msg)

	default:
		return m, nil
	}
}

// View implements tea.Model.
func (m *Model) View() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitle())
	b.WriteString("\n")

	// Tab bar
	b.WriteString(m.renderTabs())

	// Help overlay
	if m.helpShow {
		b.WriteString(m.renderHelpOverlay())
		return b.String()
	}

	// Newline to separate tab bar from tab content
	b.WriteString("\n")

	// Tab content — all tabs use the viewport for scrolling
	switch m.tab {
	case tabActivity:
		m.vp.SetContent(m.renderActivityView())
		b.WriteString(m.vp.View())
	case tabModels:
		m.vp.SetContent(m.renderModelsView())
		b.WriteString(m.vp.View())
	case tabHardware:
		m.vp.SetContent(m.renderHardwareView())
		b.WriteString(m.vp.View())
	case tabLogs:
		m.vp.SetContent(m.renderLogsContent())
		b.WriteString(m.vp.View())
	case tabProfiles:
		m.vp.SetContent(m.renderProfilesView())
		b.WriteString(m.vp.View())
	}

	// Error display
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusError)).
			Render("Error: " + m.errMsg))
	}

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// ---------------------------------------------------------------------------
// SSE management
// ---------------------------------------------------------------------------

func (m *Model) startSSE() {
	m.conn = connConnecting
	m.sse = api.NewSSEClient(m.client.BaseURL, nil)
	m.sse.Start(context.Background())

	go m.sseReader()
}

func (m *Model) sseReader() {
	eventsCh := m.sse.Events()
	errsCh := m.sse.Errors()
	for {
		select {
		case <-m.done:
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			if m.program != nil {
				m.program.Send(SSEMsg{Type: event.Type, Data: event.Data})
			}
		case err, ok := <-errsCh:
			if !ok {
				return
			}
			if m.program != nil {
				m.program.Send(SSEErrMsg(err))
			}
		}
	}
}

func (m *Model) closeSSE() {
	if m.sse != nil {
		close(m.done)
		m.sse.Close()
	}
}

// ---------------------------------------------------------------------------
// REST fetchers
// ---------------------------------------------------------------------------

func (m *Model) fetchVersion() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		v, err := m.client.GetVersion(ctx)
		if err == nil {
			m.version = *v
		}
		return nil
	}
}

func (m *Model) fetchActivity() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		page, err := m.client.GetActivity(ctx, api.ActivityQueryParams{
			Limit: 50,
			Order: "desc",
		})
		if err != nil {
			return fmt.Errorf("fetch activity: %w", err)
		}
		m.activityPage = *page
		m.lastActivityRefresh = time.Now()
		m.totalPages = page.TotalPages
		m.pageNum = page.Page

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		stats, statsErr := m.client.GetActivityStats(ctx2, "")
		if statsErr == nil {
			m.activityStats = stats
		}
		return ActivityRefreshMsg{}
	}
}

func (m *Model) fetchHardware() tea.Cmd {
	return func() tea.Msg {
		hw, err := m.client.GetHardware(context.Background())
		if err != nil {
			return fmt.Errorf("fetch hardware: %w", err)
		}
		m.hardware = *hw
		return HardwareRefreshMsg{}
	}
}

// fetchPerformance polls /api/performance, which the server appends one entry
// per device every 5s. It seeds the latest system stats and per-GPU stats so
// the live sections render without relying on SSE perf events.
func (m *Model) fetchPerformance() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := m.client.GetPerformance(ctx, m.perfCursor)
		if err != nil {
			return fmt.Errorf("fetch performance: %w", err)
		}

		var lastSys, lastGpu string
		if n := len(resp.SysStats); n > 0 {
			m.sysStat = &resp.SysStats[n-1]
			lastSys = resp.SysStats[n-1].Timestamp
		}
		if len(resp.GpuStats) > 0 {
			if m.gpuStats == nil {
				m.gpuStats = make(map[int]*api.GpuStat)
			}
			for i := range resp.GpuStats {
				gs := resp.GpuStats[i]
				m.gpuStats[gs.ID] = &gs
				if gs.Timestamp > lastGpu {
					lastGpu = gs.Timestamp
				}
			}
		}

		// Use the older of the two cursors so neither list falls behind.
		if lastSys != "" && (lastGpu == "" || lastSys <= lastGpu) {
			m.perfCursor = lastSys
		} else if lastGpu != "" {
			m.perfCursor = lastGpu
		}

		return HardwareRefreshMsg{}
	}
}

func (m *Model) fetchModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.client.GetModels(context.Background())
		if err != nil {
			return fmt.Errorf("fetch models: %w", err)
		}
		m.models = models
		m.clampSelected()
		m.selectedModel = ""
		m.modelActivity = api.ActivityPage{}
		m.modelActivityStats = nil
		return ModelsRefreshMsg{}
	}
}

// clampSelected ensures m.selected is within valid bounds for the current models list.
func (m *Model) clampSelected() {
	if len(m.models) == 0 {
		m.selected = 0
	} else if m.selected >= len(m.models) {
		m.selected = len(m.models) - 1
	}
}

func (m *Model) fetchModelActivity(modelName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		page, err := m.client.GetActivity(ctx, api.ActivityQueryParams{
			Models:  []string{modelName},
			Limit:   30,
			Order:   "desc",
		})
		if err == nil {
			m.modelActivity = *page
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		stats, statsErr := m.client.GetActivityStats(ctx2, modelName)
		if statsErr == nil {
			m.modelActivityStats = stats
		}
		return ModelsRefreshMsg{}
	}
}

func (m *Model) fetchProfiles() tea.Cmd {
	return func() tea.Msg {
		state, err := m.client.GetProfiles(context.Background())
		if err != nil {
			return fmt.Errorf("fetch profiles: %w", err)
		}
		m.activeProfile = state.Active
		m.profiles = state.Profiles
		return ProfilesRefreshMsg{}
	}
}

// ---------------------------------------------------------------------------
// SSE event handling
// ---------------------------------------------------------------------------

func (m *Model) handleSSE(msg SSEMsg) (tea.Model, tea.Cmd) {
	// The server may wrap events in a "message" envelope:
	//   event:message
	//   data:{"type":"modelStatus","data":...}
	// Unwrap the envelope so the inner type is handled below.
	if msg.Type == "message" {
		var envelope struct {
			Type string `json:"type"`
			Data any    `json:"data"`
		}
		if err := json.Unmarshal([]byte(msg.Data), &envelope); err == nil {
			msg.Type = envelope.Type
			if envelope.Data != nil {
				switch v := envelope.Data.(type) {
				case string:
					msg.Data = v
				case map[string]any:
					b, _ := json.Marshal(v)
					msg.Data = string(b)
				case []any:
					b, _ := json.Marshal(v)
					msg.Data = string(b)
				default:
					b, _ := json.Marshal(v)
					msg.Data = string(b)
				}
			}
		}
	}

	switch msg.Type {
	case "modelStatus":
		m.conn = connConnected
		var models []api.Model
		if err := json.Unmarshal([]byte(msg.Data), &models); err == nil {
			m.models = models
			m.clampSelected()
		}

	case "logData":
		m.conn = connConnected
		var logData api.LogDataEnvelope
		if err := json.Unmarshal([]byte(msg.Data), &logData); err == nil {
			m.logLines = append(m.logLines, logData.Data)
			if len(m.logLines) > m.logMax {
				m.logLines = m.logLines[len(m.logLines)-m.logMax:]
			}
		}

	case "activity":
		m.conn = connConnected
		// New activity row — trigger refresh
		return m, m.fetchActivity()

	case "inflight":
		m.conn = connConnected
		var stats api.InFlightStats
		if err := json.Unmarshal([]byte(msg.Data), &stats); err != nil {
			break
		}
		switch stats.Operation {
		case "snapshot":
			m.inflight = stats.Requests
		case "upsert":
			if stats.Request != nil {
				found := false
				for i, r := range m.inflight {
					if r.ID == stats.Request.ID {
						m.inflight[i] = *stats.Request
						found = true
						break
					}
				}
				if !found {
					m.inflight = append(m.inflight, *stats.Request)
				}
			}
		case "remove":
			for i, r := range m.inflight {
				if r.ID == stats.ID {
					m.inflight = append(m.inflight[:i], m.inflight[i+1:]...)
					break
				}
			}
		}

	case "uiConfig":
		m.conn = connConnected
		// TODO: update UI config

	case "profileChanged":
		m.conn = connConnected
		// TODO: parse and update active profile

	case "perfsys":
		m.conn = connConnected
		var sysStat api.SysStat
		if err := json.Unmarshal([]byte(msg.Data), &sysStat); err == nil {
			m.sysStat = &sysStat
		}

	case "perfgpu":
		m.conn = connConnected
		var gpuStat api.GpuStat
		if err := json.Unmarshal([]byte(msg.Data), &gpuStat); err == nil {
			if m.gpuStats == nil {
				m.gpuStats = make(map[int]*api.GpuStat)
			}
			m.gpuStats[gpuStat.ID] = &gpuStat
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help overlay takes priority
	if m.helpShow {
		if msg.String() == "?" || msg.String() == "esc" {
			m.helpShow = false
			return m, nil
		}
		return m, nil
	}

	// Model load input prompt takes priority when active
	if m.loadModelMode {
		switch msg.String() {
		case "enter":
			name := m.loadModelInput.Value()
			m.loadModelMode = false
			m.loadModelInput.Blur()
			if name == "" {
				return m, nil
			}
			m.statusMsg = fmt.Sprintf("Loading model %s...", name)
			m.statusTime = time.Now()
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				err := m.client.LoadModel(ctx, name)
				if err != nil {
					m.statusMsg = fmt.Sprintf("Failed to load %s: %v", name, err)
				} else {
					m.statusMsg = fmt.Sprintf("Loading %s initiated", name)
				}
				m.statusTime = time.Now()
				return ModelsRefreshMsg{}
			}
		case "esc", "ctrl+c":
			m.loadModelMode = false
			m.loadModelInput.Blur()
			return m, nil
		default:
			inputModel, cmd := m.loadModelInput.Update(msg)
			m.loadModelInput = inputModel
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.closeSSE()
		return m, tea.Quit

	case "?":
		m.helpShow = true
		return m, nil

	case "r":
		return m, m.refreshCurrentTab()

	case "1":
		m.tab = tabActivity
	case "2":
		m.tab = tabModels
	case "3":
		m.tab = tabHardware
	case "4":
		m.tab = tabLogs
	case "5":
		m.tab = tabProfiles

	case "tab", "n":
		m.tab = (m.tab + 1) % tabCount
	case "shift+tab", "h", "p":
		m.tab = (m.tab - 1 + tabCount) % tabCount
	case "l":
		switch m.tab {
		case tabModels:
			return m, m.loadSelectedModel()
		default:
			m.tab = (m.tab + 1) % tabCount
		}
	case "L":
		switch m.tab {
		case tabModels:
			m.loadModelMode = true
			m.loadModelInput.Reset()
			m.loadModelInput.Focus()
			return m, nil
		}

	// Tab-specific navigation
	case "pgdown", "ctrl+d":
		switch m.tab {
		case tabActivity:
			return m, m.activityNextPage()
		case tabHardware:
			m.vp.PageDown()
		case tabLogs:
			m.vp.PageDown()
		}
	case "pgup", "ctrl+u":
		switch m.tab {
		case tabActivity:
			return m, m.activityPrevPage()
		case tabHardware:
			m.vp.PageUp()
		case tabLogs:
			m.vp.PageUp()
		}
	case "u":
		switch m.tab {
		case tabModels:
			return m, m.unloadSelectedModel()
		}
	case "x":
		switch m.tab {
		case tabModels:
			return m, m.cancelSelectedRequest()
		}
	case "f":
		switch m.tab {
		case tabLogs:
			m.cycleLogFilter()
		}
	case "end":
		switch m.tab {
		case tabLogs:
			m.logScrollToBottom()
		}
	case "j", "down":
		switch m.tab {
		case tabActivity:
			m.activityScrollDown()
		case tabHardware:
			m.vp.LineDown(1)
		case tabLogs:
			m.logScrollDown()
		case tabModels:
			return m, m.modelScrollDown()
		case tabProfiles:
			m.profileScrollDown()
		}
	case "k", "up":
		switch m.tab {
		case tabActivity:
			m.activityScrollUp()
		case tabHardware:
			m.vp.LineUp(1)
		case tabLogs:
			m.logScrollUp()
		case tabModels:
			return m, m.modelScrollUp()
		case tabProfiles:
			m.profileScrollUp()
		}
	case "enter":
		switch m.tab {
		case tabProfiles:
			return m, m.switchSelectedProfile()
		}
	case "right":
		if m.tab == tabActivity {
			m.hScrollOffset += 10
		}
		return m, nil
	case "left":
		if m.tab == tabActivity {
			m.hScrollOffset = max(0, m.hScrollOffset-10)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) refreshCurrentTab() tea.Cmd {
	switch m.tab {
	case tabActivity:
		return m.fetchActivity()
	case tabHardware:
		return tea.Batch(m.fetchHardware(), m.fetchPerformance())
	case tabModels:
		return m.fetchModels()
	case tabProfiles:
		return m.fetchProfiles()
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Resize
// ---------------------------------------------------------------------------

func (m *Model) handleResize(msg tea.WindowSizeMsg) {
	// Reserve lines for: title (2) + tab bar (1) + status bar (1) = 4
	contentH := msg.Height - 4
	if contentH < 4 {
		contentH = 4
	}
	m.vp.Width = msg.Width
	m.vp.Height = contentH
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

func (m *Model) renderTitle() string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render(" llama-swap-tui ") +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextSecondary)).
			Render(fmt.Sprintf("[%s]", m.client.BaseURL))
}

func (m *Model) renderTabs() string {
	// Box-drawing tab bar with active tab highlight
	var topLine, botLine strings.Builder
	topLine.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder)).Render("╭" + strings.Repeat("─", tabWidth*int(tabCount)+int(tabCount)-1) + "╮"))

	var rows []string
	for i := 0; i < int(tabCount); i++ {
		label := tab(i).String()
		if i == int(m.tab) {
			rows = append(rows, lipgloss.NewStyle().
				Background(lipgloss.Color(colorHighlight)).
				Foreground(lipgloss.Color(colorAccent)).
				Bold(true).
				Width(tabWidth).
				Align(lipgloss.Center).
				Render(label))
		} else {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorTextSecondary)).
				Width(tabWidth).
				Align(lipgloss.Center).
				Render(label))
		}
	}

	midLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render(tabBorderChar) +
		strings.Join(rows, lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder)).Render(tabBorderChar)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder)).Render(tabBorderChar)

	botLine.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder)).Render("╰" + strings.Repeat("─", tabWidth*int(tabCount)+int(tabCount)-1) + "╯"))

	return topLine.String() + "\n" + midLine + "\n" + botLine.String()
}

func (m *Model) renderStatusBar() string {
	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorStatusReady)).
		Render("●")
	if m.conn != connConnected {
		status = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusWarning)).
			Render("●")
	}

	parts := []string{
		status + " " + m.conn.String(),
	}
	if m.activeProfile != "" {
		parts = append(parts, "Profile: "+m.activeProfile)
	}
	if m.version.Version != "" && m.version.Version != "unknown" {
		parts = append(parts, "API:"+m.version.Version)
	}
	parts = append(parts, m.appVersion)
	parts = append(parts, "q", "r", "?")

	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextSecondary)).
		Render(strings.Join(parts, "  |  "))

	// Status bar with top border for visual separation
	contentW := m.vp.Width - 2
	bar = lipgloss.NewStyle().Width(contentW).Render(bar)
	return lipgloss.NewStyle().
		Border(lipgloss.Border{Top: separatorChar, TopLeft: "╭", TopRight: "╮"}).
		BorderForeground(lipgloss.Color(colorBorder)).
		Render(bar)
}

func (m *Model) renderHelpOverlay() string {
	overlay := lipgloss.NewStyle().
		Width(60).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorAccent)).
		Render(
			"Keyboard Shortcuts\n\n" +
				"  q / Ctrl+C       Quit\n" +
				"  ?                Toggle help\n" +
				"  r                Refresh current tab\n" +
				"  1-5              Switch tabs\n" +
				"  tab / shift+tab  Next / prev tab\n\n" +
				"  Activity (1):  j/k scroll  pgup/pgdown pages\n" +
				"  Models (2):    j/k navigate  l=load  L=load-by-name  u=unload  x=cancel\n" +
				"  Hardware (3):  j/k scroll  pgup/pgdown pages  r refresh\n" +
				"  Logs (4):      f cycle filter  end=scroll to bottom\n" +
				"  Profiles (5):  j/k navigate  enter=switch",
		)
	return lipgloss.NewStyle().
		Width(80).
		Height(20).
		Align(lipgloss.Center, lipgloss.Center).
		Render(overlay)
}

// ---------------------------------------------------------------------------
// Tab view stubs
// ---------------------------------------------------------------------------

func (m *Model) handleLogLine(line LogLineMsg) (tea.Model, tea.Cmd) {
	m.logLines = append(m.logLines, string(line))
	if len(m.logLines) > m.logMax {
		m.logLines = m.logLines[len(m.logLines)-m.logMax:]
	}
	// Auto-scroll to bottom
	if m.tab == tabLogs {
		m.vp.GotoBottom()
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
