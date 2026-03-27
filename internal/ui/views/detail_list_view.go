package views

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

type detailListView struct {
	title       string
	width       int
	height      int
	styles      Styles
	lazy        lazytable.Model
	ready       bool
	filter      string
	frameStyles frame.Styles
	filterStyle filterdialog.Styles
}

func newDetailListView(
	title string,
	columns []table.Column,
	emptyMessage string,
	windowPages int,
	fallbackPageSize int,
) detailListView {
	lazy := lazytable.New(
		lazytable.WithTableOptions(
			table.WithColumns(columns),
			table.WithEmptyMessage(emptyMessage),
		),
		lazytable.WithWindowPages(windowPages),
		lazytable.WithFallbackPageSize(fallbackPageSize),
	)
	lazy.SetErrorHandler(func(err error) tea.Msg {
		return ConnectionErrorMsg{Err: err}
	})

	return detailListView{
		title: title,
		lazy:  lazy,
	}
}

func (s *detailListView) init(reset func()) tea.Cmd {
	reset()
	return requestLazyFromStart(&s.lazy)
}

func (s *detailListView) handleData(
	msg lazytable.DataMsg,
	apply func(lazytable.FetchResult),
) (bool, tea.Cmd) {
	if msg.RequestID != s.lazy.RequestID() {
		return false, nil
	}

	apply(msg.Result)
	s.ready = true

	var cmd tea.Cmd
	s.lazy, cmd = s.lazy.Update(msg)
	return true, cmd
}

func (s *detailListView) handleFilterAction(
	msg filterdialog.ActionMsg,
	updateEmptyMessage func(),
) tea.Cmd {
	if msg.Action == filterdialog.ActionNone || msg.Query == s.filter {
		return nil
	}
	return s.setFilter(msg.Query, updateEmptyMessage)
}

func (s *detailListView) handleKeyPress(
	msg tea.KeyPressMsg,
	updateEmptyMessage func(),
) (bool, tea.Cmd) {
	switch msg.String() {
	case "/":
		return true, s.openFilterDialog()
	case "ctrl+u":
		if s.filter == "" {
			return true, nil
		}
		return true, s.setFilter("", updateEmptyMessage)
	case "alt+left", "[":
		return true, moveLazyPage(&s.lazy, -1)
	case "alt+right", "]":
		return true, moveLazyPage(&s.lazy, 1)
	}
	return false, nil
}

func (s *detailListView) updateKeyPress(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	s.lazy, cmd = s.lazy.Update(msg)
	return cmd
}

func (s *detailListView) resetShell() {
	s.ready = false
	s.lazy.Reset()
}

func (s *detailListView) refreshWindow() tea.Cmd {
	return refreshLazyWindow(&s.lazy)
}

func (s *detailListView) reloadFromStart() tea.Cmd {
	return reloadLazyFromStart(&s.lazy)
}

func (s *detailListView) setFilter(filter string, updateEmptyMessage func()) tea.Cmd {
	s.filter = filter
	updateEmptyMessage()
	return s.reloadFromStart()
}

func (s detailListView) tableHelp() []key.Binding {
	return tableHelpBindings(s.lazy.Table().KeyMap)
}

func (s *detailListView) setSize(width, height int) {
	s.width = width
	s.height = height
	s.updateTableSize()
}

func (s *detailListView) updateTableSize() {
	tableWidth, tableHeight := framedTableSize(s.width, s.height)
	s.lazy.SetSize(tableWidth, tableHeight)
}

func (s *detailListView) dispose(reset func()) {
	reset()
	s.filter = ""
	s.setStyles(s.styles)
	s.updateTableSize()
}

func (s *detailListView) setStyles(styles Styles) {
	s.styles = styles
	s.frameStyles = frameStylesFromTheme(styles)
	s.filterStyle = filterDialogStylesFromTheme(styles)
	s.lazy.SetSpinnerStyle(styles.Muted)
	s.lazy.SetTableStyles(tableStylesFromTheme(styles))
}

func (s *detailListView) cancelRequests() {
	s.lazy.CancelRequest()
}

func (s detailListView) renderLoadingMessage() string {
	return renderStatusMessage(s.title, "Loading...", s.styles, s.width, s.height)
}

func (s detailListView) rowsMeta(rowCount int) string {
	label := s.styles.MetricLabel.Render("rows: ")
	start, end, total := s.lazy.Range()
	if total == 0 || rowCount == 0 {
		return label + s.styles.MetricValue.Render("0/0")
	}

	rangeLabel := fmt.Sprintf(
		"%s-%s/%s",
		display.Number(int64(start)),
		display.Number(int64(end)),
		display.Number(total),
	)
	return label + s.styles.MetricValue.Render(rangeLabel)
}

func (s detailListView) renderBox(title string, rowCount int) string {
	box := frame.New(
		frame.WithStyles(s.frameStyles),
		frame.WithTitle(title),
		frame.WithFilter(s.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(s.rowsMeta(rowCount)),
		frame.WithContent(s.lazy.View()),
		frame.WithPadding(1),
		frame.WithSize(s.width, s.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (s detailListView) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(s.filterStyle),
				filterdialog.WithQuery(s.filter),
			),
		}
	}
}
