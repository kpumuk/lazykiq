package devtools

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type forbiddenFormatter struct{}

func (forbiddenFormatter) String() string         { panic("unbounded String called") }
func (forbiddenFormatter) Format(fmt.State, rune) { panic("unbounded Format called") }

type namedText string

func TestDisplayFormatting(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		value any
		want  string
	}{
		"nil":          {nil, "(nil)"},
		"text":         {"a\r\nb\nc\rd\te", `a\nb\nc\nd\te`},
		"binary text":  {[]byte{'a', 0xff, '\n'}, `a\xff\n`},
		"strings":      {[]string{"a", "b\nc"}, `a b\nc`},
		"nested":       {[]any{nil, []any{"x", int64(2), []byte{3, 4}}, true}, "<nil> [x 2 [3 4]] true"},
		"named scalar": {namedText("ERR test"), "ERR test"},
		"map":          {map[any]any{"b": []any{1, 2}, "a": "text"}, "map[a:text b:[1 2]]"},
		"numeric map":  {map[any]any{10: "b", 2: "a"}, "map[2:a 10:b]"},
		"big integer":  {big.NewInt(1234), "1234"},
		"duration":     {time.Second, "1s"},
		"formatter":    {forbiddenFormatter{}, "<struct>" + truncationMarker},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := FormatResult(tc.value); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	cmd := redis.NewCmd(context.Background(), "SET", "key", "a\nb", []byte{1, 2}, int64(3))
	if got := formatCommand(cmd); got != "SET key a\nb [1 2] 3" {
		t.Fatal(got)
	}
}

func TestDisplayBudgets(t *testing.T) {
	t.Parallel()
	cycle := make([]any, 1)
	cycle[0] = cycle
	oversizedMap := make(map[int]any, 65)
	for i := range 65 {
		oversizedMap[i] = "x"
	}
	tests := map[string]any{
		"exact":                strings.Repeat("x", DisplayByteLimit),
		"overflow":             strings.Repeat("x", DisplayByteLimit+1),
		"multibyte":            strings.Repeat("界", DisplayByteLimit),
		"normalization":        strings.Repeat("\r\n\t", DisplayByteLimit),
		"binary":               []byte(strings.Repeat("\xff", DisplayByteLimit)),
		"wide slice":           make([]string, 100000),
		"nested":               []any{[]any{strings.Repeat("x", 1<<20)}},
		"cycle":                cycle,
		"oversized map":        oversizedMap,
		"wide map key":         map[string]any{strings.Repeat("界", 1<<20): "value"},
		"huge big integer":     new(big.Int).Lsh(big.NewInt(1), 1<<20),
		"concrete map value":   map[string][1 << 20]byte{"key": {}},
		"concrete map key":     map[[1 << 20]byte]string{{}: "value"},
		"concrete slice value": []struct{ Padding [1 << 20]byte }{{}},
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := FormatResult(value)
			if len(got) > DisplayByteLimit || !utf8.ValidString(got) {
				t.Fatalf("invalid preview: bytes=%d valid=%v", len(got), utf8.ValidString(got))
			}
			if name == "exact" {
				if got != value {
					t.Fatal("exact budget should not truncate")
				}
			} else if !strings.Contains(got, truncationMarker) {
				t.Fatal("missing truncation marker")
			}
		})
	}
	for _, value := range []any{strings.Repeat("界", 1<<20), []byte(strings.Repeat("x", 1<<20)), make([]string, 100000)} {
		cmd := redis.NewCmd(context.Background(), "LPUSH", "queue:default", value)
		got := formatCommand(cmd)
		if len(got) > DisplayByteLimit || !utf8.ValidString(got) || !strings.HasPrefix(got, "LPUSH queue:default ") || !strings.HasSuffix(got, truncationMarker) {
			t.Fatalf("bad command preview: bytes=%d", len(got))
		}
		if !reflect.DeepEqual(cmd.Args()[2], value) {
			t.Fatal("formatting changed execution arguments")
		}
	}
}

func TestTrackerRingOwnership(t *testing.T) {
	t.Parallel()
	tracker := NewTracker()
	source := strings.Repeat("x", 1<<20)
	small := source[:8]
	stamp := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	tracker.AppendLog(LogEntry{Time: stamp, Origin: small, Entry: Entry{Command: small, Duration: time.Second}})
	first := tracker.LogEntries()[0]
	if unsafe.StringData(first.Origin) == unsafe.StringData(small) || unsafe.StringData(first.Entry.Command) == unsafe.StringData(small) {
		t.Fatal("log retains borrowed backing string")
	}
	runtime.KeepAlive(source)
	wide := strings.Repeat("x", DisplayByteLimit+1)
	for i := range 600 {
		tracker.AppendLog(LogEntry{Time: stamp, Origin: wide, Entry: Entry{Kind: EntryResult, Command: fmt.Sprintf("%06d%s", i, wide), Duration: time.Duration(i)}})
	}
	entries := tracker.LogEntries()
	if len(entries) != defaultLogLimit {
		t.Fatal(len(entries))
	}
	for i, entry := range entries {
		want := i + 101
		if entry.Seq != uint64(want) || entry.Entry.Duration != time.Duration(want-1) || entry.Time != stamp || entry.Entry.Kind != EntryResult {
			t.Fatalf("entry %d lost order or metadata", i)
		}
		if !strings.HasPrefix(entry.Entry.Command, fmt.Sprintf("%06d", want-1)) || len(entry.Entry.Command) > DisplayByteLimit || len(entry.Origin) > originByteLimit {
			t.Fatalf("entry %d exceeded budget or lost identity", i)
		}
	}
	// Snapshots remain independent as the ring overwrites every old entry.
	entries[0].Entry.Command = "snapshot only"
	for range defaultLogLimit {
		tracker.AppendLog(LogEntry{Entry: Entry{Command: "short"}})
	}
	for _, entry := range tracker.LogEntries() {
		if entry.Entry.Command != "short" || entry.Origin != "" {
			t.Fatal("overwritten data retained in log")
		}
	}
	if entries[1].Seq != 102 || !strings.HasSuffix(entries[1].Entry.Command, truncationMarker) {
		t.Fatal("snapshot changed after overwrite")
	}
}

