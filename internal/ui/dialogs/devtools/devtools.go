// Package devtools provides a development diagnostics panel.
package devtools

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

// Styles holds the styles used by the dev tools panel.
type Styles struct {
	Title          lipgloss.Style
	Border         lipgloss.Style
	Text           lipgloss.Style
	Muted          lipgloss.Style
	TableHeader    lipgloss.Style
	TableSelected  lipgloss.Style
	TableSeparator lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

var devtoolsColumns = []table.Column{
	{Title: "#", Width: 4, Align: table.AlignRight},
	{Title: "Type", Width: 10},
	{Title: "Command", Width: 0},
}

// Model defines state for the dev tools panel.
type Model struct {
	styles  Styles
	title   string
	meta    string
	tracker *devtools.Tracker
	key     string
	width   int
	height  int
	padding int
	table   table.Model
	entries []devtools.Entry
}

// Option configures the dev tools panel.
type Option func(*Model)

// New creates a new dev tools panel model.
func New(opts ...Option) *Model {
	m := &Model{
		styles:  DefaultStyles(),
		title:   "Dev Commands",
		padding: 1,
		table: table.New(
			table.WithColumns(devtoolsColumns),
			table.WithEmptyMessage("No commands recorded."),
		),
	}

	for _, opt := range opts {
		opt(m)
	}

	m.applyStyles()
	m.updateTableSize()
	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithTitle sets the panel title.
func WithTitle(title string) Option {
	return func(m *Model) { m.title = title }
}

// WithMeta sets the panel meta string.
func WithMeta(meta string) Option {
	return func(m *Model) { m.meta = meta }
}

// WithTracker sets the dev tracker used for live updates.
func WithTracker(tracker *devtools.Tracker) Option {
	return func(m *Model) { m.tracker = tracker }
}

// WithKey sets the view key for live updates.
func WithKey(key string) Option {
	return func(m *Model) { m.key = key }
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.applyStyles()
}

// SetSize sets the panel dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.updateTableSize()
}

// SetMeta sets the panel meta string.
func (m *Model) SetMeta(meta string) {
	m.meta = meta
}

// SetTracker sets the tracker.
func (m *Model) SetTracker(tracker *devtools.Tracker) {
	m.tracker = tracker
}

// SetKey sets the tracker view key.
func (m *Model) SetKey(key string) {
	m.key = key
}

// Update handles input for the panel.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		updated, cmd := m.table.Update(msg)
		m.table = updated
		return m, cmd
	}

	return m, nil
}

// View renders the panel.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	m.syncEntries()
	meta := m.renderMeta()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Muted,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Muted,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle(m.title),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(m.table.View()),
		frame.WithPadding(m.padding),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (m *Model) applyStyles() {
	m.table.SetStyles(table.Styles{
		Text:      m.styles.Text,
		Muted:     m.styles.Muted,
		Header:    m.styles.TableHeader,
		Selected:  m.styles.TableSelected,
		Separator: m.styles.TableSeparator,
	})
}

func (m *Model) updateTableSize() {
	contentWidth := max(m.width-2-(m.padding*2), 0)
	contentHeight := max(m.height-2-(m.padding*2), 0)
	m.table.SetSize(contentWidth, contentHeight)
}

func (m *Model) syncEntries() {
	if m.tracker == nil || m.key == "" {
		m.entries = nil
		m.table.SetRows(nil)
		return
	}
	m.entries = m.tracker.Entries(m.key)
	rows := make([]table.Row, 0, len(m.entries))
	for i, entry := range m.entries {
		row := table.Row{
			ID: strconv.Itoa(i),
			Cells: []string{
				strconv.Itoa(i + 1),
				entryTypeLabel(entry.Kind),
				entryCommandLabel(entry),
			},
		}
		rows = append(rows, row)
	}
	m.table.SetRows(rows)
}

func entryTypeLabel(kind devtools.EntryKind) string {
	switch kind {
	case devtools.EntryCommand:
		return "command"
	case devtools.EntryPipelineBegin, devtools.EntryPipelineExec:
		return "pipeline"
	}
	return "command"
}

func entryCommandLabel(entry devtools.Entry) string {
	switch entry.Kind {
	case devtools.EntryCommand:
		return entry.Command
	case devtools.EntryPipelineBegin:
		return "pipeline begin"
	case devtools.EntryPipelineExec:
		return "pipeline execute"
	}
	return entry.Command
}

func (m *Model) renderMeta() string {
	meta := m.meta
	if m.tracker == nil || m.key == "" {
		return m.styleMeta(meta)
	}

	sample, ok := m.tracker.Sample(m.key)
	if !ok {
		return m.styleMeta(meta)
	}

	callLabel := "calls"
	if sample.Count == 1 {
		callLabel = "call"
	}

	if meta == "" {
		meta = fmt.Sprintf("%d %s | %s", sample.Count, callLabel, devtools.FormatDuration(sample.Duration))
	} else {
		meta = fmt.Sprintf("%s | %d %s | %s", meta, sample.Count, callLabel, devtools.FormatDuration(sample.Duration))
	}

	return m.styleMeta(meta)
}

func (m *Model) styleMeta(meta string) string {
	if meta == "" {
		return ""
	}
	return m.styles.Muted.Render(meta)
}
