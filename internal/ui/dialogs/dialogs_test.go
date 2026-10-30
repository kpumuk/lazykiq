package dialogs

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type schedulingModel struct{ command tea.Cmd }

func (m schedulingModel) Init() tea.Cmd                       { return m.command }
func (m schedulingModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m schedulingModel) View() tea.View                      { return tea.NewView("") }

func TestBoundCommandPreservesScheduler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session := &Session{Owner: "owner"}
	var started, finished atomic.Int32
	ready := make(chan struct{})
	leaf := func() tea.Msg {
		if started.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil
		}
		finished.Add(1)
		return CloseDialogMsg{}
	}
	var bound atomic.Int32
	cmd := bindSession(tea.Sequence(
		tea.Batch(leaf, tea.Sequence(leaf, func() tea.Msg { return nil })),
		func() tea.Msg {
			if finished.Load() != 2 {
				return nil
			}
			return tea.Quit()
		},
	), session)
	cmd = BindCommand(cmd, func(msg tea.Msg) tea.Msg {
		if closeMsg, ok := msg.(CloseDialogMsg); ok && closeMsg.Session == session {
			bound.Add(1)
		}
		return msg
	})
	_, err := tea.NewProgram(schedulingModel{cmd}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer()).Run()
	if err != nil || bound.Load() != 2 {
		t.Fatalf("scheduler failed: err=%v, bound closes=%d", err, bound.Load())
	}
}

type testDialog struct {
	id        DialogID
	initCalls int
	updates   []tea.Msg
	width     int
	height    int
	row       int
	col       int
	view      string
}

func (d *testDialog) Init() tea.Cmd {
	d.initCalls++
	return nil
}

func (d *testDialog) Update(msg tea.Msg) (DialogModel, tea.Cmd) {
	d.updates = append(d.updates, msg)
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		d.width = size.Width
		d.height = size.Height
	}
	return d, nil
}

func (d *testDialog) View() string {
	return d.view
}

func (d *testDialog) Position() (int, int) {
	return d.row, d.col
}

func (d *testDialog) ID() DialogID {
	return d.id
}

type closeDialog struct {
	testDialog
	closed bool
	msg    tea.Msg
}

func (d *closeDialog) Close() tea.Cmd {
	d.closed = true
	if d.msg == nil {
		return nil
	}
	return func() tea.Msg { return d.msg }
}

func TestDialogCmpOpenClose(t *testing.T) {
	t.Parallel()

	cmp := NewDialogCmp()
	cmp, _ = cmp.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	dialog := &testDialog{id: "a"}
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialog})

	if !cmp.HasDialogs() {
		t.Fatal("expected dialogs to be present")
	}
	if dialog.initCalls != 1 {
		t.Fatalf("init calls = %d, want %d", dialog.initCalls, 1)
	}
	if dialog.width != 80 || dialog.height != 24 {
		t.Fatalf("dialog size = %dx%d, want 80x24", dialog.width, dialog.height)
	}
	if got := cmp.ActiveDialogID(); got != "a" {
		t.Fatalf("active id = %q, want %q", got, "a")
	}

	cmp, _ = cmp.Update(CloseDialogMsg{})
	if cmp.HasDialogs() {
		t.Fatal("expected dialogs to be closed")
	}
}

func TestDialogCmpReusesExistingDialog(t *testing.T) {
	t.Parallel()

	cmp := NewDialogCmp()
	cmp, _ = cmp.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	dialogA := &testDialog{id: "a"}
	dialogB := &testDialog{id: "b"}
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogA})
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogB})

	dialogA2 := &testDialog{id: "a"}
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogA2})

	if got := cmp.ActiveModel(); got != dialogA {
		t.Fatalf("active model = %p, want %p", got, dialogA)
	}
	if len(cmp.Dialogs()) != 2 {
		t.Fatalf("dialogs len = %d, want %d", len(cmp.Dialogs()), 2)
	}

	initCalls := dialogA.initCalls
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogA})
	if dialogA.initCalls != initCalls {
		t.Fatalf("init calls = %d, want %d", dialogA.initCalls, initCalls)
	}
	if len(cmp.Dialogs()) != 2 {
		t.Fatalf("dialogs len = %d, want %d", len(cmp.Dialogs()), 2)
	}
}

