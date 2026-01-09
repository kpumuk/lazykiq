// Package theme defines UI colors and styles.
package theme

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Theme defines all colors used throughout the UI.
type Theme struct {
	// Base colors
	Primary compat.CompleteAdaptiveColor

	// Text colors
	Text      compat.CompleteAdaptiveColor
	TextMuted compat.CompleteAdaptiveColor

	// Background colors
	Bg           compat.CompleteAdaptiveColor
	MetricsBarBg compat.CompleteAdaptiveColor

	// Border colors
	Border      compat.CompleteAdaptiveColor
	BorderFocus compat.CompleteAdaptiveColor

	// Accent colors
	TableSelectedFg compat.CompleteAdaptiveColor
	TableSelectedBg compat.CompleteAdaptiveColor
	Success         compat.CompleteAdaptiveColor
	Error           compat.CompleteAdaptiveColor
	Filter          compat.CompleteAdaptiveColor
	DangerBg        compat.CompleteAdaptiveColor

	// Metrics colors
	MetricsText compat.CompleteAdaptiveColor

	// Chart colors
	ChartAxis      compat.CompleteAdaptiveColor
	ChartLabel     compat.CompleteAdaptiveColor
	ChartHistogram compat.CompleteAdaptiveColor

	// Stack bar colors
	StackBarBg   compat.CompleteAdaptiveColor
	StackBarText compat.CompleteAdaptiveColor

	// JSON colors
	JSONKey         compat.CompleteAdaptiveColor
	JSONString      compat.CompleteAdaptiveColor
	JSONNumber      compat.CompleteAdaptiveColor
	JSONBool        compat.CompleteAdaptiveColor
	JSONNull        compat.CompleteAdaptiveColor
	JSONPunctuation compat.CompleteAdaptiveColor

	// Queue colors
	QueueText compat.CompleteAdaptiveColor
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
	Bg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#F8F9FA"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#000000"), ANSI256: lipgloss.Color("0"), ANSI: lipgloss.Color("0")},
	},
	MetricsBarBg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#1c7ed6"), ANSI256: lipgloss.Color("33"), ANSI: lipgloss.Color("12")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#275DA9"), ANSI256: lipgloss.Color("25"), ANSI: lipgloss.Color("4")},
	},

	// Borders
	Border: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#D1D5DB"), ANSI256: lipgloss.Color("252"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#374151"), ANSI256: lipgloss.Color("238"), ANSI: lipgloss.Color("7")},
	},
	BorderFocus: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#B2003C"), ANSI256: lipgloss.Color("161"), ANSI: lipgloss.Color("13")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F73D68"), ANSI256: lipgloss.Color("204"), ANSI: lipgloss.Color("13")},
	},

	// Accents
	TableSelectedFg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#F8F9FA"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F8F9FA"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
	},
	TableSelectedBg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#1864AB"), ANSI256: lipgloss.Color("27"), ANSI: lipgloss.Color("4")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#1C7ED6"), ANSI256: lipgloss.Color("33"), ANSI: lipgloss.Color("4")},
	},
	Success: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#16A34A"), ANSI256: lipgloss.Color("34"), ANSI: lipgloss.Color("2")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#22C55E"), ANSI256: lipgloss.Color("70"), ANSI: lipgloss.Color("2")},
	},
	Error: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#FF0000"), ANSI256: lipgloss.Color("196"), ANSI: lipgloss.Color("9")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#FF0000"), ANSI256: lipgloss.Color("196"), ANSI: lipgloss.Color("9")},
	},
	Filter: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#C026D3"), ANSI256: lipgloss.Color("165"), ANSI: lipgloss.Color("13")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#E879F9"), ANSI256: lipgloss.Color("171"), ANSI: lipgloss.Color("13")},
	},
	DangerBg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#FFE3E3"), ANSI256: lipgloss.Color("224"), ANSI: lipgloss.Color("9")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#C92A2A"), ANSI256: lipgloss.Color("160"), ANSI: lipgloss.Color("1")},
	},

	// Metrics
	MetricsText: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#f8f9fa"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#f8f9fa"), ANSI256: lipgloss.Color("255"), ANSI: lipgloss.Color("15")},
	},

	// Charts
	ChartAxis: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#6B7280"), ANSI256: lipgloss.Color("240"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#9CA3AF"), ANSI256: lipgloss.Color("250"), ANSI: lipgloss.Color("7")},
	},
	ChartLabel: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#6B7280"), ANSI256: lipgloss.Color("240"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#9CA3AF"), ANSI256: lipgloss.Color("250"), ANSI: lipgloss.Color("7")},
	},
	ChartHistogram: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#B2003C"), ANSI256: lipgloss.Color("161"), ANSI: lipgloss.Color("13")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F73D68"), ANSI256: lipgloss.Color("204"), ANSI: lipgloss.Color("13")},
	},

	// Stack bar
	StackBarBg: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#F59F00"), ANSI256: lipgloss.Color("208"), ANSI: lipgloss.Color("3")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#F59F00"), ANSI256: lipgloss.Color("208"), ANSI: lipgloss.Color("3")},
	},
	StackBarText: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#111827"), ANSI256: lipgloss.Color("0"), ANSI: lipgloss.Color("0")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#111827"), ANSI256: lipgloss.Color("0"), ANSI: lipgloss.Color("0")},
	},

	// JSON
	JSONKey: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#364FC7"), ANSI256: lipgloss.Color("62"), ANSI: lipgloss.Color("4")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#748FFC"), ANSI256: lipgloss.Color("69"), ANSI: lipgloss.Color("4")},
	},
	JSONString: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#2B8A3E"), ANSI256: lipgloss.Color("28"), ANSI: lipgloss.Color("2")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#69DB7C"), ANSI256: lipgloss.Color("78"), ANSI: lipgloss.Color("2")},
	},
	JSONNumber: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#D9480F"), ANSI256: lipgloss.Color("166"), ANSI: lipgloss.Color("3")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#FFA94D"), ANSI256: lipgloss.Color("215"), ANSI: lipgloss.Color("3")},
	},
	JSONBool: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#C92A2A"), ANSI256: lipgloss.Color("160"), ANSI: lipgloss.Color("1")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#FF6B6B"), ANSI256: lipgloss.Color("203"), ANSI: lipgloss.Color("1")},
	},
	JSONNull: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#868E96"), ANSI256: lipgloss.Color("245"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#ADB5BD"), ANSI256: lipgloss.Color("249"), ANSI: lipgloss.Color("7")},
	},
	JSONPunctuation: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#495057"), ANSI256: lipgloss.Color("240"), ANSI: lipgloss.Color("8")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#DEE2E6"), ANSI256: lipgloss.Color("253"), ANSI: lipgloss.Color("7")},
	},

	// Queues
	QueueText: compat.CompleteAdaptiveColor{
		Light: compat.CompleteColor{TrueColor: lipgloss.Color("#1098AD"), ANSI256: lipgloss.Color("30"), ANSI: lipgloss.Color("6")},
		Dark:  compat.CompleteColor{TrueColor: lipgloss.Color("#66D9E8"), ANSI256: lipgloss.Color("81"), ANSI: lipgloss.Color("6")},
	},
}

