// Package dialogs provides a dialog stack and message types.
package dialogs

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// DialogID identifies a dialog instance.
type DialogID string

// DialogModel represents a dialog component that can be displayed.
type DialogModel interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (DialogModel, tea.Cmd)
	View() string
	Position() (int, int)
	ID() DialogID
}

// CloseCallback allows dialogs to perform cleanup when closed.
type CloseCallback interface {
	Close() tea.Cmd
}

// OpenDialogMsg is sent to open a new dialog.
type OpenDialogMsg struct {
	Model DialogModel
	Owner any
}

// Session identifies one activation of a dialog and its owner.
type Session struct {
	Owner any
}

// CloseDialogMsg closes a dialog and delivers its optional action atomically.
// The manager binds commands to their originating session before delivery.
type CloseDialogMsg struct {
	Session *Session
	Action  tea.Msg
}

// DialogCmp manages a stack of dialogs.
type DialogCmp interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (DialogCmp, tea.Cmd)
	View() string

	Dialogs() []DialogModel
	HasDialogs() bool
	GetLayers() []*lipgloss.Layer
	ActiveModel() DialogModel
	ActiveDialogID() DialogID
	ActiveSession() *Session
}

type dialogCmp struct {
	width, height int
	dialogs       []DialogModel
	idMap         map[DialogID]int
	sessions      map[DialogID]*Session
}

// NewDialogCmp creates a new dialog manager.
func NewDialogCmp() DialogCmp {
	return dialogCmp{
		dialogs:  []DialogModel{},
		idMap:    make(map[DialogID]int),
		sessions: make(map[DialogID]*Session),
	}
}

func (d dialogCmp) Init() tea.Cmd {
	return nil
}

// Update handles dialog lifecycle and forwards messages to the active dialog.
func (d dialogCmp) Update(msg tea.Msg) (DialogCmp, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		cmds := make([]tea.Cmd, 0, len(d.dialogs))
		d.width = msg.Width
		d.height = msg.Height
		for i := range d.dialogs {
			u, cmd := d.dialogs[i].Update(msg)
			d.dialogs[i] = u
			cmds = append(cmds, bindSession(cmd, d.sessions[u.ID()]))
		}
		return d, tea.Batch(cmds...)
	case OpenDialogMsg:
		return d.handleOpen(msg)
	case CloseDialogMsg:
		if len(d.dialogs) == 0 {
			return d, nil
		}
		if msg.Session != nil && msg.Session != d.ActiveSession() {
			return d, nil
		}
		inx := len(d.dialogs) - 1
		dialog := d.dialogs[inx]
		delete(d.idMap, dialog.ID())
		delete(d.sessions, dialog.ID())
		d.dialogs = d.dialogs[:len(d.dialogs)-1]
		if active := d.ActiveSession(); active != nil {
			// Revealing a covered dialog starts a new activation.
			d.sessions[d.ActiveDialogID()] = &Session{Owner: active.Owner}
		}
		if closeable, ok := dialog.(CloseCallback); ok {
			return d, closeable.Close()
		}
		return d, nil
	}

	if d.HasDialogs() {
		lastIndex := len(d.dialogs) - 1
		u, cmd := d.dialogs[lastIndex].Update(msg)
		d.dialogs[lastIndex] = u
		return d, bindSession(cmd, d.ActiveSession())
	}

	return d, nil
}

func (d dialogCmp) View() string {
	return ""
}

func (d dialogCmp) Dialogs() []DialogModel {
	return d.dialogs
}

func (d dialogCmp) ActiveModel() DialogModel {
	if len(d.dialogs) == 0 {
		return nil
	}
	return d.dialogs[len(d.dialogs)-1]
}

func (d dialogCmp) ActiveDialogID() DialogID {
	if len(d.dialogs) == 0 {
		return ""
	}
	return d.dialogs[len(d.dialogs)-1].ID()
}

func (d dialogCmp) ActiveSession() *Session {
	return d.sessions[d.ActiveDialogID()]
}

// bindSession captures identity before a command runs, including batched commands.
func bindSession(cmd tea.Cmd, session *Session) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case CloseDialogMsg:
			msg.Session = session
			return msg
		case tea.BatchMsg:
			bound := make(tea.BatchMsg, len(msg))
			for i, child := range msg {
				bound[i] = bindSession(child, session)
			}
			return bound
		default:
			return msg
		}
	}
}

func (d dialogCmp) GetLayers() []*lipgloss.Layer {
	layers := make([]*lipgloss.Layer, 0, len(d.dialogs))
	for i, dialog := range d.Dialogs() {
		dialogView := dialog.View()
		row, col := dialog.Position()
		layers = append(layers, lipgloss.NewLayer(dialogView).X(col).Y(row).Z(i+2))
	}
	return layers
}

func (d dialogCmp) HasDialogs() bool {
	return len(d.dialogs) > 0
}

func (d dialogCmp) handleOpen(msg OpenDialogMsg) (DialogCmp, tea.Cmd) {
	if d.HasDialogs() {
		dialog := d.dialogs[len(d.dialogs)-1]
		if dialog.ID() == msg.Model.ID() {
			return d, nil // already open on top
		}
	}

	// If the dialog is already in the stack, make it the last item and reuse state.
	if idx, ok := d.idMap[msg.Model.ID()]; ok {
		existing := d.dialogs[idx]
		msg.Model = existing
		msg.Owner = d.sessions[existing.ID()].Owner
		d.dialogs = slices.Delete(d.dialogs, idx, idx+1)
	}

	d.dialogs = append(d.dialogs, msg.Model)
	for i, dialog := range d.dialogs {
		d.idMap[dialog.ID()] = i
	}
	d.sessions[msg.Model.ID()] = &Session{Owner: msg.Owner}

	cmds := make([]tea.Cmd, 0, 2)
	cmds = append(cmds, msg.Model.Init())
	_, cmd := msg.Model.Update(tea.WindowSizeMsg{
		Width:  d.width,
		Height: d.height,
	})
	cmds = append(cmds, cmd)

	return d, bindSession(tea.Batch(cmds...), d.ActiveSession())
}
