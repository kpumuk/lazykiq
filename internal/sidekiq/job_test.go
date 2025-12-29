package sidekiq

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestNewJobRecord_QueueExtraction(t *testing.T) {
	value := `{"queue":"default","jid":"123","class":"MyJob","args":[1,"two"],"cattr":{"foo":"bar"}}`

	record := NewJobRecord(value, "")

	if got := record.Queue(); got != "default" {
		t.Fatalf("Queue() = %q, want %q", got, "default")
	}
	if got := record.JID(); got != "123" {
		t.Fatalf("JID() = %q, want %q", got, "123")
	}
	if got := record.Klass(); got != "MyJob" {
		t.Fatalf("Klass() = %q, want %q", got, "MyJob")
	}
	if got := record.Value(); got != value {
		t.Fatalf("Value() = %q, want %q", got, value)
	}
	if args := record.Args(); len(args) != 2 {
		t.Fatalf("Args() len = %d, want %d", len(args), 2)
	}
	if ctx := record.Context(); ctx["foo"] != "bar" {
		t.Fatalf("Context()[\"foo\"] = %v, want %v", ctx["foo"], "bar")
	}
}

func TestNewJobRecord_QueueOverride(t *testing.T) {
	value := `{"queue":"default","jid":"123"}`

	record := NewJobRecord(value, "critical")

	if got := record.Queue(); got != "critical" {
		t.Fatalf("Queue() = %q, want %q", got, "critical")
	}
}

func TestNewJobRecord_InvalidJSON(t *testing.T) {
	value := "{invalid"

	record := NewJobRecord(value, "")

	if got := record.Queue(); got != "" {
		t.Fatalf("Queue() = %q, want empty", got)
	}
	if got := record.JID(); got != "" {
		t.Fatalf("JID() = %q, want empty", got)
	}
	if got := record.Item(); len(got) != 0 {
		t.Fatalf("Item() len = %d, want 0", len(got))
	}
	if args := record.Args(); len(args) != 1 || args[0] != value {
		t.Fatalf("Args() = %v, want [%q]", args, value)
	}
}

func TestJobRecord_DisplayClass(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "wrapped",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"WrappedJob"}`,
			want:  "WrappedJob",
		},
		{
			name:  "mailer",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["UserMailer","welcome","deliver_now"]}]}`,
			want:  "UserMailer#welcome",
		},
		{
			name:  "wrapped_fallback_arg",
			value: `{"class":"Sidekiq::ActiveJob::Wrapper","args":["ArgJob",1]}`,
			want:  "ArgJob",
		},
		{
			name:  "plain",
			value: `{"class":"PlainJob"}`,
			want:  "PlainJob",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayClass(); got != tt.want {
				t.Fatalf("DisplayClass() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJobRecord_DisplayArgs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []interface{}
	}{
		{
			name:  "wrapper_basic",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[1,2]}]}`,
			want:  []interface{}{float64(1), float64(2)},
		},
		{
			name:  "wrapper_global_id",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[{"_aj_globalid":"gid://app/Model/1"}]}]}`,
			want:  []interface{}{"gid://app/Model/1"},
		},
		{
			name:  "wrapper_strips_aj_keys",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[{"_aj_extras":"ignore","foo":"bar"}]}]}`,
			want:  []interface{}{map[string]interface{}{"foo": "bar"}},
		},
		{
			name:  "mailer_delivery",
			value: `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["UserMailer","welcome","deliver_now","user_id"]}]}`,
			want:  []interface{}{"user_id"},
		},
		{
			name:  "mailer_params",
			value: `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":["UserMailer","welcome","deliver_now",{"params":{"x":1},"args":[2,3]}]}]}`,
			want: []interface{}{
				map[string]interface{}{"x": float64(1)},
				[]interface{}{float64(2), float64(3)},
			},
		},
		{
			name:  "encrypted",
			value: `{"class":"PlainJob","encrypt":true,"args":[1,"secret"]}`,
			want:  []interface{}{float64(1), "[encrypted data]"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayArgs(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DisplayArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestJobRecord_ErrorFields(t *testing.T) {
	value := `{"error_class":"Err","error_message":"bad","retry_count":2,"failed_at":10.5,"retried_at":20.5}`

	record := NewJobRecord(value, "")

	if got := record.ErrorClass(); got != "Err" {
		t.Fatalf("ErrorClass() = %q, want %q", got, "Err")
	}
	if got := record.ErrorMessage(); got != "bad" {
		t.Fatalf("ErrorMessage() = %q, want %q", got, "bad")
	}
	if got := record.HasError(); !got {
		t.Fatalf("HasError() = false, want true")
	}
	if got := record.RetryCount(); got != 2 {
		t.Fatalf("RetryCount() = %d, want %d", got, 2)
	}
	if got := record.FailedAt(); got != 10.5 {
		t.Fatalf("FailedAt() = %v, want %v", got, 10.5)
	}
	if got := record.RetriedAt(); got != 20.5 {
		t.Fatalf("RetriedAt() = %v, want %v", got, 20.5)
	}
}

func TestJobRecord_Metadata(t *testing.T) {
	value := `{"bid":"BID-1","tags":["a","b"],"enqueued_at":1000,"created_at":2000}`

	record := NewJobRecord(value, "")

	if got := record.Bid(); got != "BID-1" {
		t.Fatalf("Bid() = %q, want %q", got, "BID-1")
	}
	if got := record.EnqueuedAt(); got != 1000 {
		t.Fatalf("EnqueuedAt() = %v, want %v", got, 1000)
	}
	if got := record.CreatedAt(); got != 2000 {
		t.Fatalf("CreatedAt() = %v, want %v", got, 2000)
	}
	if got := record.Tags(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("Tags() = %#v, want %#v", got, []string{"a", "b"})
	}
}

func TestJobRecord_Latency(t *testing.T) {
	now := time.Now().Unix()
	value := `{"enqueued_at":` + jsonNumber(now-10) + `}`
	record := NewJobRecord(value, "")

	latency := record.Latency()
	if latency < 8 || latency > 12 {
		t.Fatalf("Latency() = %v, want around 10", latency)
	}

	nowMs := time.Now().UnixMilli()
	valueMs := `{"created_at":` + jsonNumber(nowMs-5000) + `}`
	recordMs := NewJobRecord(valueMs, "")
	latencyMs := recordMs.Latency()
	if latencyMs < 4 || latencyMs > 7 {
		t.Fatalf("Latency() (ms) = %v, want around 5", latencyMs)
	}
}

func TestJobRecord_ErrorBacktrace(t *testing.T) {
	backtrace := []string{"line1", "line2"}
	encoded := encodeBacktrace(t, backtrace)
	value := `{"error_backtrace":"` + encoded + `"}`

	record := NewJobRecord(value, "")
	if got := record.ErrorBacktrace(); !reflect.DeepEqual(got, backtrace) {
		t.Fatalf("ErrorBacktrace() = %#v, want %#v", got, backtrace)
	}
}

func encodeBacktrace(t *testing.T, backtrace []string) string {
	t.Helper()

	raw, err := json.Marshal(backtrace)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var buf bytes.Buffer
	writer := zlib.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func jsonNumber(n int64) string {
	return fmt.Sprintf("%d", n)
}
