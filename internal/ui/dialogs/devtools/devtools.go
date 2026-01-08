// Package devtools provides a quake-style development console.
package devtools

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// DialogID identifies the dev tools dialog.
const DialogID dialogs.DialogID = "devtools"

// Styles holds the styles used by the dev tools console.
type Styles struct {
	Title          lipgloss.Style
	Border         lipgloss.Style
	Text           lipgloss.Style
	Muted          lipgloss.Style
	Prompt         lipgloss.Style
	Placeholder    lipgloss.Style
	Cursor         lipgloss.Style
	TableHeader    lipgloss.Style
	TableSelected  lipgloss.Style
	TableSeparator lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

var logColumns = []table.Column{
	{Title: "#", Width: 6, Align: table.AlignRight},
	{Title: "Time", Width: 12},
	{Title: "Origin", Width: 28},
	{Title: "Type", Width: 10},
	{Title: "Dur", Width: 8, Align: table.AlignRight},
	{Title: "Command", Width: 0},
}

type commandResultMsg struct {
	output string
	err    error
}

// Model defines state for the dev tools console.
type Model struct {
	styles         Styles
	title          string
	client         sidekiq.API
	tracker        *devtools.Tracker
	table          table.Model
	input          textinput.Model
	inputFocused   bool
	inputContainer lipgloss.Style
	width          int
	height         int
	windowWidth    int
	windowHeight   int
	row            int
	col            int
	padding        int
	minHeight      int
}

// Option configures the dev tools console.
type Option func(*Model)

// New creates a new dev tools console model.
func New(opts ...Option) *Model {
	m := &Model{
		styles:    DefaultStyles(),
		title:     "Dev Console",
		padding:   1,
		minHeight: 10,
		table: table.New(
			table.WithColumns(logColumns),
			table.WithEmptyMessage("No commands recorded."),
		),
		input: textinput.New(),
	}
	m.input.Prompt = "redis> "
	m.input.Placeholder = "enter redis command"

	for _, opt := range opts {
		opt(m)
	}

	m.applyStyles()
	m.applySize()
	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithTitle sets the dialog title.
func WithTitle(title string) Option {
	return func(m *Model) { m.title = title }
}

// WithTracker sets the dev tracker used for live updates.
func WithTracker(tracker *devtools.Tracker) Option {
	return func(m *Model) { m.tracker = tracker }
}

// WithClient sets the Sidekiq client used to execute Redis commands.
func WithClient(client sidekiq.API) Option {
	return func(m *Model) { m.client = client }
}

// Init focuses the input.
func (m *Model) Init() tea.Cmd {
	m.inputFocused = true
	return m.input.Focus()
}

// Update handles input and console lifecycle.
func (m *Model) Update(msg tea.Msg) (dialogs.DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.applySize()
		return m, nil
	case commandResultMsg:
		m.appendResult(msg.output, msg.err)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "f12", "~", "esc":
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
		case "tab", "shift+tab":
			return m, m.toggleFocus()
		case "enter":
			if m.inputFocused {
				return m, m.executeInput()
			}
		case "ctrl+u":
			if m.inputFocused {
				m.input.SetValue("")
				m.input.CursorEnd()
				return m, nil
			}
		}

		if m.inputFocused {
			switch msg.String() {
			case "up", "k", "down", "j", "pgup", "pgdown":
				updated, cmd := m.table.Update(msg)
				m.table = updated
				return m, cmd
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		updated, cmd := m.table.Update(msg)
		m.table = updated
		return m, cmd
	}

	return m, nil
}

// View renders the console.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	m.syncEntries()

	contentWidth := m.contentWidth()
	contentHeight := m.contentHeight()
	tableHeight := max(contentHeight-2, 1)
	m.table.SetSize(contentWidth, tableHeight)
	m.syncInputWidth(contentWidth)

	divider := strings.Repeat("─", contentWidth)
	if divider != "" {
		divider = m.styles.Muted.Render(divider)
	}

	tableView := m.table.View()
	if pad := tableHeight - lipgloss.Height(tableView); pad > 0 {
		tableView += strings.Repeat("\n", pad)
	}

	inputView := m.inputContainer.Render(m.input.View())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		tableView,
		divider,
		inputView,
	)

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
		frame.WithContent(content),
		frame.WithPadding(m.padding),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// Position returns the dialog position.
func (m *Model) Position() (int, int) {
	return m.row, m.col
}

// ID returns the dialog ID.
func (m *Model) ID() dialogs.DialogID {
	return DialogID
}

func (m *Model) toggleFocus() tea.Cmd {
	if m.inputFocused {
		m.inputFocused = false
		m.input.Blur()
		return nil
	}
	m.inputFocused = true
	return m.input.Focus()
}

func (m *Model) applyStyles() {
	m.table.SetStyles(table.Styles{
		Text:      m.styles.Text,
		Muted:     m.styles.Muted,
		Header:    m.styles.TableHeader,
		Selected:  m.styles.TableSelected,
		Separator: m.styles.TableSeparator,
	})
	inputStyles := m.input.Styles()
	inputStyles.Focused.Text = m.styles.Text
	inputStyles.Focused.Placeholder = m.styles.Placeholder
	inputStyles.Focused.Prompt = m.styles.Prompt
	inputStyles.Blurred.Text = m.styles.Text
	inputStyles.Blurred.Placeholder = m.styles.Placeholder
	inputStyles.Blurred.Prompt = m.styles.Prompt
	m.input.SetStyles(inputStyles)
}

func (m *Model) applySize() {
	if m.windowWidth == 0 || m.windowHeight == 0 {
		return
	}
	m.width = m.windowWidth
	height := m.windowHeight / 2
	height = max(height, m.minHeight)
	height = min(height, m.windowHeight-1)
	m.height = max(height, 1)
	m.row = 0
	m.col = 0
	contentWidth := m.contentWidth()
	m.inputContainer = lipgloss.NewStyle().Width(contentWidth).MaxWidth(contentWidth)
}

func (m *Model) contentWidth() int {
	return max(m.width-2-(m.padding*2), 0)
}

func (m *Model) contentHeight() int {
	return max(m.height-2, 0)
}

func (m *Model) syncInputWidth(contentWidth int) {
	promptWidth := lipgloss.Width(m.input.Prompt)
	width := max(contentWidth-promptWidth, 1)
	m.input.SetWidth(width)
}

func (m *Model) syncEntries() {
	if m.tracker == nil {
		m.table.SetRows(nil)
		return
	}
	prevRows := m.table.Rows()
	wasAtEnd := len(prevRows) == 0 || m.table.Cursor() >= len(prevRows)-1
	entries := m.tracker.LogEntries()
	rows := make([]table.Row, 0, len(entries))
	for _, entry := range entries {
		row := table.Row{
			ID: strconv.FormatUint(entry.Seq, 10),
			Cells: []string{
				strconv.FormatUint(entry.Seq, 10),
				entry.Time.Format("15:04:05.000"),
				entry.Origin,
				entryTypeLabel(entry.Entry.Kind),
				formatDuration(entry.Entry.Duration),
				entryCommandLabel(entry.Entry),
			},
		}
		rows = append(rows, row)
	}
	m.table.SetRows(rows)
	if wasAtEnd && len(rows) > 0 {
		m.table.MoveDown(len(rows))
	}
}

func entryTypeLabel(kind devtools.EntryKind) string {
	switch kind {
	case devtools.EntryCommand:
		return "command"
	case devtools.EntryPipelineBegin, devtools.EntryPipelineExec:
		return "pipeline"
	case devtools.EntryResult:
		return "result"
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
	case devtools.EntryResult:
		return entry.Command
	}
	return entry.Command
}

func (m *Model) executeInput() tea.Cmd {
	if m.client == nil {
		m.input.SetValue("")
		return func() tea.Msg {
			return commandResultMsg{err: errors.New("redis client not available")}
		}
	}

	raw := strings.TrimSpace(m.input.Value())
	m.input.SetValue("")
	m.input.CursorEnd()

	if raw == "" {
		return nil
	}

	return func() tea.Msg {
		args, err := parseRedisArgs(raw)
		if err != nil {
			return commandResultMsg{err: err}
		}
		ctx := devtools.WithOrigin(context.Background(), "console")
		result, err := m.client.Do(ctx, toAnyArgs(args)...)
		return commandResultMsg{output: formatRedisResult(result), err: err}
	}
}

func (m *Model) appendResult(output string, err error) {
	if m.tracker == nil {
		return
	}
	if err != nil {
		output = "error: " + err.Error()
	}
	output = normalizeOneLine(output)
	m.tracker.AppendLog(devtools.LogEntry{
		Time:   time.Now(),
		Origin: "console",
		Entry: devtools.Entry{
			Kind:     devtools.EntryResult,
			Command:  output,
			Duration: 0,
		},
	})
}

func parseRedisArgs(input string) ([]string, error) {
	var args []string
	var buf strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		args = append(args, buf.String())
		buf.Reset()
	}

	for _, r := range input {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			buf.WriteRune(r)
			continue
		}

		switch r {
		case '"', '\'':
			quote = r
		case ' ', '\t', '\n':
			flush()
		default:
			buf.WriteRune(r)
		}
	}

	if escaped || quote != 0 {
		return nil, errors.New("unterminated quoted string")
	}

	flush()
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	return args, nil
}

func toAnyArgs(args []string) []any {
	result := make([]any, len(args))
	for i, arg := range args {
		result[i] = arg
	}
	return result
}

func formatRedisResult(result any) string {
	if result == nil {
		return "(nil)"
	}
	switch value := result.(type) {
	case string:
		return normalizeOneLine(value)
	case []byte:
		return normalizeOneLine(string(value))
	case []string:
		return normalizeOneLine(strings.Join(value, " "))
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, fmt.Sprint(item))
		}
		return normalizeOneLine(strings.Join(parts, " "))
	default:
		return normalizeOneLine(fmt.Sprint(value))
	}
}

func normalizeOneLine(value string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "\\n",
		"\t", "\\t",
	)
	return replacer.Replace(value)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return devtools.FormatDuration(d)
}