func TestDialogCmpCloseCallback(t *testing.T) {
	t.Parallel()

	cmp := NewDialogCmp()
	dialog := &closeDialog{
		id:  "close",
		msg: tea.QuitMsg{},
	}

	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialog})
	_, cmd := cmp.Update(CloseDialogMsg{})
	if !dialog.closed {
		t.Fatal("expected Close to be called")
	}
	if cmd == nil {
		t.Fatal("expected close cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("unexpected close message type %T", cmd())
	}
}

func TestDialogCmpForwardsUpdatesToActive(t *testing.T) {
	t.Parallel()

	cmp := NewDialogCmp()
	dialogA := &testDialog{id: "a"}
	dialogB := &testDialog{id: "b"}
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogA})
	cmp, _ = cmp.Update(OpenDialogMsg{Model: dialogB})

	key := tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'})
	_, _ = cmp.Update(key)

	if len(dialogA.updates) != 1 {
		t.Fatalf("dialogA updates = %d, want %d", len(dialogA.updates), 1)
	}
	if len(dialogB.updates) != 2 {
		t.Fatalf("dialogB updates = %d, want %d", len(dialogB.updates), 2)
	}
	if dialogB.updates[len(dialogB.updates)-1] != key {
		t.Fatalf("dialogB last update = %T, want key msg", dialogB.updates[len(dialogB.updates)-1])
	}
}

type completingDialog struct {
	testDialog
	action string
}

func (d *completingDialog) Update(msg tea.Msg) (DialogModel, tea.Cmd) {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		action := d.action
		return d, func() tea.Msg { return CloseDialogMsg{Action: action} }
	}
	d.testDialog.Update(msg)
	return d, nil
}

func TestDialogSessionRejectsLateClose(t *testing.T) {
	for name, steps := range map[string][]string{
		"replacement same instance": {"close", "open", "late"},
		"covered and revealed":      {"cover", "late", "close", "late"},
		"duplicate completion":      {"late", "open", "late"},
	} {
		t.Run(name, func(t *testing.T) {
			cmp := NewDialogCmp()
			d := &completingDialog{action: "original"}
			d.id = "a"
			cmp, _ = cmp.Update(OpenDialogMsg{Model: d, Owner: "owner"})
			cmp, cmd := cmp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			late := cmd().(CloseDialogMsg)
			if late.Session != cmp.ActiveSession() || late.Session.Owner != "owner" || late.Action != "original" {
				t.Fatalf("unbound completion: %#v", late)
			}
			for i, step := range steps {
				switch step {
				case "close":
					cmp, _ = cmp.Update(CloseDialogMsg{})
				case "open":
					cmp, _ = cmp.Update(OpenDialogMsg{Model: d, Owner: "replacement"})
				case "cover":
					cmp, _ = cmp.Update(OpenDialogMsg{Model: &testDialog{id: "b"}})
				case "late":
					active := cmp.ActiveModel()
					session := cmp.ActiveSession()
					cmp, _ = cmp.Update(late)
					if i > 0 && (cmp.ActiveModel() != active || cmp.ActiveSession() != session) {
						t.Fatal("stale completion changed dialog stack")
					}
				}
			}
		})
	}
}

func TestDialogBatchedCloseIsBound(t *testing.T) {
	session := &Session{Owner: "owner"}
	cmd := bindSession(tea.Batch(func() tea.Msg { return CloseDialogMsg{} }, func() tea.Msg { return "other" }), session)
	batch := cmd().(tea.BatchMsg)
	if got := batch[0]().(CloseDialogMsg).Session; got != session {
		t.Fatal("batched close lost session")
	}
	if got := batch[1](); got != "other" {
		t.Fatalf("unrelated message changed: %#v", got)
	}
}
