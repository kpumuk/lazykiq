package views

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
)

func requestLazyFromStart(lazy *lazytable.Model) tea.Cmd {
	return lazy.RequestWindow(0, lazytable.CursorStart)
}

func reloadLazyFromStart(lazy *lazytable.Model) tea.Cmd {
	lazy.Table().SetCursor(0)
	return requestLazyFromStart(lazy)
}

func refreshLazyWindow(lazy *lazytable.Model) tea.Cmd {
	if lazy.Loading() {
		return nil
	}
	return lazy.RequestWindow(lazy.WindowStart(), lazytable.CursorKeep)
}

func moveLazyPage(lazy *lazytable.Model, delta int) tea.Cmd {
	lazy.MovePage(delta)
	return lazy.MaybePrefetch()
}
