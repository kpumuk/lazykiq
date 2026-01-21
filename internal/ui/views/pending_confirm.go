package views

import (
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

// pendingConfirm tracks a pending confirmation action for sorted-entry views.
type pendingConfirm[T comparable] struct {
	action T
	entry  *sidekiq.SortedEntry
	target string
}

func (p *pendingConfirm[T]) Set(action T, entry *sidekiq.SortedEntry, target string) {
	p.action = action
	p.entry = entry
	p.target = target
}

func (p *pendingConfirm[T]) SetForEntry(action T, entry *sidekiq.SortedEntry) {
	target := ""
	if entry != nil {
		target = entry.JID()
	}
	p.Set(action, entry, target)
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
	if !enabled || p.action == none || (p.target != "" && msg.Target != p.target) {
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
