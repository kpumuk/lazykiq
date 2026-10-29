// Package theme defines UI colors and styles.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// ColorPair holds palette alternatives without querying a terminal.
type ColorPair struct {
	Light, Dark color.Color
}

// Resolve selects a concrete color for the application-owned background.
func (c ColorPair) Resolve(isDark bool) color.Color {
	return lipgloss.LightDark(isDark)(c.Light, c.Dark)
}

// Theme defines all colors used throughout the UI.
type Theme struct {
	// Base colors
	Primary ColorPair

	// Text colors
	Text      ColorPair
	TextMuted ColorPair

	// Background colors
	Bg           ColorPair
	MetricsBarBg ColorPair

	// Border colors
	Border      ColorPair
	BorderFocus ColorPair

	// Accent colors
	TableSelectedFg ColorPair
	TableSelectedBg ColorPair
	Success         ColorPair
	Error           ColorPair
	Filter          ColorPair
	DangerBg        ColorPair

	// Metrics colors
	MetricsText ColorPair

	// Chart colors
	ChartAxis      ColorPair
	ChartLabel     ColorPair
	ChartHistogram ColorPair

	// Stack bar colors
	StackBarBg   ColorPair
	StackBarText ColorPair

	// JSON colors
	JSONKey         ColorPair
	JSONString      ColorPair
	JSONNumber      ColorPair
	JSONBool        ColorPair
	JSONNull        ColorPair
	JSONPunctuation ColorPair

	// Queue colors
	QueueText ColorPair
}

