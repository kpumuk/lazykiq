package requestctx

import (
	"context"
	"errors"
	"testing"
)

func TestScopeAndControllerLifetimes(t *testing.T) {
	var scope Scope
	if scope.Context() == nil || scope.Context().Err() != nil {
		t.Fatal("zero scope must supply a live context")
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	scope.SetContext(parent)
	var controller Controller
	first := controller.Start(scope.Context())
	second := controller.Start(scope.Context())
	if !errors.Is(first.Err(), context.Canceled) || second.Err() != nil || parent.Err() != nil {
		t.Fatal("replacement must cancel only the old request")
	}
	controller.Cancel()
	if second.Err() == nil || parent.Err() != nil {
		t.Fatal("request cancellation must preserve the scope")
	}
	third := controller.Start(scope.Context())
	cancel()
	if !errors.Is(third.Err(), context.Canceled) {
		t.Fatal("parent cancellation did not reach the request")
	}
	if controller.Start(scope.Context()).Err() == nil {
		t.Fatal("new requests must retain canceled parent")
	}
}
