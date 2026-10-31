package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	palettedialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/palette"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

const (
	paletteBusyFlat = -1
	paletteBusyTree = -2
)

func (a App) paletteItems() []palettedialog.Item {
	items := make([]palettedialog.Item, 0, len(a.viewOrder)+4)
	for _, id := range a.viewOrder {
		label := a.viewRegistry[id].Name()
		if id == viewQueueDetails {
			label = "Queue jobs"
		}
		items = append(items, palettedialog.Item{Label: label, ID: int(id)})
	}
	return append(items,
		palettedialog.Item{Label: "Busy: flat jobs", ID: paletteBusyFlat},
		palettedialog.Item{Label: "Busy: tree by process", ID: paletteBusyTree},
		palettedialog.Item{Label: "Busy: processes", ID: int(viewProcessesList)},
		palettedialog.Item{Label: "Queues", ID: int(viewQueuesList)},
	)
}

func (a App) openPalette() dialogs.OpenDialogMsg {
	client := a.sidekiq
	model := palettedialog.New(a.paletteItems(), palettedialog.WithLoader(a.ctx, func(ctx context.Context) ([]palettedialog.Item, error) {
		queues, err := client.GetQueues(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]palettedialog.Item, 0, len(queues))
		for _, queue := range queues {
			items = append(items, palettedialog.Item{Label: "Queue: " + queue.Name(), ID: int(viewQueueDetails), Queue: queue.Name()})
		}
		return items, nil
	}))

	return dialogs.OpenDialogMsg{Model: model}
}

func (a *App) navigatePalette(id int) tea.Cmd {
	// Only destinations offered by this app instance may be selected.
	valid := false
	for _, item := range a.paletteItems() {
		if item.ID == id {
			valid = true
			break
		}
	}
	if !valid {
		return nil
	}
	switch id {
	case paletteBusyFlat, paletteBusyTree:
		cmd := a.setActiveView(viewBusy)
		if busy, ok := a.viewRegistry[viewBusy].(*views.Busy); ok {
			busy.SetTreeMode(id == paletteBusyTree)
		}
		return cmd
	case int(viewProcessesList):
		cmd := a.setActiveView(viewBusy)
		return tea.Batch(cmd, a.pushView(viewProcessesList))
	case int(viewQueuesList):
		cmd := a.setActiveView(viewQueueDetails)
		return tea.Batch(cmd, a.pushView(viewQueuesList))
	default:
		return a.setActiveView(viewID(id))
	}
}

func (a *App) navigatePaletteAction(action palettedialog.ActionMsg) tea.Cmd {
	if action.Queue != "" && action.ID == int(viewQueueDetails) {
		if setter, ok := a.viewRegistry[viewQueueDetails].(views.QueueDetailsSetter); ok {
			setter.SetQueue(action.Queue)
		}
		return a.setActiveView(viewQueueDetails)
	}
	return a.navigatePalette(action.ID)
}
