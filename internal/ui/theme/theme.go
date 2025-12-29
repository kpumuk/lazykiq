package theme

import "charm.land/lipgloss/v2"
import "charm.land/lipgloss/v2/compat"

// Theme defines all colors used throughout the UI.
type Theme struct {
	// Base colors
	Primary compat.CompleteAdaptiveColor

	// Text colors
	Text      compat.CompleteAdaptiveColor
	TextMuted compat.CompleteAdaptiveColor

	// Background colors
	Bg           compat.AdaptiveColor
	MetricsBarBg compat.CompleteAdaptiveColor

	// Border colors
	Border      compat.AdaptiveColor
	BorderFocus compat.CompleteAdaptiveColor

	// Accent colors
	TableSelectedFg compat.AdaptiveColor
	TableSelectedBg compat.AdaptiveColor
	Success         compat.AdaptiveColor
	Error           compat.AdaptiveColor

	// Metrics colors
	MetricsText compat.CompleteAdaptiveColor
}

// DefaultTheme is the adaptive color scheme used by default.
// Use Open Color palette when possible to define colors: https://yeun.github.io/open-color/
var DefaultTheme = Theme{
	Primary: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#B2003C"), ANSI256: lipgloss.Color("161"), ANSI: lipgloss.Color("13")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F73D68"), ANSI256: lipgloss.Color("204"), ANSI: lipgloss.Color("13")},
	},

	// Text
	Text: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#111827"), ANSI256: lipgloss.Color("0"), ANSI: lipgloss.Color("0")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F9FAFB"), ANSI256: lipgloss.Color("15"), ANSI: lipgloss.Color("15")},
	},
	TextMuted: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#6B7280"), ANSI256: lipgloss.Color("240"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#9CA3AF"), ANSI256: lipgloss.Color("250"), ANSI: lipgloss.Color("7")},
	},

	// Backgrounds
	Bg: compat.AdaptiveColor{
		Light: lipgloss.Color("15"),
		Dark:  lipgloss.Color("0"),
	},
	MetricsBarBg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#1c7ed6"), ANSI256: lipgloss.Color("33"), ANSI: lipgloss.Color("12")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#4dabf7"), ANSI256: lipgloss.Color("25"), ANSI: lipgloss.Color("4")},
	},

	// Borders
	Border: compat.AdaptiveColor{
		Light: lipgloss.Color("#D1D5DB"), // Gray-300
		Dark:  lipgloss.Color("#374151"), // Gray-700
	},
	BorderFocus: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#B2003C"), ANSI256: lipgloss.Color("161"), ANSI: lipgloss.Color("13")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F73D68"), ANSI256: lipgloss.Color("204"), ANSI: lipgloss.Color("13")},
	},

	// Accents
	TableSelectedFg: compat.AdaptiveColor{
		Light: lipgloss.Color("229"),
		Dark:  lipgloss.Color("229"),
	},
	TableSelectedBg: compat.AdaptiveColor{
		Light: lipgloss.Color("57"),
		Dark:  lipgloss.Color("57"),
	},
	Success: compat.AdaptiveColor{
		Light: lipgloss.Color("#16A34A"),
		Dark:  lipgloss.Color("#22C55E"),
	},
	Error: compat.AdaptiveColor{
		Light: lipgloss.Color("#FF0000"),
		Dark:  lipgloss.Color("#FF0000"),
	},

	// Metrics
	MetricsText: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#f8f9fa"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#f8f9fa"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
	},
}

// Styles holds all lipgloss styles derived from a theme
type Styles struct {
	// Metrics bar
	MetricsBar   lipgloss.Style
	MetricsFill  lipgloss.Style
	MetricsLabel lipgloss.Style
	MetricsValue lipgloss.Style
	MetricsSep   lipgloss.Style
	MetricLabel  lipgloss.Style
	MetricValue  lipgloss.Style

	// Navbar
	NavBar  lipgloss.Style
	NavItem lipgloss.Style
	NavKey  lipgloss.Style
	NavQuit lipgloss.Style

	// Content
	ViewTitle lipgloss.Style
	ViewText  lipgloss.Style
	ViewMuted lipgloss.Style

	// Table
	TableHeader    lipgloss.Style
	TableSelected  lipgloss.Style
	TableSeparator lipgloss.Style

	// Layout helpers
	BoxPadding  lipgloss.Style
	BorderStyle lipgloss.Style
	FocusBorder lipgloss.Style

	// Charts
	ChartSuccess lipgloss.Style
	ChartFailure lipgloss.Style

	// Errors
	ErrorTitle  lipgloss.Style
	ErrorBorder lipgloss.Style
}

// NewStyles creates a Styles instance from the default adaptive theme.
func NewStyles() Styles {
	t := DefaultTheme
	return Styles{
		// Metrics bar
		MetricsBar: lipgloss.NewStyle().
			Foreground(t.MetricsText).
			Background(t.MetricsBarBg).
			Padding(0, 0),

		MetricsFill: lipgloss.NewStyle().
			Background(t.MetricsBarBg),

		MetricsLabel: lipgloss.NewStyle().
			Foreground(t.MetricsText).
			Background(t.MetricsBarBg),

		MetricsValue: lipgloss.NewStyle().
			Foreground(t.MetricsText).
			Background(t.MetricsBarBg).
			Bold(true),

		MetricLabel: lipgloss.NewStyle().
			Foreground(t.TextMuted),

		MetricValue: lipgloss.NewStyle().
			Foreground(t.Text).
			Bold(true),

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

		// Table
		TableHeader: lipgloss.NewStyle().
			Foreground(t.Text).
			Bold(true),

		TableSelected: lipgloss.NewStyle().
			Foreground(t.TableSelectedFg).
			Background(t.TableSelectedBg),

		TableSeparator: lipgloss.NewStyle().
			Foreground(t.Border),

		// Layout helpers
		BoxPadding: lipgloss.NewStyle().
			Padding(0, 1),

		BorderStyle: lipgloss.NewStyle().
			Foreground(t.Border),

		FocusBorder: lipgloss.NewStyle().
			Foreground(t.BorderFocus),

		ChartSuccess: lipgloss.NewStyle().
			Foreground(t.Success),

		ChartFailure: lipgloss.NewStyle().
			Foreground(t.Error),

		ErrorTitle: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),

		ErrorBorder: lipgloss.NewStyle().
			Foreground(t.Error),
	}
}
