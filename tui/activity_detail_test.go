package tui

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llama-swap-tui/api"

	tea "github.com/charmbracelet/bubbletea"
)

const detailTestCaptureID = "26112716-a7Kq"

// detailTestServer serves one canned capture for detailTestCaptureID
// and a 404 for any other ID, like a pruned capture.
func detailTestServer(t *testing.T) (*httptest.Server, *api.Client) {
	t.Helper()
	reqBody := base64.StdEncoding.EncodeToString([]byte(
		`{"model":"smoke-gemma","messages":[{"role":"user","content":"smoke test prompt"}],"stream":false}`))
	respBody := base64.StdEncoding.EncodeToString([]byte(
		`{"choices":[{"message":{"role":"assistant","content":"smoke test reply"}}]}`))
	mux := http.NewServeMux()
	mux.HandleFunc("/api/captures/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/captures/"+detailTestCaptureID {
			http.Error(w, "capture not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"req_path":"/v1/chat/completions",` +
			`"req_body":"` + reqBody + `","resp_body":"` + respBody + `"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, api.NewClient(srv.URL, http.Header{})
}

// newDetailTestModel builds a Model on the Activity tab with two rows:
// row 0 has a capture, row 1 does not.
func newDetailTestModel(client *api.Client) *Model {
	m := NewModel(client, "test")
	m.tab = tabActivity
	m.vp.Width = 120
	m.activityPage = api.ActivityPage{
		Data: []api.ActivityLogEntry{
			{ID: 1, Timestamp: time.Now(), Model: "smoke-gemma", ReqPath: "/v1/chat/completions", RespStatusCode: 200, HasCapture: true, CaptureID: detailTestCaptureID},
			{ID: 2, Timestamp: time.Now(), Model: "smoke-gemma", ReqPath: "/v1/chat/completions", RespStatusCode: 200, HasCapture: false},
		},
		Page: 1, Limit: 10, Total: 2, TotalPages: 1,
	}
	return m
}

func (m *Model) pressKey(t *testing.T, msg tea.KeyMsg) tea.Cmd {
	t.Helper()
	next, cmd := m.handleKey(msg)
	*m = *next.(*Model)
	return cmd
}

func enterKey() tea.KeyMsg { return tea.KeyMsg(tea.Key{Type: tea.KeyEnter}) }
func ctrlJKey() tea.KeyMsg { return tea.KeyMsg(tea.Key{Type: tea.KeyCtrlJ}) }
func ctrlKKey() tea.KeyMsg { return tea.KeyMsg(tea.Key{Type: tea.KeyCtrlK}) }

func TestStripControlChars(t *testing.T) {
	in := "a\x1b]52;c;SGVsbG8=\x07b\x00c\x7fd\u009be\tf\ng"
	want := "a]52;c;SGVsbG8=bcde\tf\ng"
	if got := stripControlChars(in); got != want {
		t.Errorf("stripControlChars(%q) = %q, want %q", in, got, want)
	}
}

func TestActivityDetailStripsControlSequences(t *testing.T) {
	m := newDetailTestModel(nil)
	m.detail = &activityDetail{
		entryID: 1, model: "smoke-gemma", time: "12:00:00",
		prompts: captureDetail{
			input:  "hello\x1b]52;c;SGVsbG8=\x07world",
			output: "plain",
		},
	}
	view := m.renderActivityView()
	// The OSC 52 sequence must be defused: no ESC introducing it and
	// no BEL terminating it. (Lipgloss styling emits its own ESC
	// sequences, so the check is on the OSC introducer, not ESC alone.)
	if strings.Contains(view, "\x1b]52;") || strings.Contains(view, "\x07") {
		t.Errorf("control sequence reached the rendered view:\n%q", view)
	}
	if !strings.Contains(view, "hello") || !strings.Contains(view, "world") {
		t.Errorf("captured text lost after sanitising:\n%s", view)
	}
}

func TestCaptureFetchedMsgStaleGeneration(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	openDetail(t, m)
	if m.detail.prompts.output != "smoke test reply" {
		t.Fatalf("output = %q, want loaded detail", m.detail.prompts.output)
	}

	// A result from an earlier open of the same row: same entry and
	// capture IDs, older generation. It must be ignored.
	stale := CaptureFetchedMsg{
		entryID:    m.detail.entryID,
		captureID:  m.detail.captureID,
		generation: m.detail.generation - 1,
		err:        errors.New("stale boom"),
	}
	next, _ := m.Update(stale)
	m = next.(*Model)
	if m.detail.err != "" {
		t.Errorf("stale result applied: err = %q", m.detail.err)
	}
	if m.detail.prompts.output != "smoke test reply" {
		t.Errorf("stale result clobbered prompts: output = %q", m.detail.prompts.output)
	}

	// The current generation is still accepted.
	current := CaptureFetchedMsg{
		entryID:    m.detail.entryID,
		captureID:  m.detail.captureID,
		generation: m.detail.generation,
		err:        errors.New("current boom"),
	}
	next, _ = m.Update(current)
	m = next.(*Model)
	if m.detail.err != "current boom" {
		t.Errorf("current-generation result ignored: err = %q", m.detail.err)
	}
}

func TestActivityDetailUnscrolledByHScroll(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	m.vp.Width = 40
	// A long path makes the table wider than the viewport so the
	// horizontal offset sticks.
	m.activityPage.Data[0].ReqPath = "/v1/chat/completions/" + strings.Repeat("x", 100)
	openDetail(t, m)

	m.hScrollOffset = 10
	view := m.renderActivityView()
	if !strings.Contains(view, "── Request #1") {
		t.Errorf("detail header clipped by h-scroll:\n%s", view)
	}
	if !strings.Contains(view, "smoke test prompt") {
		t.Errorf("detail prompt clipped by h-scroll:\n%s", view)
	}
}

func TestActivitySelectionKeepsRowVisible(t *testing.T) {
	m := newDetailTestModel(nil)
	rows := make([]api.ActivityLogEntry, 10)
	for i := range rows {
		rows[i] = api.ActivityLogEntry{ID: i + 1, Timestamp: time.Now(), Model: "m", RespStatusCode: 200}
	}
	m.activityPage.Data = rows
	m.vp.Height = 5
	m.vp.SetContent(m.renderActivityView())
	checkVisible := func() {
		t.Helper()
		rowLine := m.activityTableStartLine() + m.activitySelected
		if rowLine < m.vp.YOffset || rowLine >= m.vp.YOffset+m.vp.Height {
			t.Fatalf("selected row %d at line %d not visible (YOffset %d, height %d)",
				m.activitySelected, rowLine, m.vp.YOffset, m.vp.Height)
		}
	}
	for i := 0; i < 9; i++ {
		m.pressKey(t, runeKey('j'))
		checkVisible()
	}
	for i := 0; i < 9; i++ {
		m.pressKey(t, runeKey('k'))
		checkVisible()
	}
}

// TestActivitySelectionVisibleWithStats is the statistics-present
// counterpart of TestActivitySelectionKeepsRowVisible. It locates the
// selected row in the rendered content by its request path instead of
// trusting activityTableStartLine: the helper under test must not be
// the thing proving its own arithmetic.
func TestActivitySelectionVisibleWithStats(t *testing.T) {
	m := newDetailTestModel(nil)
	m.activityStats = &api.ActivityStats{TotalRequests: 10, TotalInputTokens: 100, TotalOutputTokens: 200, TotalCacheTokens: 50}
	rows := make([]api.ActivityLogEntry, 10)
	for i := range rows {
		rows[i] = api.ActivityLogEntry{ID: i + 1, Timestamp: time.Now(), Model: "m", ReqPath: fmt.Sprintf("/row-%02d", i+1), RespStatusCode: 200}
	}
	m.activityPage.Data = rows
	m.vp.Height = 5
	m.vp.SetContent(m.renderActivityView())
	checkVisible := func() {
		t.Helper()
		marker := fmt.Sprintf("/row-%02d", m.activitySelected+1)
		lines := strings.Split(m.renderActivityView(), "\n")
		rowLine := -1
		for i, line := range lines {
			if strings.Contains(line, marker) {
				rowLine = i
				break
			}
		}
		if rowLine < 0 {
			t.Fatalf("selected row %d (%s) not found in rendered view", m.activitySelected, marker)
		}
		if rowLine < m.vp.YOffset || rowLine >= m.vp.YOffset+m.vp.Height {
			t.Fatalf("selected row %d at rendered line %d not visible (YOffset %d, height %d)",
				m.activitySelected, rowLine, m.vp.YOffset, m.vp.Height)
		}
	}
	for i := 0; i < 9; i++ {
		m.pressKey(t, runeKey('j'))
		checkVisible()
	}
	for i := 0; i < 9; i++ {
		m.pressKey(t, runeKey('k'))
		checkVisible()
	}
}

func TestActivityDetailKeyboardScroll(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	openDetail(t, m)
	m.vp.Height = 5
	m.vp.SetContent(m.renderActivityView())
	if m.vp.YOffset != 0 {
		t.Fatalf("YOffset = %d, want 0 before scrolling", m.vp.YOffset)
	}

	m.pressKey(t, ctrlJKey())
	if m.vp.YOffset != 1 {
		t.Errorf("ctrl+j: YOffset = %d, want 1", m.vp.YOffset)
	}
	if m.detail == nil {
		t.Error("ctrl+j closed the detail, want it open")
	}
	m.pressKey(t, ctrlKKey())
	if m.vp.YOffset != 0 {
		t.Errorf("ctrl+k: YOffset = %d, want 0", m.vp.YOffset)
	}

	// With the detail closed, ctrl+j/ctrl+k do nothing.
	m.pressKey(t, escKey())
	m.pressKey(t, ctrlJKey())
	if m.vp.YOffset != 0 {
		t.Errorf("ctrl+j with detail closed: YOffset = %d, want 0", m.vp.YOffset)
	}

	if !strings.Contains(m.renderHelpOverlay(), "ctrl+j/k") {
		t.Error("help overlay does not document the detail scroll keys")
	}
}
func escKey() tea.KeyMsg { return tea.KeyMsg(tea.Key{Type: tea.KeyEsc}) }
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{r}})
}

// openDetail presses Enter on the selected row and applies the fetch
// message, returning the model with the detail loaded.
func openDetail(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.pressKey(t, enterKey())
	if cmd == nil {
		t.Fatal("Enter on a row with a capture: want a fetch Cmd, got nil")
	}
	msg := cmd()
	next, _ := m.Update(msg)
	*m = *next.(*Model)
	if m.detail == nil {
		t.Fatal("detail is nil after Enter + fetch")
	}
}

func TestActivityDetailToggleEnter(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)

	openDetail(t, m)
	if m.detail.loading {
		t.Error("detail still loading after fetch applied")
	}
	if m.detail.prompts.input != "user: smoke test prompt" {
		t.Errorf("input = %q, want %q", m.detail.prompts.input, "user: smoke test prompt")
	}
	if m.detail.prompts.output != "smoke test reply" {
		t.Errorf("output = %q, want %q", m.detail.prompts.output, "smoke test reply")
	}
	view := m.renderActivityView()
	if !strings.Contains(view, "smoke test prompt") || !strings.Contains(view, "smoke test reply") {
		t.Errorf("detail not rendered in activity view:\n%s", view)
	}

	// Enter again on the same row closes the detail.
	m.pressKey(t, enterKey())
	if m.detail != nil {
		t.Error("second Enter: detail still open, want closed")
	}
	if strings.Contains(m.renderActivityView(), "smoke test prompt") {
		t.Error("detail text still rendered after closing")
	}
}

func TestActivityDetailEscCloses(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	openDetail(t, m)

	m.pressKey(t, escKey())
	if m.detail != nil {
		t.Error("Esc: detail still open, want closed")
	}
	if strings.Contains(m.renderActivityView(), "smoke test prompt") {
		t.Error("detail text still rendered after Esc")
	}
}

func TestActivityDetailNoCapture(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)

	// Move to row 1 (no capture) and press Enter: a note, no fetch.
	m.pressKey(t, runeKey('j'))
	if m.activitySelected != 1 {
		t.Fatalf("activitySelected = %d, want 1", m.activitySelected)
	}
	cmd := m.pressKey(t, enterKey())
	if cmd != nil {
		t.Error("Enter on a row without a capture: want no fetch Cmd")
	}
	if m.detail == nil || !m.detail.noCapture {
		t.Error("detail missing or noCapture false for a row without a capture")
	}
	if !strings.Contains(m.renderActivityView(), "No capture available") {
		t.Errorf("no-capture note missing:\n%s", m.renderActivityView())
	}
}

func TestActivityDetailPruned404(t *testing.T) {
	srv, client := detailTestServer(t)
	_ = srv
	m := newDetailTestModel(client)
	// Point row 0 at a capture the server 404s, like a pruned one.
	m.activityPage.Data[0].CaptureID = "26112716-zzzz"

	openDetail(t, m)
	if !m.detail.pruned {
		t.Errorf("detail.pruned = false after 404, err = %q", m.detail.err)
	}
	if !strings.Contains(m.renderActivityView(), "no longer available") {
		t.Errorf("pruned note missing:\n%s", m.renderActivityView())
	}
}

func TestActivityDetailClosesOnSelectionMove(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	openDetail(t, m)

	m.pressKey(t, runeKey('j'))
	if m.detail != nil {
		t.Error("moving selection: detail still open, want closed")
	}
	if m.activitySelected != 1 {
		t.Errorf("activitySelected = %d, want 1", m.activitySelected)
	}
}

func TestActivityDetailClosesOnPageChange(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	m.totalPages = 2
	openDetail(t, m)

	cmd := m.activityNextPage()
	_ = cmd
	if m.detail != nil {
		t.Error("page change: detail still open, want closed")
	}
	if m.activitySelected != 0 {
		t.Errorf("activitySelected = %d, want 0 after page change", m.activitySelected)
	}
}

func TestActivityDetailSurvivesRefresh(t *testing.T) {
	_, client := detailTestServer(t)
	m := newDetailTestModel(client)
	openDetail(t, m)

	// Same rows, new page object: the detail stays open on the same entry.
	next, _ := m.Update(ActivityPageFetchedMsg{page: m.activityPage})
	m = next.(*Model)
	if m.detail == nil {
		t.Error("page refresh: detail closed, want it to stay open")
	}

	// The entry dropped off the page: the detail closes.
	empty := m.activityPage
	empty.Data = nil
	next, _ = m.Update(ActivityPageFetchedMsg{page: empty})
	m = next.(*Model)
	if m.detail != nil {
		t.Error("entry gone from page: detail still open, want closed")
	}
}

func TestActivitySelectionKeys(t *testing.T) {
	m := newDetailTestModel(nil)

	// j/k move the activity selection without touching the Models tab's.
	m.pressKey(t, runeKey('j'))
	if m.activitySelected != 1 {
		t.Fatalf("j: activitySelected = %d, want 1", m.activitySelected)
	}
	if m.selected != 0 {
		t.Errorf("j moved m.selected to %d, want it untouched", m.selected)
	}
	m.pressKey(t, runeKey('j'))
	if m.activitySelected != 1 {
		t.Errorf("j at last row: activitySelected = %d, want clamped to 1", m.activitySelected)
	}
	m.pressKey(t, runeKey('k'))
	if m.activitySelected != 0 {
		t.Errorf("k: activitySelected = %d, want 0", m.activitySelected)
	}
}
