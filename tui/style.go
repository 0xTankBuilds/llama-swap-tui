package tui

import "github.com/charmbracelet/lipgloss"

// Color tokens — every color in the UI traces back here.
// Warm amber accent on dark terminal, semantic colors that read clearly.

// Structural colors
const (
	colorTextPrimary   = "#E8E4D9" // main text, readable on dark
	colorTextSecondary = "#8A8578" // supporting text, labels
	colorTextTertiary  = "#5C5850" // metadata, hints
	colorBackground    = "#1A1814" // terminal background (dark warm)
	colorBorder        = "#3A3630" // separation, subtle
	colorAccent        = "#D4A574" // warm amber — headers, active elements
	colorAccentMuted   = "#A07850" // muted amber — section sub-headers
	colorHighlight     = "#2A2218" // active tab background, subtle
)

// Semantic colors
const (
	colorStatusReady   = "#6B8E6B" // green — ready, success, proxy source
	colorStatusStarting = "#C4B840" // yellow — starting
	colorStatusStopping = "#D4A574" // amber — stopping (matches accent)
	colorStatusStopped  = "#5C5850" // gray — stopped
	colorStatusError    = "#C75656" // red — errors, 500s
	colorStatusWarning  = "#D4A574" // amber — warnings, 400s
	colorInfo           = "#0BB"   // blue — info, upstream source
	colorLoading        = "#C4B840" // yellow — loading state
)

// UI constants
const (
	tabWidth          = 12
	tabPadding        = 2
	contentPadding    = 4
	statusBarPadding  = 4
	separatorChar     = "─"
	tabBorderChar     = "│"
	fontWeightBold    = true
	fontWeightNormal  = false
	textAlignCenter   = lipgloss.Center
	textAlignLeft     = lipgloss.Left
)
