package sidekiq

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExplicitCapabilities(t *testing.T) {
	for name, names := range map[string][]string{"none": nil, "pro": {"pro"}, "ent": {"ent"}, "combined": {"pro", "ent", "pro"}} {
		t.Run(name, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			_, _ = mr.ZAdd("batches", 1, "bid")
			_, _ = mr.SetAdd("paused", "default")
			_ = mr.Set("periodic-version", "version")
			features, err := ResolveFeatures(names)
			if err != nil {
				t.Fatal(err)
			}
			client.capabilities = features
			got := client.GetCapabilities()
			if got.SupportsQueuePause() != (len(names) > 0) {
				t.Fatalf("capabilities = %+v, %v", got, err)
			}
			err = client.NewQueue("default").SetPaused(testContext(t), true)
			if len(names) == 0 && !errors.Is(err, ErrQueuePauseUnsupported) {
				t.Fatalf("SetPaused = %v", err)
			}
			if len(names) > 0 && err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Fixtures generated with Ruby's Marshal.dump(Sidekiq::Pro::Message.new(...)).
// Keep them independent of the Go encoder and free of a Ruby test dependency.
func TestQueueConfigMessageRubyFixtures(t *testing.T) {
	prefix := "0408533a1a536964656b69713a3a50726f3a3a4d657373616765083a09766572623a"
	payload := "3a0c7061796c6f61644922"
	ending := "063a0645543a07646269"
	for name, test := range map[string]struct {
		verb, queue string
		db          int
		hex         string
	}{
		"pause":             {"pause", "default", 0, prefix + "0a7061757365" + payload + "0c64656661756c74" + ending + "00"},
		"resume nonzero db": {"unpause", "default", 9, prefix + "0c756e7061757365" + payload + "0c64656661756c74" + ending + "0e"},
		"utf8":              {"pause", "メール", 123, prefix + "0a7061757365" + payload + "0ee383a1e383bce383ab" + ending + "017b"},
		"compact boundary":  {"pause", strings.Repeat("x", 122), 122, prefix + "0a7061757365" + payload + "7f" + strings.Repeat("78", 122) + ending + "7f"},
		"long":              {"pause", strings.Repeat("x", 123), 256, prefix + "0a7061757365" + payload + "017b" + strings.Repeat("78", 123) + ending + "020001"},
		"longer":            {"unpause", strings.Repeat("x", 256), 65536, prefix + "0c756e7061757365" + payload + "020001" + strings.Repeat("78", 256) + ending + "03000001"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := hex.EncodeToString(queueConfigMessage(test.verb, test.queue, test.db)); got != test.hex {
				t.Fatalf("Marshal = %s; want %s", got, test.hex)
			}
		})
	}
}

func TestQueueSetPausedNotifiesIdempotently(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)
	client.capabilities, _ = ResolveFeatures([]string{"pro"})
	pubsub := client.Redis().Subscribe(ctx, "sidekiq:config")
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	queue := client.NewQueue("メール")
	for _, paused := range []bool{true, true, false, false} {
		if err := queue.SetPaused(ctx, paused); err != nil {
			t.Fatal(err)
		}
		got, err := queue.Paused(ctx)
		if err != nil || got != paused {
			t.Fatalf("Paused = %v, %v; want %v", got, err, paused)
		}
		message, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		verb := "unpause"
		if paused {
			verb = "pause"
		}
		if message.Payload != string(queueConfigMessage(verb, queue.Name(), 0)) {
			t.Fatalf("unexpected notification: %x", message.Payload)
		}
	}
}

func TestQueueSetPausedRejectsUnsupportedAndWrongType(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)
	queue := client.NewQueue("default")
	if err := queue.SetPaused(ctx, true); !errors.Is(err, ErrQueuePauseUnsupported) {
		t.Fatalf("SetPaused = %v", err)
	}
	if mr.Exists("paused") {
		t.Fatal("unsupported mutation wrote pause state")
	}
	client.capabilities, _ = ResolveFeatures([]string{"pro"})
	_ = mr.Set("paused", "wrong-type")
	pubsub := client.Redis().Subscribe(ctx, "sidekiq:config")
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	messages := pubsub.Channel()
	if err := queue.SetPaused(ctx, true); err == nil {
		t.Fatal("wrong type mutation succeeded")
	}
	select {
	case msg := <-messages:
		t.Fatalf("failed mutation published %x", msg.Payload)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestFeaturesAreClientLocal(t *testing.T) {
	features, err := ResolveFeatures([]string{"ent"})
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := NewClient("", WithFeatures(features))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enabled.Close() }()
	disabled, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = disabled.Close() }()
	features = Capabilities{}
	if !enabled.GetCapabilities().SupportsQueuePause() || disabled.GetCapabilities().SupportsQueuePause() || features.SupportsQueuePause() {
		t.Fatal("feature configuration leaked across clients or was not copied")
	}
}
