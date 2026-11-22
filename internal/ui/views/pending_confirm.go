package views

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

// pendingConfirm tracks a pending confirmation action for sorted-entry views.
type pendingConfirm[T comparable] struct {
	action     T
	entry      *sidekiq.SortedEntry
	target     string
	generation uint64
}

func (p *pendingConfirm[T]) Set(action T, entry *sidekiq.SortedEntry, target string) {
	if p.target != "" {
		return
	}
	p.generation++
	p.action = action
	p.entry = entry
	p.target = fmt.Sprintf("sorted:%p:%d:%s", p, p.generation, target)
}

func (p *pendingConfirm[T]) SetForEntry(action T, entry *sidekiq.SortedEntry) {
	target := ""
	if entry != nil {
		target = entry.JID()
	}
	p.Set(action, entry, target)
}

func (p *pendingConfirm[T]) OpenDialog(opener dialogs.Opener, model *confirmdialog.Model) tea.Cmd {
	target := p.target
	return opener.OpenDialog(model, func() {
		if p.target == target {
			var none T
			p.Clear(none)
		}
	})
}

func (p *pendingConfirm[T]) Clear(none T) {
	p.action = none
	p.entry = nil
	p.target = ""
}

// Confirm clears the pending action on a matching confirmation message.
// It returns ok=true only when the action is confirmed.
func (p *pendingConfirm[T]) Confirm(msg confirmdialog.ActionMsg, enabled bool, none T) (T, *sidekiq.SortedEntry, bool) {
	var zero T
	if !enabled || p.action == none || msg.Target != p.target {
		return zero, nil, false
	}

	action := p.action
	entry := p.entry
	p.Clear(none)
	if !msg.Confirmed {
		return zero, nil, false
	}
	return action, entry, true
}
