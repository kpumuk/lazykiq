// Package requestctx provides cancellable request controllers for UI fetches.
package requestctx

import (
	"context"
	"errors"
)

// Controller keeps at most one in-flight request alive for a caller.
type Controller struct {
	cancel context.CancelFunc
}

// Start cancels the previous request and returns a new cancellable context.
func (c *Controller) Start(parent context.Context) context.Context {
	c.Cancel()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	return ctx
}

// Cancel stops the current request, if any.
func (c *Controller) Cancel() {
	if c.cancel == nil {
		return
	}
	c.cancel()
	c.cancel = nil
}

// IsCanceled reports whether the error was caused by request cancellation.
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// Scope supplies the parent context for a component's reads and mutations.
// SetContext must be called before the component starts issuing commands.
// The zero value uses a background context for standalone components.
type Scope struct {
	context context.Context
}

// SetContext binds subsequent commands to the owner's lifetime.
func (s *Scope) SetContext(ctx context.Context) { s.context = ctx }

// Context returns the owner's lifetime context.
func (s Scope) Context() context.Context {
	if s.context == nil {
		return context.Background()
	}
	return s.context
}