// DefaultTheme is the adaptive color scheme used by default.
// Use Open Color palette when possible to define colors: https://yeun.github.io/open-color/
var DefaultTheme = Theme{
	Primary: ColorPair{
		Light: lipgloss.Color("#B2003C"),
		Dark:  lipgloss.Color("#F73D68"),
	},

	// Text
	Text: ColorPair{
		Light: lipgloss.Color("#111827"),
		Dark:  lipgloss.Color("#F9FAFB"),
	},
	TextMuted: ColorPair{
		Light: lipgloss.Color("#6B7280"),
		Dark:  lipgloss.Color("#9CA3AF"),
	},

	// Backgrounds
	Bg: ColorPair{
		Light: lipgloss.Color("#F8F9FA"),
		Dark:  lipgloss.Color("#000000"),
	},
	MetricsBarBg: ColorPair{
		Light: lipgloss.Color("#1c7ed6"),
		Dark:  lipgloss.Color("#275DA9"),
	},

	// Borders
	Border: ColorPair{
		Light: lipgloss.Color("#D1D5DB"),
		Dark:  lipgloss.Color("#374151"),
	},
	BorderFocus: ColorPair{
		Light: lipgloss.Color("#B2003C"),
		Dark:  lipgloss.Color("#F73D68"),
	},

	// Accents
	TableSelectedFg: ColorPair{
		Light: lipgloss.Color("#F8F9FA"),
		Dark:  lipgloss.Color("#F8F9FA"),
	},
	TableSelectedBg: ColorPair{
		Light: lipgloss.Color("#1864AB"),
		Dark:  lipgloss.Color("#1C7ED6"),
	},
	Success: ColorPair{
		Light: lipgloss.Color("#16A34A"),
		Dark:  lipgloss.Color("#22C55E"),
	},
	Error: ColorPair{
		Light: lipgloss.Color("#FF0000"),
		Dark:  lipgloss.Color("#FF0000"),
	},
	Filter: ColorPair{
		Light: lipgloss.Color("#C026D3"),
		Dark:  lipgloss.Color("#E879F9"),
	},
	DangerBg: ColorPair{
		Light: lipgloss.Color("#FFE3E3"),
		Dark:  lipgloss.Color("#C92A2A"),
	},

	// Metrics
	MetricsText: ColorPair{
		Light: lipgloss.Color("#f8f9fa"),
		Dark:  lipgloss.Color("#f8f9fa"),
	},

	// Charts
	ChartAxis: ColorPair{
		Light: lipgloss.Color("#6B7280"),
		Dark:  lipgloss.Color("#9CA3AF"),
	},
	ChartLabel: ColorPair{
		Light: lipgloss.Color("#6B7280"),
		Dark:  lipgloss.Color("#9CA3AF"),
	},
	ChartHistogram: ColorPair{
		Light: lipgloss.Color("#B2003C"),
		Dark:  lipgloss.Color("#F73D68"),
	},

	// Stack bar
	StackBarBg: ColorPair{
		Light: lipgloss.Color("#F59F00"),
		Dark:  lipgloss.Color("#F59F00"),
	},
	StackBarText: ColorPair{
		Light: lipgloss.Color("#111827"),
		Dark:  lipgloss.Color("#111827"),
	},

	// JSON
	JSONKey: ColorPair{
		Light: lipgloss.Color("#364FC7"),
		Dark:  lipgloss.Color("#748FFC"),
	},
	JSONString: ColorPair{
		Light: lipgloss.Color("#2B8A3E"),
		Dark:  lipgloss.Color("#69DB7C"),
	},
	JSONNumber: ColorPair{
		Light: lipgloss.Color("#D9480F"),
		Dark:  lipgloss.Color("#FFA94D"),
	},
	JSONBool: ColorPair{
		Light: lipgloss.Color("#C92A2A"),
		Dark:  lipgloss.Color("#FF6B6B"),
	},
	JSONNull: ColorPair{
		Light: lipgloss.Color("#868E96"),
		Dark:  lipgloss.Color("#ADB5BD"),
	},
	JSONPunctuation: ColorPair{
		Light: lipgloss.Color("#495057"),
		Dark:  lipgloss.Color("#DEE2E6"),
	},

	// Queues
	QueueText: ColorPair{
		Light: lipgloss.Color("#1098AD"),
		Dark:  lipgloss.Color("#66D9E8"),
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
	StackBar  lipgloss.Style
	StackItem lipgloss.Style

	// Content
	ViewTitle lipgloss.Style
	ViewText  lipgloss.Style
	ViewMuted lipgloss.Style

	// Table
	TableHeader    lipgloss.Style
	TableSelected  lipgloss.Style
	TableSeparator lipgloss.Style
	ScrollbarTrack lipgloss.Style
	ScrollbarThumb lipgloss.Style

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
	QueuePaused lipgloss.Style
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

// NewStyles creates styles with the dark no-reply fallback palette.
func NewStyles() Styles {
	return NewStylesForBackground(true)
}

// NewStylesForBackground resolves the palette without terminal I/O.
// Bubble Tea performs output color-profile conversion.
func NewStylesForBackground(isDark bool) Styles {
	t := DefaultTheme
	return Styles{
		// Metrics bar
		MetricsBar: lipgloss.NewStyle().
			Foreground(t.MetricsText.Resolve(isDark)).
			Background(t.MetricsBarBg.Resolve(isDark)).
			Padding(0, 0),

		MetricsFill: lipgloss.NewStyle().
			Background(t.MetricsBarBg.Resolve(isDark)),

		MetricsLabel: lipgloss.NewStyle().
			Foreground(t.MetricsText.Resolve(isDark)).
			Background(t.MetricsBarBg.Resolve(isDark)),

		MetricsValue: lipgloss.NewStyle().
			Foreground(t.MetricsText.Resolve(isDark)).
			Background(t.MetricsBarBg.Resolve(isDark)).
			Bold(true),

		MetricLabel: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)),

		MetricValue: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)).
			Bold(true),

		// Navbar
		NavBar: lipgloss.NewStyle().
			Padding(0, 1),

		NavItem: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)).
			PaddingRight(1),

		NavKey: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)).
			Background(t.Border.Resolve(isDark)).
			Padding(0, 1),

		NavQuit: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)).
			PaddingRight(1),

		NavBrand: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)),

		// Stack bar
		StackBar: lipgloss.NewStyle().
			Padding(0, 1),

		StackItem: lipgloss.NewStyle().
			Foreground(t.StackBarText.Resolve(isDark)).
			Background(t.StackBarBg.Resolve(isDark)).
			Padding(0, 1),

		// Content
		ViewTitle: lipgloss.NewStyle().
			Foreground(t.Primary.Resolve(isDark)).
			Bold(true),

		ViewText: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)),

		ViewMuted: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)),

		// Table
		TableHeader: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)).
			Bold(true),

		TableSelected: lipgloss.NewStyle().
			Foreground(t.TableSelectedFg.Resolve(isDark)).
			Background(t.TableSelectedBg.Resolve(isDark)),

		TableSeparator: lipgloss.NewStyle().
			Foreground(t.Border.Resolve(isDark)),
		ScrollbarTrack: lipgloss.NewStyle().
			Foreground(t.Border.Resolve(isDark)),
		ScrollbarThumb: lipgloss.NewStyle().
			Foreground(t.BorderFocus.Resolve(isDark)),

		// Layout helpers
		BoxPadding: lipgloss.NewStyle().
			Padding(0, 1),

		BorderStyle: lipgloss.NewStyle().
			Foreground(t.Border.Resolve(isDark)),

		FocusBorder: lipgloss.NewStyle().
			Foreground(t.BorderFocus.Resolve(isDark)),

		ChartSuccess: lipgloss.NewStyle().
			Foreground(t.Success.Resolve(isDark)),

		ChartFailure: lipgloss.NewStyle().
			Foreground(t.Error.Resolve(isDark)),

		ChartAxis: lipgloss.NewStyle().
			Foreground(t.ChartAxis.Resolve(isDark)),

		ChartLabel: lipgloss.NewStyle().
			Foreground(t.ChartLabel.Resolve(isDark)),

		ChartHistogram: lipgloss.NewStyle().
			Foreground(t.ChartHistogram.Resolve(isDark)),

		JSONKey: lipgloss.NewStyle().
			Foreground(t.JSONKey.Resolve(isDark)),

		JSONString: lipgloss.NewStyle().
			Foreground(t.JSONString.Resolve(isDark)),

		JSONNumber: lipgloss.NewStyle().
			Foreground(t.JSONNumber.Resolve(isDark)),

		JSONBool: lipgloss.NewStyle().
			Foreground(t.JSONBool.Resolve(isDark)),

		JSONNull: lipgloss.NewStyle().
			Foreground(t.JSONNull.Resolve(isDark)),

		JSONPunctuation: lipgloss.NewStyle().
			Foreground(t.JSONPunctuation.Resolve(isDark)),

		QueuePaused: lipgloss.NewStyle().
			Foreground(t.Primary.Resolve(isDark)),

		QueueText: lipgloss.NewStyle().
			Foreground(t.QueueText.Resolve(isDark)),

		QueueWeight: lipgloss.NewStyle().
			Foreground(t.QueueText.Resolve(isDark)).
			Bold(true),

		ErrorTitle: lipgloss.NewStyle().
			Foreground(t.Error.Resolve(isDark)).
			Bold(true),

		ErrorBorder: lipgloss.NewStyle().
			Foreground(t.Error.Resolve(isDark)),

		FilterFocused: lipgloss.NewStyle().
			Foreground(t.MetricsText.Resolve(isDark)).
			Background(t.Filter.Resolve(isDark)),

		FilterBlurred: lipgloss.NewStyle().
			Foreground(t.Filter.Resolve(isDark)),

		// Context bar
		ContextBar: lipgloss.NewStyle().
			Padding(0, 1),

		ContextLabel: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)),

		ContextValue: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)),

		ContextKey: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)).
			Background(t.Border.Resolve(isDark)).
			Padding(0, 1),

		ContextDesc: lipgloss.NewStyle().
			Foreground(t.TextMuted.Resolve(isDark)),

		ContextDangerKey: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)).
			Background(t.DangerBg.Resolve(isDark)).
			Padding(0, 1),

		ContextDangerDesc: lipgloss.NewStyle().
			Foreground(t.Text.Resolve(isDark)),
	}
}
