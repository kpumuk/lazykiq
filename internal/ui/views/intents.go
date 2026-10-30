package views

import tea "charm.land/bubbletea/v2"

// IntentMsg retains the owner of a view-local dialog or navigation command.
type IntentMsg struct {
	Owner any
	Msg   tea.Msg
}

// IntentOwnerSetter receives the root's current view activation before Update
// or Init. Service commands keep their own ownership and are not affected.
type IntentOwnerSetter interface {
	SetIntentOwner(any)
}

// IntentScope supplies ownership to commands when they are created, before
// they can be composed into batches or sequences. Its zero value supports
// standalone views without a root router.
type IntentScope struct {
	owner any
}

// SetIntentOwner binds subsequent intents to a view activation.
func (s *IntentScope) SetIntentOwner(owner any) { s.owner = owner }

// EmitIntent snapshots both the intent and its owner before execution.
func (s IntentScope) EmitIntent(msg tea.Msg) tea.Cmd {
	if s.owner != nil {
		msg = IntentMsg{Owner: s.owner, Msg: msg}
	}
	return func() tea.Msg { return msg }
}
