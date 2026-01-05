// Package views contains the main UI views.
package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

// Styles holds the view-related styles from the theme.
type Styles struct {
	Text            lipgloss.Style
	Muted           lipgloss.Style
	Title           lipgloss.Style
	MetricLabel     lipgloss.Style
	MetricValue     lipgloss.Style
	TableHeader     lipgloss.Style
	TableSelected   lipgloss.Style
	TableSeparator  lipgloss.Style
	BoxPadding      lipgloss.Style
	BorderStyle     lipgloss.Style
	FocusBorder     lipgloss.Style
	NavKey          lipgloss.Style
	ChartAxis       lipgloss.Style
	ChartLabel      lipgloss.Style
	ChartSuccess    lipgloss.Style
	ChartFailure    lipgloss.Style
	ChartHistogram  lipgloss.Style
	JSONKey         lipgloss.Style
	JSONString      lipgloss.Style
	JSONNumber      lipgloss.Style
	JSONBool        lipgloss.Style
	JSONNull        lipgloss.Style
	JSONPunctuation lipgloss.Style
	QueueText       lipgloss.Style
	QueueWeight     lipgloss.Style
	FilterFocused   lipgloss.Style
	FilterBlurred   lipgloss.Style
}

// RefreshMsg is broadcast by the app on the 5-second ticker.
// Views should respond by fetching their data.
type RefreshMsg struct{}

// ConnectionErrorMsg indicates a Redis connection error occurred.
// Views emit this when data fetching fails.
type ConnectionErrorMsg struct {
	Err error
}

// View defines the interface that all views must implement.
type View interface {
	// Init returns an initial command for the view
	Init() tea.Cmd

	// Update handles messages and returns the updated view and any commands
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view as a string
	View() string

	// Name returns the display name for this view (shown in navbar)
	Name() string

	// ShortHelp returns keybindings to show in the help view
	ShortHelp() []key.Binding

	// SetSize updates the view dimensions
	SetSize(width, height int) View

	// SetStyles updates the view styles
	SetStyles(styles Styles) View
}

// ContextItem represents a contextual label/value pair for the header.
type ContextItem struct {
	Label string
	Value string
}

// ContextProvider exposes contextual header items for the active view.
type ContextProvider interface {
	ContextItems() []ContextItem
}

// HintProvider exposes key hints for the header.
type HintProvider interface {
	HintBindings() []key.Binding
}

// HelpSection groups help bindings under a title.
type HelpSection struct {
	Title    string
	Bindings []key.Binding
	Lines    []string
	Column   HelpColumn
}

// HelpProvider exposes help sections for the active view.
type HelpProvider interface {
	HelpSections() []HelpSection
}

// HeaderLinesProvider exposes left-column header lines for the context bar.
type HeaderLinesProvider interface {
	HeaderLines() []string
}

// HelpColumn describes which column a section should render in.
type HelpColumn int

const (
	// HelpColumnAuto lets the dialog decide placement.
	HelpColumnAuto HelpColumn = iota
	// HelpColumnLeft forces placement in the left column.
	HelpColumnLeft
	// HelpColumnRight forces placement in the right column.
	HelpColumnRight
)

// TableHelpProvider exposes table-specific help bindings.
type TableHelpProvider interface {
	TableHelp() []key.Binding
}

// ShowJobDetailMsg requests a stacked job detail view.
type ShowJobDetailMsg struct {
	Job *sidekiq.JobRecord
}

// ShowErrorDetailsMsg requests a stacked error details view.
type ShowErrorDetailsMsg struct {
	DisplayClass string
	ErrorClass   string
	Queue        string
	Query        string
}

// ShowJobMetricsMsg requests a stacked job metrics view.
type ShowJobMetricsMsg struct {
	Job    string
	Period string
}

// JobDetailSetter allows setting job data on a job detail view.
type JobDetailSetter interface {
	SetJob(job *sidekiq.JobRecord)
}

// ErrorDetailsSetter allows setting error group data on an error details view.
type ErrorDetailsSetter interface {
	SetErrorGroup(displayClass, errorClass, queue, query string)
}

// JobMetricsSetter allows setting job metrics data on a job metrics view.
type JobMetricsSetter interface {
	SetJobMetrics(jobName, period string)
}

// Disposable allows views to clean up when removed from the stack.
type Disposable interface {
	Dispose()
}
