// Package devtools provides development instrumentation for Redis usage.
package devtools

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultCommandLimit = 200

type measurementKey struct{}

// EntryKind describes the type of a tracked entry.
type EntryKind int

const (
	// EntryCommand represents a single Redis command.
	EntryCommand EntryKind = iota
	// EntryPipelineBegin marks the start of a pipeline execution.
	EntryPipelineBegin
	// EntryPipelineExec marks the execution of a pipeline.
	EntryPipelineExec
)

// Entry captures a single tracked entry.
type Entry struct {
	Kind    EntryKind
	Command string
}

// Sample captures a completed refresh measurement.
type Sample struct {
	View      string
	Count     int
	Duration  time.Duration
	Entries   []Entry
	UpdatedAt time.Time
}

// Measurement tracks Redis activity for a single refresh.
type Measurement struct {
	view    string
	start   time.Time
	count   int
	stored  int
	entries []Entry
	mu      sync.Mutex
}

// Tracker stores refresh measurements and records Redis commands.
type Tracker struct {
	commandLimit int
	mu           sync.RWMutex
	samples      map[string]Sample
}

// NewTracker creates a new development tracker.
func NewTracker() *Tracker {
	return &Tracker{
		commandLimit: defaultCommandLimit,
		samples:      make(map[string]Sample),
	}
}

// NewMeasurement creates a measurement container for a view.
func NewMeasurement(view string) *Measurement {
	return &Measurement{view: view}
}

// Start marks the measurement start time.
func (m *Measurement) Start() {
	m.mu.Lock()
	m.start = time.Now()
	m.mu.Unlock()
}

// WithMeasurement returns a context carrying the measurement.
func WithMeasurement(ctx context.Context, m *Measurement) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, measurementKey{}, m)
}

// MeasurementFromContext extracts the measurement from context.
func MeasurementFromContext(ctx context.Context) *Measurement {
	if ctx == nil {
		return nil
	}
	if value := ctx.Value(measurementKey{}); value != nil {
		if measurement, ok := value.(*Measurement); ok {
			return measurement
		}
	}
	return nil
}

// Finish finalizes a measurement and stores it.
func (t *Tracker) Finish(m *Measurement) Sample {
	if m == nil {
		return Sample{}
	}

	now := time.Now()
	m.mu.Lock()
	start := m.start
	count := m.count
	entries := append([]Entry(nil), m.entries...)
	m.mu.Unlock()

	if start.IsZero() {
		start = now
	}

	sample := Sample{
		View:      m.view,
		Count:     count,
		Duration:  now.Sub(start),
		Entries:   entries,
		UpdatedAt: now,
	}

	t.mu.Lock()
	t.samples[m.view] = sample
	t.mu.Unlock()

	return sample
}

// Sample returns the last sample for a view.
func (t *Tracker) Sample(view string) (Sample, bool) {
	t.mu.RLock()
	sample, ok := t.samples[view]
	t.mu.RUnlock()
	return sample, ok
}

// Entries returns the last recorded entries for a view.
func (t *Tracker) Entries(view string) []Entry {
	sample, ok := t.Sample(view)
	if !ok {
		return nil
	}
	return append([]Entry(nil), sample.Entries...)
}

// Hook returns a Redis hook for tracking commands.
func (t *Tracker) Hook() redis.Hook {
	return hook{tracker: t}
}

// FormatDuration renders a compact duration string.
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < 10*time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

type hook struct {
	tracker *Tracker
}

func (h hook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.record(ctx, cmd)
		return next(ctx, cmd)
	}
}

func (h hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if len(cmds) == 0 {
			return next(ctx, cmds)
		}
		h.recordPipelineMarker(ctx, EntryPipelineBegin)
		for _, cmd := range cmds {
			h.record(ctx, cmd)
		}
		h.recordPipelineMarker(ctx, EntryPipelineExec)
		return next(ctx, cmds)
	}
}

func (h hook) record(ctx context.Context, cmd redis.Cmder) {
	if h.tracker == nil {
		return
	}
	measurement := MeasurementFromContext(ctx)
	if measurement == nil {
		return
	}

	commandText := formatCommand(cmd)

	measurement.mu.Lock()
	measurement.count++
	if h.tracker.commandLimit <= 0 || measurement.stored < h.tracker.commandLimit {
		measurement.entries = append(measurement.entries, Entry{
			Kind:    EntryCommand,
			Command: commandText,
		})
		measurement.stored++
	}
	measurement.mu.Unlock()
}

func (h hook) recordPipelineMarker(ctx context.Context, kind EntryKind) {
	if h.tracker == nil {
		return
	}
	measurement := MeasurementFromContext(ctx)
	if measurement == nil {
		return
	}
	measurement.mu.Lock()
	measurement.entries = append(measurement.entries, Entry{
		Kind:    kind,
		Command: "",
	})
	measurement.mu.Unlock()
}

func formatCommand(cmd redis.Cmder) string {
	args := cmd.Args()
	if len(args) == 0 {
		return cmd.Name()
	}
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprint(arg)
	}
	return strings.Join(parts, " ")
}
