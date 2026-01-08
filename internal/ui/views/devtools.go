package views

import (
	"context"

	"github.com/kpumuk/lazykiq/internal/devtools"
)

// DevelopmentSetter allows views to receive development tracking configuration.
type DevelopmentSetter interface {
	SetDevelopment(tracker *devtools.Tracker, key string)
}

func devContext(tracker *devtools.Tracker, key, origin string) (context.Context, func()) {
	if tracker == nil || key == "" {
		return context.Background(), func() {}
	}
	measurement := devtools.NewMeasurement(key)
	measurement.Start()
	ctx := devtools.WithMeasurement(context.Background(), measurement)
	if origin != "" {
		ctx = devtools.WithOrigin(ctx, origin)
	}
	finish := func() {
		tracker.Finish(measurement)
	}
	return ctx, finish
}

func devOriginContext(tracker *devtools.Tracker, origin string) context.Context {
	if tracker == nil || origin == "" {
		return context.Background()
	}
	return devtools.WithOrigin(context.Background(), origin)
}
