package dialogs

import "testing"

func TestOpenerCapturesOwnerBeforeExecution(t *testing.T) {
	var opener Opener
	original := &Session{}
	replacement := &Session{}
	opener.SetDialogOwner(original)
	dialog := &testDialog{id: "filter"}
	old := opener.OpenDialog(dialog)
	opener.SetDialogOwner(replacement)
	current := opener.OpenDialog(dialog)
	if msg := old().(OpenDialogMsg); msg.Owner != original || msg.Model != dialog {
		t.Fatalf("delayed open = %#v, want original owner and dialog", msg)
	}
	if msg := current().(OpenDialogMsg); msg.Owner != replacement || msg.Model != dialog {
		t.Fatalf("current open = %#v, want replacement owner and dialog", msg)
	}
}
