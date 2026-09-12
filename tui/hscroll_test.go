package tui

import (
	"strings"
	"testing"

	"llama-swap-tui/api"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestApplyHScrollANSI(t *testing.T) {
	// A colored line (11 printable cells) and a long plain line.
	colored := "\x1b[1;31mHello world\x1b[0m"
	long := "abcdefghij" + strings.Repeat("x", 60) + "END"
	content := colored + "\n" + long

	// Offset 0: first 40 cells.
	got := applyHScroll(content, 0, 40)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if w := ansi.StringWidth(lines[1]); w != 40 {
		t.Fatalf("width at offset 0 = %d, want 40", w)
	}
	if !strings.Contains(lines[0], "Hello world") {
		t.Fatalf("colored line mangled at offset 0: %q", lines[0])
	}

	// Offset 33 (max scroll for a 73-cell line in a 40-cell window):
	// the window must contain the tail of the long line.
	got = applyHScroll(content, 33, 40)
	lines = strings.Split(strings.TrimRight(got, "\n"), "\n")
	if !strings.Contains(lines[1], "END") {
		t.Fatalf("scrolled window missing tail: %q", lines[1])
	}
	if w := ansi.StringWidth(lines[1]); w != 40 {
		t.Fatalf("width at offset 33 = %d, want 40", w)
	}

	// Over-scroll: must clamp (no blank line, tail visible).
	got = applyHScroll(content, 9999, 40)
	lines = strings.Split(strings.TrimRight(got, "\n"), "\n")
	if !strings.Contains(lines[1], "END") {
		t.Fatalf("over-scroll should clamp, got: %q", lines[1])
	}
}

func TestHScrollKeys(t *testing.T) {
	m := NewModel(nil, "test")
	m.tab = tabActivity
	handle := func(k tea.KeyType) {
		next, _ := m.handleKey(tea.KeyMsg(tea.Key{Type: k}))
		m = next.(*Model)
	}

	// Right arrow should advance the offset on the activity tab.
	handle(tea.KeyRight)
	if m.hScrollOffset != 10 {
		t.Fatalf("right arrow: offset = %d, want 10", m.hScrollOffset)
	}

	// Left arrow should decrement and clamp at 0.
	handle(tea.KeyLeft)
	if m.hScrollOffset != 0 {
		t.Fatalf("left arrow: offset = %d, want 0", m.hScrollOffset)
	}
	handle(tea.KeyLeft)
	if m.hScrollOffset != 0 {
		t.Fatalf("left arrow clamp: offset = %d, want 0", m.hScrollOffset)
	}

	// Other tabs should not move the offset.
	m.tab = tabHardware
	before := m.hScrollOffset
	handle(tea.KeyRight)
	if m.hScrollOffset != before {
		t.Fatalf("right arrow on hardware tab moved offset to %d", m.hScrollOffset)
	}
}

func TestActivityColumnsAdaptive(t *testing.T) {
	entry := api.ActivityLogEntry{
		ID:             1,
		Model:          "test-model",
		ReqPath:        "/v1/chat/completions",
		RespStatusCode: 200,
	}
	m := NewModel(nil, "test")
	m.activityPage.Data = []api.ActivityLogEntry{entry}

	// Wide terminal: token columns shown, prefill/decode too.
	m.vp.Width = 140
	wide := m.renderActivityView()
	for _, want := range []string{"Cached", "In", "Out", "P/s", "D/s"} {
		if !strings.Contains(wide, want) {
			t.Errorf("wide layout missing %q", want)
		}
	}

	// Narrow terminal: token columns dropped, prefill/decode kept.
	m.vp.Width = 100
	narrow := m.renderActivityView()
	if strings.Contains(narrow, "Cached") {
		t.Error("narrow layout should not show Cached column")
	}
	for _, want := range []string{"P/s", "D/s"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("narrow layout missing %q", want)
		}
	}

	// Header/row alignment: separator width matches header width.
	m.vp.Width = 140
	hdr := m.renderActivityHeader()
	headerLine := strings.SplitN(hdr, "\n", 2)[0]
	sepLine := strings.SplitN(hdr, "\n", 2)[1]
	if ansi.StringWidth(headerLine) != ansi.StringWidth(sepLine) {
		t.Errorf("header/separator width mismatch: %d vs %d",
			ansi.StringWidth(headerLine), ansi.StringWidth(sepLine))
	}
}
