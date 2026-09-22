package tui

import (
	"fmt"
	"strings"
	"testing"

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

// TestModelsRaceSwapData simulates the fetch goroutine swapping the activity
// page while the view renders. The renderer snapshots the data slice, so this
// must not panic (regression: index out of range during j/k navigation).
func TestModelsRaceSwapData(t *testing.T) {
	m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
	m.models = append(m.models, api.Model{Name: "model-00", State: "ready"})
	m.selected = 0
	m.modelActivity.Data = make([]api.ActivityLogEntry, 20)
	m.winW, m.winH = 100, 30
	m.recalcVP(1)
	m.tab = tabModels

	// Swap exactly like fetchModelActivity's goroutine does, repeatedly,
	// between renders.
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

// TestAllTabsLineCount verifies every tab renders exactly winH lines so the
// terminal never scrolls (regression: overflow hid the sticky header).
func TestAllTabsLineCount(t *testing.T) {
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
	for _, c := range cases {
		m := NewModel(&api.Client{BaseURL: "http://test"}, "test")
		m.winW, m.winH = 120, 52
		c.seed(m)
		m.tab = c.tab
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) != m.winH {
			t.Errorf("%v: total=%d want %d", c.tab, len(lines), m.winH)
		}
	}
}
