package views

import (
	"context"

	"github.com/kpumuk/lazykiq/internal/devtools"
)

// DevelopmentSetter allows views to receive development tracking configuration.
type DevelopmentSetter interface {
	SetDevelopment(tracker *devtools.Tracker, key string)
}

func devContext(tracker *devtools.Tracker, key string) (context.Context, func()) {
	if tracker == nil || key == "" {
		return context.Background(), func() {}
	}
	measurement := devtools.NewMeasurement(key)
	measurement.Start()
	ctx := devtools.WithMeasurement(context.Background(), measurement)
	finish := func() {
		tracker.Finish(measurement)
	}
	return ctx, finish
}
