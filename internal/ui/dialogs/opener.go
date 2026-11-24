package dialogs

import tea "charm.land/bubbletea/v2"

// OwnerSetter binds subsequent dialog commands to their originating activation.
type OwnerSetter interface {
	SetDialogOwner(owner any)
}

// Opener captures ownership when constructing a command, before composition.
// Its zero value supports dialogs used outside the application router.
type Opener struct {
	owner any
}

// SetDialogOwner sets the owner for subsequent dialog commands.
func (o *Opener) SetDialogOwner(owner any) { o.owner = owner }

// OpenDialog captures ownership and optional cleanup for a rejected open.
// Cleanup runs in the event loop, never from the asynchronous command.
func (o Opener) OpenDialog(model DialogModel, onRejected ...func()) tea.Cmd {
	msg := OpenDialogMsg{Model: model, Owner: o.owner}
	if len(onRejected) > 0 {
		msg.request = &openRequest{onRejected: onRejected[0]}
	}
	return func() tea.Msg { return msg }
}
