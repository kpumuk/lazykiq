// Package devtools provides development instrumentation for Redis usage.
package devtools

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultLogLimit = 500

type originKey struct{}

// EntryKind describes the type of a tracked entry.
type EntryKind int

const (
	// EntryCommand represents a single Redis command.
	EntryCommand EntryKind = iota
	// EntryPipelineBegin marks the start of a pipeline execution.
	EntryPipelineBegin
	// EntryPipelineExec marks the execution of a pipeline.
	EntryPipelineExec
	// EntryResult represents a result produced by the dev console.
	EntryResult
)

// Entry captures a single tracked entry.
type Entry struct {
	Kind     EntryKind
	Command  string
	Duration time.Duration
}

// LogEntry captures a single tracked log line.
type LogEntry struct {
	Seq    uint64
	Time   time.Time
	Origin string
	Entry  Entry
}

// Tracker records Redis commands for development diagnostics.
type Tracker struct {
	logLimit int
	logMu    sync.RWMutex
	log      []LogEntry
	logHead  int
	logFull  bool
	logSeq   uint64
}

// NewTracker creates a new development tracker.
func NewTracker() *Tracker {
	return &Tracker{
		logLimit: defaultLogLimit,
	}
}

// WithOrigin returns a context carrying the origin label.
func WithOrigin(ctx context.Context, origin string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, originKey{}, origin)
}

// WithTracker returns a context carrying the origin label.
func WithTracker(ctx context.Context, origin string) context.Context {
	if origin == "" {
		return ctx
	}
	return WithOrigin(ctx, origin)
}

// OriginFromContext extracts the origin label from context.
func OriginFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value := ctx.Value(originKey{}); value != nil {
		if origin, ok := value.(string); ok {
			return origin
		}
	}
	return ""
}

// LogEntries returns the most recent Redis log entries in chronological order.
func (t *Tracker) LogEntries() []LogEntry {
	t.logMu.RLock()
	defer t.logMu.RUnlock()
	if len(t.log) == 0 {
		return nil
	}
	if !t.logFull {
		return append([]LogEntry(nil), t.log...)
	}
	result := make([]LogEntry, 0, len(t.log))
	result = append(result, t.log[t.logHead:]...)
	result = append(result, t.log[:t.logHead]...)
	return result
}

// AppendLog appends a log entry to the ring buffer.
func (t *Tracker) AppendLog(entry LogEntry) {
	if t == nil || t.logLimit == 0 {
		return
	}

	t.logMu.Lock()
	entry.Seq = t.logSeq
	t.logSeq++
	if len(t.log) < t.logLimit {
		t.log = append(t.log, entry)
		if len(t.log) == t.logLimit {
			t.logHead = 0
			t.logFull = true
		}
		t.logMu.Unlock()
		return
	}
	t.log[t.logHead] = entry
	t.logHead = (t.logHead + 1) % t.logLimit
	t.logFull = true
	t.logMu.Unlock()
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
		start := time.Now()
		err := next(ctx, cmd)
		h.record(ctx, cmd, time.Since(start))
		return err
	}
}

func (h hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if len(cmds) == 0 {
			return next(ctx, cmds)
		}
		h.recordPipelineMarker(ctx, EntryPipelineBegin, 0)
		start := time.Now()
		err := next(ctx, cmds)
		for _, cmd := range cmds {
			h.record(ctx, cmd, 0)
		}
		h.recordPipelineMarker(ctx, EntryPipelineExec, time.Since(start))
		return err
	}
}

func (h hook) record(ctx context.Context, cmd redis.Cmder, duration time.Duration) {
	if h.tracker == nil {
		return
	}

	commandText := formatCommand(cmd)
	h.tracker.appendLogEntry(ctx, Entry{
		Kind:     EntryCommand,
		Command:  commandText,
		Duration: duration,
	})
}

func (h hook) recordPipelineMarker(ctx context.Context, kind EntryKind, duration time.Duration) {
	if h.tracker == nil {
		return
	}
	h.tracker.appendLogEntry(ctx, Entry{
		Kind:     kind,
		Command:  "",
		Duration: duration,
	})
}

func (t *Tracker) appendLogEntry(ctx context.Context, entry Entry) {
	if t == nil {
		return
	}
	origin := OriginFromContext(ctx)
	if origin == "" {
		origin = originFromCallers()
	}
	if origin == "" {
		origin = "unknown"
	}
	t.AppendLog(LogEntry{
		Time:   time.Now(),
		Origin: origin,
		Entry:  entry,
	})
}

func originFromCallers() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(4, pcs)
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var sidekiqFallback string
	for {
		frame, more := frames.Next()
		fn := frame.Function
		if fn == "" {
			if !more {
				break
			}
			continue
		}
		if strings.Contains(fn, "/internal/ui/") || strings.Contains(fn, "/internal/ui.") {
			return shortFuncName(fn)
		}
		if sidekiqFallback == "" && (strings.Contains(fn, "/internal/sidekiq/") || strings.Contains(fn, "/internal/sidekiq.")) {
			sidekiqFallback = shortFuncName(fn)
		}
		if !more {
			break
		}
	}
	return sidekiqFallback
}

func shortFuncName(fn string) string {
	if idx := strings.LastIndex(fn, "/"); idx >= 0 {
		fn = fn[idx+1:]
	}
	fn = strings.TrimSuffix(fn, ".func1")
	fn = strings.ReplaceAll(fn, "(*", "")
	fn = strings.ReplaceAll(fn, ")", "")
	return fn
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
