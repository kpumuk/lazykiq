package theme

import "github.com/charmbracelet/lipgloss"

// Theme defines all colors used throughout the UI
type Theme struct {
	Name string

	// Base colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color

	// Text colors
	Text       lipgloss.Color
	TextMuted  lipgloss.Color
	TextBright lipgloss.Color

	// Background colors
	Bg    lipgloss.Color
	BgAlt lipgloss.Color

	// Border colors
	Border      lipgloss.Color
	BorderFocus lipgloss.Color
}

// Dark is the default dark color scheme
var Dark = Theme{
	Name: "dark",

	// Sidekiq-inspired primary
	Primary:   lipgloss.Color("#DC2626"), // Red-600
	Secondary: lipgloss.Color("#6B7280"), // Gray-500

	// Text
	Text:       lipgloss.Color("#F9FAFB"), // Gray-50
	TextMuted:  lipgloss.Color("#9CA3AF"), // Gray-400
	TextBright: lipgloss.Color("#FFFFFF"), // White

	// Backgrounds
	Bg:    lipgloss.Color("#111827"), // Gray-900
	BgAlt: lipgloss.Color("#1F2937"), // Gray-800

	// Borders
	Border:      lipgloss.Color("#374151"), // Gray-700
	BorderFocus: lipgloss.Color("#6B7280"), // Gray-500
}

// Light is the light color scheme
var Light = Theme{
	Name: "light",

	// Sidekiq-inspired primary
	Primary:   lipgloss.Color("#DC2626"), // Red-600
	Secondary: lipgloss.Color("#6B7280"), // Gray-500

	// Text
	Text:       lipgloss.Color("#111827"), // Gray-900
	TextMuted:  lipgloss.Color("#6B7280"), // Gray-500
	TextBright: lipgloss.Color("#030712"), // Gray-950

	// Backgrounds
	Bg:    lipgloss.Color("#FFFFFF"), // White
	BgAlt: lipgloss.Color("#F3F4F6"), // Gray-100

	// Borders
	Border:      lipgloss.Color("#D1D5DB"), // Gray-300
	BorderFocus: lipgloss.Color("#9CA3AF"), // Gray-400
}

// Styles holds all lipgloss styles derived from a theme
type Styles struct {
	Theme Theme

	// Metrics bar
	MetricsBar  lipgloss.Style
	MetricLabel lipgloss.Style
	MetricValue lipgloss.Style
	MetricSep   lipgloss.Style

	// Navbar
	NavBar  lipgloss.Style
	NavItem lipgloss.Style
	NavKey  lipgloss.Style
	NavQuit lipgloss.Style

	// Content
	ViewTitle lipgloss.Style
	ViewText  lipgloss.Style
	ViewMuted lipgloss.Style
}

// NewStyles creates a Styles instance from a Theme
func NewStyles(t Theme) Styles {
	return Styles{
		Theme: t,

		// Metrics bar
		MetricsBar: lipgloss.NewStyle().
			Foreground(t.Text).
			Padding(0, 1),

		MetricLabel: lipgloss.NewStyle().
			Foreground(t.TextMuted),

		MetricValue: lipgloss.NewStyle().
			Foreground(t.Text).
			Bold(true),

		MetricSep: lipgloss.NewStyle().
			Foreground(t.Border),

		// Navbar
		NavBar: lipgloss.NewStyle().
			Padding(0, 1),

		NavItem: lipgloss.NewStyle().
			Foreground(t.TextMuted).
			PaddingRight(1),

		NavKey: lipgloss.NewStyle().
			Foreground(t.Text).
			Background(t.Border).
			Padding(0, 1),

		NavQuit: lipgloss.NewStyle().
			Foreground(t.TextMuted).
			PaddingRight(1),

		// Content
		ViewTitle: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		ViewText: lipgloss.NewStyle().
			Foreground(t.Text),

		ViewMuted: lipgloss.NewStyle().
			Foreground(t.TextMuted),
	}
}
