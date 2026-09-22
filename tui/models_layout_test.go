package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"llama-swap-tui/api"
)

// TestModelsLargeListClips reproduces the reported scenario: ~45 models in a
// tall terminal, scrolled to the bottom. The output must be exactly winH
// lines (the terminal never scrolls), and the sticky header plus the
// right-pane Activity section must stay visible.
func TestModelsLargeListClips(t *testing.T) {
	m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
	for i := 0; i < 45; i++ {
		m.models = append(m.models, api.Model{Name: fmt.Sprintf("model-%02d", i), State: "ready"})
	}
	m.selected = 0
	m.modelActivityStats = &api.ActivityStats{TotalRequests: 3}
	for i := 0; i < 30; i++ {
		m.modelActivity.Data = append(m.modelActivity.Data, api.ActivityLogEntry{ReqPath: "/test"})
	}
	m.winW, m.winH = 120, 52
	m.recalcVP(1)
	m.tab = tabModels

	// Scroll the model list to the bottom (as the user would with j/k),
	// interleaving a frame like the real app does.
	for i := 0; i < 45; i++ {
		if i > 0 {
			m.View() // previous frame keeps vp content current
		}
		m.modelScrollDown()
	}

	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.winH {
		t.Fatalf("total=%d want %d (winH)", len(lines), m.winH)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Name") {
		t.Errorf("header row missing from output")
	}
	if !strings.Contains(joined, "Activity") {
		t.Errorf("Activity section missing from output")
	}
	if !strings.Contains(joined, "Requests:") {
		t.Errorf("activity stats missing from output")
	}
	if !strings.Contains(joined, "model-44") {
		t.Errorf("bottom of list not visible (selected=%d, vp.Y=%d)", m.selected, m.vp.YOffset)
	}
}

// TestModelsSwapDataBetweenFrames steps through the fetch goroutine swapping
// the activity page between renders (sequentially). The renderer snapshots
// the data slice, so this must not panic.
func TestModelsSwapDataBetweenFrames(t *testing.T) {
	m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
	m.models = append(m.models, api.Model{Name: "model-00", State: "ready"})
	m.selected = 0
	m.modelActivity.Data = make([]api.ActivityLogEntry, 20)
	m.winW, m.winH = 100, 30
	m.recalcVP(1)
	m.tab = tabModels

	for i := 0; i < 200; i++ {
		m.modelActivity = api.ActivityPage{} // fetch reset (empty page)
		m.View()
		if i%2 == 0 {
			page := api.ActivityPage{}
			for j := 0; j < 20; j++ {
				page.Data = append(page.Data, api.ActivityLogEntry{ReqPath: "/test"})
			}
			m.modelActivity = page // new page arrives
		}
	}
}

// startFakeAPI serves minimal-but-valid JSON for every endpoint the fetch
// Cmds use, so the closures take their success paths.
func startFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	write := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/api/version", write(`{"version":"1.0","commit":"x","build_date":"y"}`))
	mux.HandleFunc("/api/metrics/activity",
		write(`{"data":[],"page":1,"limit":50,"total":0,"total_pages":0}`))
	mux.HandleFunc("/api/metrics/stats", write(`{}`))
	mux.HandleFunc("/v1/models", write(`{"data":[]}`))
	mux.HandleFunc("/api/hardware", write(`{}`))
	mux.HandleFunc("/api/profiles", write(`{"active":"default","profiles":[]}`))
	mux.HandleFunc("/api/performance",
		write(`{"sys_stats":[{"timestamp":"t1","mem_total_mb":100,"mem_used_mb":10}],` +
			`"gpu_stats":[{"timestamp":"t1","id":0,"name":"T","gpu_util_pct":10}]}`))
	return httptest.NewServer(mux)
}

