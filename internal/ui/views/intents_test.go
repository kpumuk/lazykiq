package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestIntentScopeCapturesOwnerAndPayload(t *testing.T) {
	var scope IntentScope
	scope.SetIntentOwner("original")
	msg := ShowErrorDetailsMsg{Query: "original query"}
	cmd := scope.EmitIntent(msg)
	scope.SetIntentOwner("replacement")
	msg.Query = "replacement query"
	got := cmd().(IntentMsg)
	if got.Owner != "original" || got.Msg.(ShowErrorDetailsMsg).Query != "original query" {
		t.Fatalf("intent changed after creation: %#v", got)
	}
	if next := scope.EmitIntent(msg)().(IntentMsg); next.Owner != "replacement" {
		t.Fatal("subsequent intent retained the previous owner")
	}
}

func TestStandaloneIntentScope(t *testing.T) {
	var scope IntentScope
	msg := ShowProcessesListMsg{}
	if got := scope.EmitIntent(msg)(); got != tea.Msg(msg) {
		t.Fatalf("standalone intent = %#v", got)
	}
}