func TestTrackerConcurrent(t *testing.T) {
	t.Parallel()
	tracker := NewTracker()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				tracker.AppendLog(LogEntry{Entry: Entry{Command: "ping"}})
				entries := tracker.LogEntries()
				for i := 1; i < len(entries); i++ {
					if entries[i].Seq != entries[i-1].Seq+1 {
						t.Error("snapshot out of order")
						return
					}
				}
			}
		})
	}
	wg.Wait()
	entries := tracker.LogEntries()
	if len(entries) != defaultLogLimit || entries[len(entries)-1].Seq != 1599 {
		t.Fatal("lost concurrent entries")
	}
}

func TestTrackerHooksPreserveExecution(t *testing.T) {
	t.Parallel()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	tracker := NewTracker()
	client.AddHook(tracker.Hook())
	ctx := WithOrigin(context.Background(), "test")
	payload := strings.Repeat("界\x00", 65536)
	if err := client.Set(ctx, "full", payload, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if got, err := client.Get(ctx, "full").Result(); err != nil || got != payload {
		t.Fatal("execution truncated full result", err)
	}
	pipe := client.Pipeline()
	a := pipe.Get(ctx, "full")
	b := pipe.Get(ctx, "missing")
	if _, err := pipe.Exec(ctx); !errors.Is(err, redis.Nil) {
		t.Fatal(err)
	}
	if a.Val() != payload || !errors.Is(b.Err(), redis.Nil) {
		t.Fatal("pipeline results changed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := client.Set(canceled, "canceled", payload, 0).Err(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if server.Exists("canceled") {
		t.Fatal("canceled command executed")
	}
	entries := tracker.LogEntries()
	var commands, begin, exec int
	for _, e := range entries {
		if len(e.Entry.Command) > DisplayByteLimit {
			t.Fatal("unbounded command")
		}
		switch e.Entry.Kind {
		case EntryCommand:
			commands++
		case EntryPipelineBegin:
			begin++
		case EntryPipelineExec:
			exec++
		case EntryResult:
			t.Fatal("hook logged a console result")
		}
	}
	if commands < 5 || begin < 1 || begin != exec {
		t.Fatalf("commands=%d pipeline markers=%d/%d", commands, begin, exec)
	}
}

func TestRawBinaryPreview(t *testing.T) {
	t.Parallel()
	for _, width := range []int{16, 65536} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()
			payload := "foo" + strings.Repeat("\x80", width)
			cmd := redis.NewCmd(context.Background(), "SET", "key", payload)
			if got := formatCommand(cmd); !strings.HasPrefix(got, `SET key foo\x80\x80`) || !utf8.ValidString(got) || len(got) > DisplayByteLimit {
				t.Fatal("binary command prefix lost")
			}
			tracker := NewTracker()
			tracker.AppendLog(LogEntry{Origin: payload, Entry: Entry{Command: payload}})
			entry := tracker.LogEntries()[0]
			for _, got := range []string{entry.Origin, entry.Entry.Command} {
				if !strings.HasPrefix(got, `foo\x80\x80`) || !utf8.ValidString(got) {
					t.Fatal("binary log prefix lost")
				}
			}
		})
	}
}

func TestTrackerPipelineOrder(t *testing.T) {
	t.Parallel()
	tracker := NewTracker()
	ctx := WithOrigin(context.Background(), "pipeline-test")
	cmds := []redis.Cmder{redis.NewCmd(ctx, "GET", "one"), redis.NewCmd(ctx, "GET", "two")}
	failed := errors.New("pipeline failed")
	process := tracker.Hook().ProcessPipelineHook(func(_ context.Context, actual []redis.Cmder) error {
		if !reflect.DeepEqual(actual, cmds) {
			t.Error("execution commands changed")
		}
		return failed
	})
	if err := process(ctx, cmds); !errors.Is(err, failed) {
		t.Fatal("pipeline error changed")
	}
	entries := tracker.LogEntries()
	if len(entries) != 4 {
		t.Fatal(len(entries))
	}
	kinds := []EntryKind{EntryPipelineBegin, EntryCommand, EntryCommand, EntryPipelineExec}
	commands := []string{"", "GET one", "GET two", ""}
	for i, entry := range entries {
		if entry.Seq != uint64(i) || entry.Origin != "pipeline-test" || entry.Entry.Kind != kinds[i] || entry.Entry.Command != commands[i] {
			t.Fatalf("entry %d changed pipeline order/identity", i)
		}
	}
}

func TestMixedMapPreviewOrder(t *testing.T) {
	t.Parallel()
	values := map[any]any{2: "a", 10: "b", "15": "c"}
	for range 100 {
		if got := FormatResult(values); got != "map[2:a 10:b 15:c]" {
			t.Fatalf("inconsistent mixed-key ordering: %q", got)
		}
	}
}
