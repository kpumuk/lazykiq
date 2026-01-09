package views

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// QueuesListInfo holds queue information for the list view.
type QueuesListInfo struct {
	Name          string
	Size          int64
	Latency       float64
	OldestJobTime time.Time
	HasOldestJob  bool
}

// queuesListDataMsg carries queues list data internally.
type queuesListDataMsg struct {
	queues []*QueuesListInfo
}

// QueuesList shows all Sidekiq queues in a table.
type QueuesList struct {
	client                  sidekiq.API
	width                   int
	height                  int
	styles                  Styles
	queues                  []*QueuesListInfo
	table                   table.Model
	ready                   bool
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
}

// NewQueuesList creates a new QueuesList view.
func NewQueuesList(client sidekiq.API) *QueuesList {
	return &QueuesList{
		client: client,
		table: table.New(
			table.WithColumns(queuesListColumns),
			table.WithEmptyMessage("No queues"),
		),
	}
}

// Init implements View.
func (q *QueuesList) Init() tea.Cmd {
	q.reset()
	return q.fetchDataCmd()
}

// Update implements View.
func (q *QueuesList) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case queuesListDataMsg:
		q.queues = msg.queues
		q.ready = true
		q.updateTableRows()
		return q, nil

	case RefreshMsg:
		return q, q.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return q, nil
		}
		if msg.Query == q.filter {
			return q, nil
		}
		q.filter = msg.Query
		q.table.SetCursor(0)
		return q, q.fetchDataCmd()

	case confirmdialog.ActionMsg:
		if !q.dangerousActionsEnabled {
			return q, nil
		}
		if !msg.Confirmed {
			return q, nil
		}
		if msg.Target == "" {
			return q, nil
		}
		return q, q.deleteQueueCmd(msg.Target)

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return q, func() tea.Msg {
				return dialogs.OpenDialogMsg{
					Model: filterdialog.New(
						filterdialog.WithStyles(q.filterStyle),
						filterdialog.WithQuery(q.filter),
					),
				}
			}
		case "enter":
			// Show queue details for selected queue
			if idx := q.table.Cursor(); idx >= 0 && idx < len(q.queues) {
				return q, func() tea.Msg {
					return ShowQueueDetailsMsg{QueueName: q.queues[idx].Name}
				}
			}
			return q, nil
		}

		if q.dangerousActionsEnabled {
			switch msg.String() {
			case "d":
				if queueName, ok := q.selectedQueueName(); ok {
					return q, func() tea.Msg {
						return dialogs.OpenDialogMsg{
							Model: newConfirmDialog(
								q.styles,
								"Delete queue",
								fmt.Sprintf(
									"Are you sure you want to delete the %s queue?\n\nThis will remove all jobs currently in the queue.\nThe queue will be created again automatically if you add new jobs to it later.",
									q.styles.Text.Bold(true).Render(queueName),
								),
								queueName,
								q.styles.DangerAction,
							),
						}
					}
				}
				return q, nil
			}
		}

		q.table, _ = q.table.Update(msg)
		return q, nil
	}

	return q, nil
}

// View implements View.
func (q *QueuesList) View() string {
	if !q.ready {
		return q.renderMessage("Loading...")
	}

	if len(q.queues) == 0 {
		if q.filter != "" {
			return q.renderMessage("No queues matching filter")
		}
		return q.renderMessage("No queues")
	}

	return q.renderQueuesBox()
}

// Name implements View.
func (q *QueuesList) Name() string {
	return "Select queue"
}

// ShortHelp implements View.
func (q *QueuesList) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (q *QueuesList) ContextItems() []ContextItem {
	items := []ContextItem{}

	// Calculate total items across all queues
	var totalItems int64
	var highestLatency float64
	var oldestJob time.Time

	for _, queue := range q.queues {
		totalItems += queue.Size
		if queue.Latency > highestLatency {
			highestLatency = queue.Latency
		}
		if queue.HasOldestJob {
			if oldestJob.IsZero() || queue.OldestJobTime.Before(oldestJob) {
				oldestJob = queue.OldestJobTime
			}
		}
	}

	items = append(items, ContextItem{Label: "Total Items", Value: format.Number(totalItems)})
	items = append(items, ContextItem{Label: "Highest Latency", Value: formatLatency(highestLatency)})
	if !oldestJob.IsZero() {
		items = append(items, ContextItem{Label: "Oldest Job", Value: oldestJob.Format("2006-01-02 15:04:05")})
	}

	return items
}

// HintBindings implements HintProvider.
func (q *QueuesList) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"enter"}, "enter", "view queue"),
	}
}

// MutationBindings implements MutationHintProvider.
func (q *QueuesList) MutationBindings() []key.Binding {
	if !q.dangerousActionsEnabled {
		return nil
	}
	return []key.Binding{
		helpBinding([]string{"d"}, "d", "delete queue"),
	}
}

