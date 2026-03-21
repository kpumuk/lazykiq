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
