package dialogs

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
		testDialog: testDialog{id: "close"},
		msg:        tea.QuitMsg{},
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
