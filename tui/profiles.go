package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Profiles view state
// ---------------------------------------------------------------------------

type profilesView struct {
	selected   int
	loading    bool
	statusMsg  string
	statusTime time.Time
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderProfilesHeader() string {
	var b strings.Builder

	// Dynamic column widths
	idW := 24
	descW := m.vp.Width - idW - 20 // 20 = space for " ◉ active" indicator + padding
	if descW < 20 {
		descW = 20
	}

	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Profiles") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", idW+descW+4)) + "\n")
	return b.String()
}

func (m *Model) renderProfilesBody() string {
	var b strings.Builder

	// Status message
	if m.statusMsg != "" && time.Since(m.statusTime) < 5*time.Second {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorLoading)).
			Render("  "+m.statusMsg) + "\n")
	}

	// Dynamic column widths
	idW := 24
	descW := m.vp.Width - idW - 20 // 20 = space for " ◉ active" indicator + padding
	if descW < 20 {
		descW = 20
	}

	// Fetch profiles if we don't have any
	if len(m.profiles) == 0 {
		b.WriteString("  No profiles configured.\n")
		return b.String()
	}

	for i, profile := range m.profiles {
		active := ""
		if profile.ID == m.activeProfile || profile.Description == m.activeProfile {
			active = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorStatusReady)).
				Bold(true).
				Render(" ◉ active")
		}

		desc := profile.Description
		if desc == "" {
			desc = "(no description)"
		}

		// Show pinned models
		pins := ""
		if len(profile.Pins) > 0 {
			var pinParts []string
			for model, target := range profile.Pins {
				pinParts = append(pinParts, fmt.Sprintf("%s→%s", model, target))
			}
			pins = " [" + strings.Join(pinParts, ", ") + "]"
		}

		line := fmt.Sprintf("  %-*s %-*s%s",
			idW, truncate(profile.ID, idW-1),
			descW, truncate(desc, descW-1),
			active,
		)
		if i == m.selected {
			line = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color(colorHighlight)).
				Render(line)
		}
		b.WriteString(line + "\n")

		if pins != "" && i == m.selected {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorTextSecondary)).
				Render("    Pins:"+pins) + "\n")
		}
	}

	// Footer hint
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextTertiary)).
		Render("  (j/k: navigate, enter: switch profile)"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m *Model) profileScrollDown() {
	if len(m.profiles) == 0 || m.selected < 0 || m.selected >= len(m.profiles)-1 {
		return
	}
	m.selected++
	m.vp.LineDown(1)
}

func (m *Model) profileScrollUp() {
	if len(m.profiles) == 0 || m.selected < 1 {
		return
	}
	m.selected--
	m.vp.LineUp(1)
}

func (m *Model) switchSelectedProfile() tea.Cmd {
	if len(m.profiles) == 0 || m.selected < 0 || m.selected >= len(m.profiles) {
		return nil
	}
	profile := m.profiles[m.selected]
	m.statusMsg = fmt.Sprintf("Switching to profile %s...", profile.ID)
	m.statusTime = time.Now()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		name := profile.ID
		state, err := m.client.SetActiveProfile(ctx, &name)
		return ProfileSwitchMsg{name: name, state: state, err: err}
	}
}
