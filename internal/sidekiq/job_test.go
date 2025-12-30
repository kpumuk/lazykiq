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
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayClass(); got != tt.want {
				t.Fatalf("DisplayClass() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJobRecord_DisplayClass_SerializedActiveJobs(t *testing.T) {
	for _, tt := range serializedActiveJobTests() {
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayClass(); got != tt.wantClass {
				t.Fatalf("DisplayClass() = %q, want %q", got, tt.wantClass)
			}
		})
	}
}

func TestJobRecord_DisplayArgs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []any
	}{
		{
			name:  "wrapper_basic",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[1,2]}]}`,
			want:  []any{float64(1), float64(2)},
		},
		{
			name:  "wrapper_global_id",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[{"_aj_globalid":"gid://app/Model/1"}]}]}`,
			want:  []any{"gid://app/Model/1"},
		},
		{
			name:  "wrapper_strips_aj_keys",
			value: `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyJob","args":[{"arguments":[{"_aj_extras":"ignore","foo":"bar"}]}]}`,
			want:  []any{map[string]any{"foo": "bar"}},
		},
		{
			name:  "mailer_delivery",
			value: `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["UserMailer","welcome","deliver_now","user_id"]}]}`,
			want:  []any{"user_id"},
		},
		{
			name:  "mailer_params",
			value: `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":["UserMailer","welcome","deliver_now",{"params":{"x":1},"args":[2,3]}]}]}`,
			want: []any{
				map[string]any{"x": float64(1)},
				[]any{float64(2), float64(3)},
			},
		},
		{
			name:  "encrypted",
			value: `{"class":"PlainJob","encrypt":true,"args":[1,"secret"]}`,
			want:  []any{float64(1), "[encrypted data]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayArgs(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DisplayArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestJobRecord_DisplayArgs_SerializedActiveJobs(t *testing.T) {
	for _, tt := range serializedActiveJobTests() {
		t.Run(tt.name, func(t *testing.T) {
			record := NewJobRecord(tt.value, "")
			if got := record.DisplayArgs(); !reflect.DeepEqual(got, tt.wantArgs) {
				t.Fatalf("DisplayArgs() = %#v, want %#v", got, tt.wantArgs)
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

type serializedActiveJobTest struct {
	name      string
	value     string
	wantClass string
	wantArgs  []any
}

func serializedActiveJobTests() []serializedActiveJobTest {
	return []serializedActiveJobTest{
		{
			name:      "5x_active_job",
			value:     `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"ApiAjJob","queue":"default","args":[{"job_class":"ApiAjJob","job_id":"f1bde53f-3852-4ae4-a879-c12eacebbbb0","provider_job_id":null,"queue_name":"default","priority":null,"arguments":[1,2,3],"executions":0,"locale":"en"}],"retry":true,"jid":"099eee72911085a511d0e312","created_at":1568305542.339916,"enqueued_at":1568305542.339947}`,
			wantClass: "ApiAjJob",
			wantArgs:  []any{float64(1), float64(2), float64(3)},
		},
		{
			name:      "5x_mailer_delivery",
			value:     `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"ActionMailer::DeliveryJob","queue":"mailers","args":[{"job_class":"ActionMailer::DeliveryJob","job_id":"19cc0115-3d1c-4bbe-a51e-bfa1385895d1","provider_job_id":null,"queue_name":"mailers","priority":null,"arguments":["ApiMailer","test_email","deliver_now",1,2,3],"executions":0,"locale":"en"}],"retry":true,"jid":"37436e5504936400e8cf98db","created_at":1568305542.370133,"enqueued_at":1568305542.370241}`,
			wantClass: "ApiMailer#test_email",
			wantArgs:  []any{float64(1), float64(2), float64(3)},
		},
		{
			name:      "6x_active_job",
			value:     `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"ApiAjJob","queue":"default","args":[{"job_class":"ApiAjJob","job_id":"ff2b48d4-bdce-4825-af6b-ef8c11ab651e","provider_job_id":null,"queue_name":"default","priority":null,"arguments":[1,2,3],"executions":0,"exception_executions":{},"locale":"en","timezone":"UTC","enqueued_at":"2019-09-12T16:28:37Z"}],"retry":true,"jid":"ce121bf77b37ae81fe61b6dc","created_at":1568305717.9469702,"enqueued_at":1568305717.947005}`,
			wantClass: "ApiAjJob",
			wantArgs:  []any{float64(1), float64(2), float64(3)},
		},
		{
			name:      "6x_mailer_delivery",
			value:     `{"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"ActionMailer::MailDeliveryJob","queue":"mailers","args":[{"job_class":"ActionMailer::MailDeliveryJob","job_id":"2f967da1-a389-479c-9a4e-5cc059e6d65c","provider_job_id":null,"queue_name":"mailers","priority":null,"arguments":["ApiMailer","test_email","deliver_now",{"params":{"user":{"_aj_globalid":"gid://app/User/1"}, "_aj_symbol_keys":["user"]},"args":[1,2,3],"_aj_symbol_keys":["params", "args"]}],"executions":0,"exception_executions":{},"locale":"en","timezone":"UTC","enqueued_at":"2019-09-12T16:28:37Z"}],"retry":true,"jid":"469979df52bb9ef9f48b49e1","created_at":1568305717.9457421,"enqueued_at":1568305717.9457731}`,
			wantClass: "ApiMailer#test_email",
			wantArgs: []any{
				map[string]any{"user": "gid://app/User/1"},
				[]any{float64(1), float64(2), float64(3)},
			},
		},
		{
			name:      "8x_active_job",
			value:     `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ApiAjJob","queue":"default","args":[{"job_class":"ApiAjJob","job_id":"37649eb0-c437-4e00-8a29-85cc12da1440","provider_job_id":null,"queue_name":"default","priority":null,"arguments":[1,2,3],"executions":0,"exception_executions":{},"locale":"en","timezone":null,"enqueued_at":"2024-09-19T16:39:26.609737000Z","scheduled_at":null}],"retry":true,"jid":"ec42f101ed1b16f27c6a6188","created_at":1726763966.610073,"enqueued_at":1726763966.6101718}`,
			wantClass: "ApiAjJob",
			wantArgs:  []any{float64(1), float64(2), float64(3)},
		},
		{
			name:      "8x_mailer_delivery",
			value:     `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","queue":"mailers","args":[{"job_class":"ActionMailer::MailDeliveryJob","job_id":"d6573e12-dd78-454a-83d5-67df94934c82","provider_job_id":null,"queue_name":"mailers","priority":null,"arguments":["ApiMailer","test_email","deliver_now",{"args":[1,2,3],"_aj_ruby2_keywords":["args"]}],"executions":0,"exception_executions":{},"locale":"en","timezone":null,"enqueued_at":"2024-09-19T16:45:38.673195000Z","scheduled_at":null}],"retry":true,"jid":"2cd7874f7651115f453d6315","created_at":1726764338.673335,"enqueued_at":1726764338.673404}`,
			wantClass: "ApiMailer#test_email",
			wantArgs: []any{
				nil,
				[]any{float64(1), float64(2), float64(3)},
			},
		},
	}
}
