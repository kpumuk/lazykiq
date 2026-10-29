// Package devtools provides development instrumentation for Redis usage.
package devtools

import (
	"cmp"
	"context"
	"fmt"
	"math/big"
	"net"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

const (
	defaultLogLimit = 500
	// DisplayByteLimit includes the truncation marker. Execution data is unaffected.
	DisplayByteLimit  = 4096
	originByteLimit   = 256
	truncationMarker  = " [truncated]"
	displayDepthLimit = 16
	displayMapLimit   = 64
)

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
	// Own even short substrings so they cannot retain a large source allocation.
	entry.Entry.Command = boundedText(entry.Entry.Command, DisplayByteLimit)
	entry.Origin = boundedText(entry.Origin, originByteLimit)

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
	if _, name, ok := strings.CutLast(fn, "/"); ok {
		fn = name
	}
	fn = strings.TrimSuffix(fn, ".func1")
	fn = strings.ReplaceAll(fn, "(*", "")
	fn = strings.ReplaceAll(fn, ")", "")
	return fn
}

func formatCommand(cmd redis.Cmder) string {
	args := cmd.Args()
	if len(args) == 0 {
		return boundedText(cmd.Name(), DisplayByteLimit)
	}
	w := displayWriter{limit: DisplayByteLimit}
	for i, arg := range args {
		if i > 0 {
			w.text(" ")
		}
		w.value(reflect.ValueOf(arg), 0)
		if w.truncated {
			break
		}
	}
	return w.result()
}

// FormatResult formats a bounded, single-line Redis console preview.
// It never invokes user-defined formatters or changes the result itself.
func FormatResult(result any) string {
	w := displayWriter{limit: DisplayByteLimit, normalize: true}
	switch value := result.(type) {
	case nil:
		w.text("(nil)")
	case []byte:
		w.textBytes(value)
	case []string:
		for i, item := range value {
			if i > 0 {
				w.text(" ")
			}
			w.text(item)
			if w.truncated {
				break
			}
		}
	case []any:
		for i, item := range value {
			if i > 0 {
				w.text(" ")
			}
			w.value(reflect.ValueOf(item), 0)
			if w.truncated {
				break
			}
		}
	default:
		w.value(reflect.ValueOf(result), 0)
	}
	return w.result()
}

// FormatError formats an error without copying its full message into a preview.
func FormatError(err error) string {
	w := displayWriter{limit: DisplayByteLimit, normalize: true}
	w.text("error: ")
	w.text(err.Error())
	return w.result()
}

func boundedText(s string, limit int) string {
	if len(s) <= limit && utf8.ValidString(s) {
		return strings.Clone(s)
	}
	w := displayWriter{limit: limit}
	w.text(s)
	return w.result()
}

type displayWriter struct {
	buf       strings.Builder
	limit     int
	normalize bool
	truncated bool
}

func (w *displayWriter) write(s string) {
	if w.truncated {
		return
	}
	if len(s) > w.limit-w.buf.Len() {
		w.truncated = true
		return
	}
	w.buf.WriteString(s)
}

func (w *displayWriter) text(s string) {
	if w.truncated {
		return
	}
	if !w.normalize {
		n := min(len(s), w.limit-w.buf.Len())
		for cut := 0; cut < utf8.UTFMax-1 && n > 0 && n < len(s) && !utf8.RuneStart(s[n]); cut++ {
			n--
		}
		if utf8.ValidString(s[:n]) {
			w.write(s[:n])
			w.truncated = n < len(s)
			return
		}
	}
	start := 0
	for i := 0; i < len(s) && !w.truncated; {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		if i-start+size > w.limit-w.buf.Len() {
			w.write(s[start:i])
			w.truncated = true
			return
		}
		replacement := ""
		switch {
		case r == utf8.RuneError && size == 1:
			const hex = "0123456789abcdef"
			escaped := [4]byte{'\\', 'x', hex[s[i]>>4], hex[s[i]&15]}
			replacement = string(escaped[:])
		case w.normalize && r == '\r':
			if i+size < len(s) && s[i+size] == '\n' {
				size++
			}
			replacement = "\\n"
		case w.normalize && r == '\n':
			replacement = "\\n"
		case w.normalize && r == '\t':
			replacement = "\\t"
		}
		if replacement != "" {
			w.write(s[start:i])
			w.write(replacement)
			start = i + size
		}
		i += size
	}
	if !w.truncated {
		w.write(s[start:])
	}
}