// HelpSections implements HelpProvider.
func (q *QueuesList) HelpSections() []HelpSection {
	sections := []HelpSection{{
		Title: "Queue Actions",
		Bindings: []key.Binding{
			helpBinding([]string{"/"}, "/", "filter queues"),
			helpBinding([]string{"enter"}, "enter", "view queue details"),
		},
	}}
	if q.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"d"}, "d", "delete queue"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (q *QueuesList) TableHelp() []key.Binding {
	return tableHelpBindings(q.table.KeyMap)
}

// SetSize implements View.
func (q *QueuesList) SetSize(width, height int) View {
	q.width = width
	q.height = height
	q.updateTableSize()
	return q
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (q *QueuesList) SetDangerousActionsEnabled(enabled bool) {
	q.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (q *QueuesList) Dispose() {
	q.reset()
	q.updateTableSize()
}

// SetStyles implements View.
func (q *QueuesList) SetStyles(styles Styles) View {
	q.styles = styles
	q.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	q.frameStyles = frame.Styles{
		Focused: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterFocused,
			Border: styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterBlurred,
			Border: styles.BorderStyle,
		},
	}
	q.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Prompt:      styles.Text,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	return q
}

// fetchDataCmd fetches queues data from Redis.
func (q *QueuesList) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "queues.fetchDataCmd")

		queues, err := q.client.GetQueues(ctx)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		queueInfos := make([]*QueuesListInfo, 0, len(queues))
		for _, queue := range queues {
			// Apply filter
			if q.filter != "" && !strings.Contains(strings.ToLower(queue.Name()), strings.ToLower(q.filter)) {
				continue
			}

			size, _ := queue.Size(ctx)
			latency, _ := queue.Latency(ctx)

			info := &QueuesListInfo{
				Name:    queue.Name(),
				Size:    size,
				Latency: latency,
			}

			// Calculate oldest job timestamp from latency
			if size > 0 && latency > 0 {
				info.HasOldestJob = true
				info.OldestJobTime = time.Now().Add(-time.Duration(latency * float64(time.Second)))
			}

			queueInfos = append(queueInfos, info)
		}

		// Sort by name
		sort.Slice(queueInfos, func(i, j int) bool {
			return queueInfos[i].Name < queueInfos[j].Name
		})

		return queuesListDataMsg{
			queues: queueInfos,
		}
	}
}

func (q *QueuesList) reset() {
	q.ready = false
	q.queues = nil
	q.table.SetRows(nil)
	q.table.SetCursor(0)
}

func (q *QueuesList) selectedQueueName() (string, bool) {
	idx := q.table.Cursor()
	if idx < 0 || idx >= len(q.queues) {
		return "", false
	}
	return q.queues[idx].Name, true
}

// Table columns for queues list.
var queuesListColumns = []table.Column{
	{Title: "Name", Width: 30},
	{Title: "Size", Width: 15, Align: table.AlignRight},
	{Title: "Latency", Width: 15, Align: table.AlignRight},
	{Title: "Oldest Job", Width: 30},
}

// updateTableSize updates the table dimensions based on current view size.
func (q *QueuesList) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(q.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := q.width - 4
	q.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts queue data to table rows.
func (q *QueuesList) updateTableRows() {
	rows := make([]table.Row, 0, len(q.queues))
	for _, queue := range q.queues {
		oldestJobStr := ""
		if queue.HasOldestJob {
			oldestJobStr = queue.OldestJobTime.Format("2006-01-02 15:04:05")
		}

		row := table.Row{
			ID: queue.Name,
			Cells: []string{
				q.styles.QueueText.Render(queue.Name),
				format.Number(queue.Size),
				formatLatency(queue.Latency),
				oldestJobStr,
			},
		}
		rows = append(rows, row)
	}
	q.table.SetRows(rows)
	q.updateTableSize()
}

func (q *QueuesList) deleteQueueCmd(queueName string) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "queues.deleteQueueCmd")
		if err := q.client.NewQueue(queueName).Clear(ctx); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderQueuesBox renders the bordered box containing the queues table.
func (q *QueuesList) renderQueuesBox() string {
	// Build meta: queue count
	meta := q.styles.MetricLabel.Render("queues: ") + q.styles.MetricValue.Render(strconv.Itoa(len(q.queues)))

	// Calculate box height
	boxHeight := q.height

	// Get table content
	content := q.table.View()

	box := frame.New(
		frame.WithStyles(q.frameStyles),
		frame.WithTitle("Select queue"),
		frame.WithFilter(q.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(q.width, boxHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (q *QueuesList) renderMessage(msg string) string {
	box := messagebox.Render(messagebox.Styles{
		Title:  q.styles.Title,
		Muted:  q.styles.Muted,
		Border: q.styles.FocusBorder,
	}, "Select queue", msg, q.width, q.height)

	return box
}
