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

// OpenDialog returns a command bound to the current owner, even if it runs late.
func (o Opener) OpenDialog(model DialogModel) tea.Cmd {
	msg := OpenDialogMsg{Model: model, Owner: o.owner}
	return func() tea.Msg { return msg }
}