func (w *displayWriter) textBytes(b []byte) {
	// Copy only a bounded prefix before converting binary data to text.
	n := min(len(b), w.limit+utf8.UTFMax)
	w.text(string(b[:n]))
	if n < len(b) {
		w.truncated = true
	}
}

func (w *displayWriter) result() string {
	s := w.buf.String()
	if !w.truncated {
		return s
	}
	n := min(len(s), w.limit-len(truncationMarker))
	for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + truncationMarker
}

func (w *displayWriter) value(v reflect.Value, depth int) {
	if w.truncated {
		return
	}
	if depth >= displayDepthLimit {
		w.truncated = true
		return
	}
	if !v.IsValid() {
		w.text("<nil>")
		return
	}
	if v.CanInterface() && w.specialValue(v) {
		return
	}
	var number [64]byte
	switch v.Kind() {
	case reflect.Interface:
		w.value(v.Elem(), depth+1)
	case reflect.String:
		w.text(v.String())
	case reflect.Bool:
		w.text(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.text(string(strconv.AppendInt(number[:0], v.Int(), 10)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.text(string(strconv.AppendUint(number[:0], v.Uint(), 10)))
	case reflect.Float32, reflect.Float64:
		w.text(strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()))
	case reflect.Complex64, reflect.Complex128:
		w.text(strconv.FormatComplex(v.Complex(), 'g', -1, v.Type().Bits()))
	case reflect.Slice, reflect.Array:
		w.text("[")
		for i := 0; i < v.Len() && !w.truncated; i++ {
			if i > 0 {
				w.text(" ")
			}
			w.value(v.Index(i), depth+1)
		}
		w.text("]")
	case reflect.Map:
		w.mapValue(v, depth)
	case reflect.Pointer:
		if v.IsNil() {
			w.text("<nil>")
			return
		}
		w.text("<pointer>")
		w.truncated = true
	case reflect.Invalid, reflect.Chan, reflect.Func, reflect.Struct, reflect.UnsafePointer:
		w.text("<" + v.Kind().String() + ">")
		w.truncated = true
	}
}

func (w *displayWriter) specialValue(v reflect.Value) bool {
	if d, ok := reflect.TypeAssert[time.Duration](v); ok {
		w.text(d.String())
		return true
	}
	if n, ok := reflect.TypeAssert[*big.Int](v); ok && n != nil {
		if n.BitLen() > DisplayByteLimit {
			w.text("<big.Int>")
			w.truncated = true
		} else {
			w.text(n.String())
		}
		return true
	}
	return false
}

func (w *displayWriter) mapValue(v reflect.Value, depth int) {
	w.text("map[")
	// MapRange copies concrete keys/values. Check their size before iteration.
	if v.Len() > displayMapLimit || v.Type().Key().Size() > DisplayByteLimit || v.Type().Elem().Size() > DisplayByteLimit {
		w.truncated = true
		return
	}
	type pair struct {
		key    string
		rawKey reflect.Value
		value  reflect.Value
	}
	pairs := make([]pair, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		key := displayWriter{limit: originByteLimit, normalize: w.normalize}
		rawKey := iter.Key()
		key.value(rawKey, depth+1)
		pairs = append(pairs, pair{key.result(), rawKey, iter.Value()})
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		left, right := a.rawKey, b.rawKey
		if left.Kind() == reflect.Interface {
			left = left.Elem()
		}
		if right.Kind() == reflect.Interface {
			right = right.Elem()
		}
		if kind := left.Kind(); kind == right.Kind() {
			if kind >= reflect.Int && kind <= reflect.Int64 {
				return cmp.Compare(left.Int(), right.Int())
			}
			if kind >= reflect.Uint && kind <= reflect.Uintptr {
				return cmp.Compare(left.Uint(), right.Uint())
			}
			if kind == reflect.Float32 || kind == reflect.Float64 {
				return cmp.Compare(left.Float(), right.Float())
			}
		}
		return strings.Compare(a.key, b.key)
	})
	for i, p := range pairs {
		if i > 0 {
			w.text(" ")
		}
		w.text(p.key)
		w.text(":")
		w.value(p.value, depth+1)
		if w.truncated {
			break
		}
	}
	w.text("]")
}