// TestFetchCmdsRaceFree proves fetch Cmd goroutines never write Model state
// while View() runs. The closures hit a live (fake) API and return
// data-carrying messages; under -race any m.* write inside a closure races
// with the render loop.
func TestFetchCmdsRaceFree(t *testing.T) {
	srv := startFakeAPI(t)
	defer srv.Close()

	m := NewModel(api.NewClient(srv.URL, http.Header{}), "test")
	for i := 0; i < 45; i++ {
		m.models = append(m.models, api.Model{Name: fmt.Sprintf("model-%02d", i), State: "ready"})
	}
	m.winW, m.winH = 120, 52
	m.recalcVP(1)
	m.tab = tabModels

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			m.fetchVersion()()
			m.fetchActivity()()
			m.fetchModels()()
			m.fetchModelActivity("model-00")()
			m.fetchHardware()()
			m.fetchProfiles()()
			m.fetchPerformance()()
		}
	}()

	for i := 0; i < 3000; i++ {
		m.View()
	}
	<-done
}

// TestAllTabsLineCount verifies every tab renders no more than winH lines so
// the terminal never scrolls (regression: overflow hid the sticky header).
// Each tab fits exactly once winH reaches its structural floor:
// chrome (7: title 1 + tabs 3 + status 2 + gap 1) + sticky header + one
// content line. Below the floor the chrome cannot shrink, so the frame
// renders the floor — never more.
func TestAllTabsLineCount(t *testing.T) {
	floors := map[tab]int{
		tabActivity: 11, // header 3 (columns + separator)
		tabModels:   9,  // header 1
		tabHardware: 12, // header 4 (system info + separator)
		tabLogs:     9,  // header 1
		tabProfiles: 10, // header 2
	}
	cases := []struct {
		tab  tab
		seed func(m *Model)
	}{
		{tabActivity, func(m *Model) {
			m.activityPage.Data = make([]api.ActivityLogEntry, 60)
			for i := range m.activityPage.Data {
				m.activityPage.Data[i].ReqPath = "/test"
			}
		}},
		{tabModels, func(m *Model) {
			for i := 0; i < 45; i++ {
				m.models = append(m.models, api.Model{Name: "m"})
			}
		}},
		{tabHardware, func(m *Model) {
			m.hardware = api.HardwareSnapshot{}
		}},
		{tabLogs, func(m *Model) {
			m.logLines = make([]string, 40)
		}},
		{tabProfiles, func(m *Model) {
			m.profiles = []api.Profile{{ID: "a"}, {ID: "b"}}
		}},
	}
	for winH := 8; winH <= 20; winH++ {
		for _, c := range cases {
			m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
			m.winW, m.winH = 120, winH
			c.seed(m)
			m.tab = c.tab
			out := m.View()
			lines := strings.Split(out, "\n")
			max := floors[c.tab]
			if winH > max {
				max = winH
			}
			if len(lines) > max {
				t.Errorf("%v @winH=%d: total=%d lines, terminal scrolls (max %d)", c.tab, winH, len(lines), max)
			}
		}
	}
}

// TestActivityHeaderHScrollAligns verifies the sticky activity header
// h-scrolls with the body's shared offset, keeping columns aligned.
func TestActivityHeaderHScrollAligns(t *testing.T) {
	m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
	m.winW, m.winH = 120, 52
	m.recalcVP(2)
	m.tab = tabActivity
	m.activityPage.Data = make([]api.ActivityLogEntry, 10)
	for i := range m.activityPage.Data {
		// Long paths so the rows are wider than the viewport and the
		// shared offset clamp actually kicks in.
		m.activityPage.Data[i].ReqPath = strings.Repeat("/some/long/path/endpoint", 3)
	}

	// Baseline at offset 0 (eff offset 0 renders the full styled header).
	m.hScrollOffset = 0
	full := strings.Split(m.renderActivityHeader(), "\n")

	for off := 0; off <= 40; off += 5 {
		m.hScrollOffset = off
		eff := m.activityOffset()
		got := strings.Split(m.renderActivityHeader(), "\n")
		for i := range full {
			if i >= len(got) {
				break
			}
			want := ansi.Cut(full[i], eff, eff+m.vp.Width)
			if got[i] != want {
				t.Errorf("offset %d line %d: header not h-scrolled to body alignment", off, i)
			}
		}
	}

	// The shared offset must clamp against the widest row, not the header.
	m.hScrollOffset = 1000
	if off := m.activityOffset(); off == 0 || off > 200 {
		t.Errorf("shared offset not clamped to content: %d", off)
	}
}