// Styles holds all lipgloss styles derived from a theme.
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
	NavBar   lipgloss.Style
	NavItem  lipgloss.Style
	NavKey   lipgloss.Style
	NavQuit  lipgloss.Style
	NavBrand lipgloss.Style

	// Stack bar
	StackBar       lipgloss.Style
	StackItem      lipgloss.Style
	StackArrowLeft lipgloss.Style
	StackArrow     lipgloss.Style

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
	ChartAxis      lipgloss.Style
	ChartLabel     lipgloss.Style
	ChartSuccess   lipgloss.Style
	ChartFailure   lipgloss.Style
	ChartHistogram lipgloss.Style

	// JSON highlighting
	JSONKey         lipgloss.Style
	JSONString      lipgloss.Style
	JSONNumber      lipgloss.Style
	JSONBool        lipgloss.Style
	JSONNull        lipgloss.Style
	JSONPunctuation lipgloss.Style

	// Queues
	QueueText   lipgloss.Style
	QueueWeight lipgloss.Style

	// Errors
	ErrorTitle  lipgloss.Style
	ErrorBorder lipgloss.Style

	// Frame title filter
	FilterFocused lipgloss.Style
	FilterBlurred lipgloss.Style

	// Context bar
	ContextBar        lipgloss.Style
	ContextLabel      lipgloss.Style
	ContextValue      lipgloss.Style
	ContextKey        lipgloss.Style
	ContextDesc       lipgloss.Style
	ContextDangerKey  lipgloss.Style
	ContextDangerDesc lipgloss.Style
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

		NavBrand: lipgloss.NewStyle().
			Foreground(t.TextMuted),

		// Stack bar
		StackBar: lipgloss.NewStyle().
			Padding(0, 1),

		StackItem: lipgloss.NewStyle().
			Foreground(t.StackBarText).
			Background(t.StackBarBg).
			Padding(0, 1),

		StackArrow: lipgloss.NewStyle().
			Foreground(t.StackBarBg),

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

		ChartAxis: lipgloss.NewStyle().
			Foreground(t.ChartAxis),

		ChartLabel: lipgloss.NewStyle().
			Foreground(t.ChartLabel),

		ChartHistogram: lipgloss.NewStyle().
			Foreground(t.ChartHistogram),

		JSONKey: lipgloss.NewStyle().
			Foreground(t.JSONKey),

		JSONString: lipgloss.NewStyle().
			Foreground(t.JSONString),

		JSONNumber: lipgloss.NewStyle().
			Foreground(t.JSONNumber),

		JSONBool: lipgloss.NewStyle().
			Foreground(t.JSONBool),

		JSONNull: lipgloss.NewStyle().
			Foreground(t.JSONNull),

		JSONPunctuation: lipgloss.NewStyle().
			Foreground(t.JSONPunctuation),

		QueueText: lipgloss.NewStyle().
			Foreground(t.QueueText),

		QueueWeight: lipgloss.NewStyle().
			Foreground(t.QueueText).
			Bold(true),

		ErrorTitle: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),

		ErrorBorder: lipgloss.NewStyle().
			Foreground(t.Error),

		FilterFocused: lipgloss.NewStyle().
			Foreground(t.MetricsText).
			Background(t.Filter),

		FilterBlurred: lipgloss.NewStyle().
			Foreground(t.Filter),

		// Context bar
		ContextBar: lipgloss.NewStyle().
			Padding(0, 1),

		ContextLabel: lipgloss.NewStyle().
			Foreground(t.TextMuted),

		ContextValue: lipgloss.NewStyle().
			Foreground(t.Text),

		ContextKey: lipgloss.NewStyle().
			Foreground(t.Text).
			Background(t.Border).
			Padding(0, 1),

		ContextDesc: lipgloss.NewStyle().
			Foreground(t.TextMuted),

		ContextDangerKey: lipgloss.NewStyle().
			Foreground(t.Text).
			Background(t.DangerBg).
			Padding(0, 1),

		ContextDangerDesc: lipgloss.NewStyle().
			Foreground(t.Text),
	}
}
