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

func TestOpenDialogAbandonment(t *testing.T) {
	for name, tc := range map[string]struct {
		accept bool
		cover  bool
	}{
		"rejected":                {},
		"accepted then duplicate": {accept: true},
		"accepted then covered":   {accept: true, cover: true},
	} {
		t.Run(name, func(t *testing.T) {
			abandoned := 0
			msg := (Opener{}).OpenDialog(&testDialog{id: "confirm"}, func() { abandoned++ })().(OpenDialogMsg)
			manager := NewDialogCmp()
			if abandoned != 0 {
				t.Fatal("command performed cleanup outside the event loop")
			}
			if tc.accept {
				manager, _ = manager.Update(msg)
				if tc.cover {
					manager, _ = manager.Update(OpenDialogMsg{Model: &testDialog{id: "help"}})
				}
			}
			session := manager.ActiveSession()
			msg.Reject()
			msg.Reject()
			manager, _ = manager.Update(msg)
			if manager.ActiveSession() != session {
				t.Fatal("duplicate or rejected open changed active session")
			}
			want := 1
			if tc.accept {
				want = 0
			}
			if abandoned != want {
				t.Fatalf("cleanup count = %d, want %d", abandoned, want)
			}
		})
	}
}

func TestOpenDialogReusedIDAbandonsNewPreparation(t *testing.T) {
	for name, cover := range map[string]bool{"active": false, "covered": true} {
		t.Run(name, func(t *testing.T) {
			manager := NewDialogCmp()
			original := &testDialog{id: "confirm"}
			manager, _ = manager.Update(OpenDialogMsg{Model: original})
			if cover {
				manager, _ = manager.Update(OpenDialogMsg{Model: &testDialog{id: "help"}})
			}
			abandoned := 0
			open := (Opener{}).OpenDialog(&testDialog{id: "confirm"}, func() { abandoned++ })
			manager, _ = manager.Update(open())
			if abandoned != 1 || manager.ActiveModel() != original {
				t.Fatal("reusing dialog state did not abandon unused preparation")
			}
			manager, _ = manager.Update(CloseDialogMsg{})
			manager, _ = manager.Update(open())
			if abandoned != 1 || manager.ActiveDialogID() == "confirm" {
				t.Fatal("abandoned open was revived by duplicate delivery")
			}
		})
	}
}
