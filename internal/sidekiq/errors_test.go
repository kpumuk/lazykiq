package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestGetErrorSummaryExactAcrossScanBatches(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	for i := range 130 {
		addSortedSetJob(t, mr, deadSetKey, float64(i+1), errorPayload(
			fmt.Sprintf("dead-cleanup-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("dead cleanup %03d", i),
			"",
		))
	}
	for i := range 95 {
		addSortedSetJob(t, mr, retrySetKey, float64(i+1), errorPayload(
			fmt.Sprintf("retry-cleanup-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("retry cleanup %03d", i),
			"",
		))
	}
	for i := range 40 {
		addSortedSetJob(t, mr, deadSetKey, float64(1000+i), errorPayload(
			fmt.Sprintf("dead-mail-%03d", i),
			"MailJob",
			"mailers",
			"TimeoutError",
			fmt.Sprintf("mail timeout %03d", i),
			"",
		))
	}

	rows, meta, err := client.GetErrorSummary(ctx, "")
	if err != nil {
		t.Fatalf("GetErrorSummary failed: %v", err)
	}

	if meta.DeadCount != 170 {
		t.Fatalf("meta.DeadCount = %d, want 170", meta.DeadCount)
	}
	if meta.RetryCount != 95 {
		t.Fatalf("meta.RetryCount = %d, want 95", meta.RetryCount)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	byKey := map[ErrorGroupKey]ErrorSummaryRow{}
	for _, row := range rows {
		byKey[ErrorGroupKey{
			DisplayClass: row.DisplayClass,
			ErrorClass:   row.ErrorClass,
			Queue:        row.Queue,
		}] = row
	}

	cleanup := byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}]
	if cleanup.Count != 225 {
		t.Fatalf("cleanup.Count = %d, want 225", cleanup.Count)
	}
	if cleanup.ErrorMessage != "dead cleanup 129" {
		t.Fatalf("cleanup.ErrorMessage = %q, want %q", cleanup.ErrorMessage, "dead cleanup 129")
	}

	mail := byKey[ErrorGroupKey{DisplayClass: "MailJob", ErrorClass: "TimeoutError", Queue: "mailers"}]
	if mail.Count != 40 {
		t.Fatalf("mail.Count = %d, want 40", mail.Count)
	}
}

func TestGetErrorSummaryFilteredAcrossScanBatches(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	for i := range 120 {
		marker := ""
		if i < 70 {
			marker = "needle"
		}
		addSortedSetJob(t, mr, deadSetKey, float64(i+1), errorPayload(
			fmt.Sprintf("dead-match-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("dead cleanup %03d", i),
			marker,
		))
	}
	for i := range 110 {
		marker := ""
		if i < 45 {
			marker = "needle"
		}
		addSortedSetJob(t, mr, retrySetKey, float64(i+1), errorPayload(
			fmt.Sprintf("retry-match-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("retry cleanup %03d", i),
			marker,
		))
	}
	for i := range 10 {
		addSortedSetJob(t, mr, retrySetKey, float64(1000+i), errorPayload(
			fmt.Sprintf("retry-other-%03d", i),
			"OtherJob",
			"critical",
			"RuntimeError",
			fmt.Sprintf("other failure %03d", i),
			"needle",
		))
	}

	rows, meta, err := client.GetErrorSummary(ctx, "needle")
	if err != nil {
		t.Fatalf("GetErrorSummary failed: %v", err)
	}

	if meta.DeadCount != 70 {
		t.Fatalf("meta.DeadCount = %d, want 70", meta.DeadCount)
	}
	if meta.RetryCount != 55 {
		t.Fatalf("meta.RetryCount = %d, want 55", meta.RetryCount)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	byKey := map[ErrorGroupKey]int64{}
	for _, row := range rows {
		byKey[ErrorGroupKey{
			DisplayClass: row.DisplayClass,
			ErrorClass:   row.ErrorClass,
			Queue:        row.Queue,
		}] = row.Count
	}

	if byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}] != 115 {
		t.Fatalf("cleanup filtered count = %d, want 115", byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}])
	}
	if byKey[ErrorGroupKey{DisplayClass: "OtherJob", ErrorClass: "RuntimeError", Queue: "critical"}] != 10 {
		t.Fatalf("other filtered count = %d, want 10", byKey[ErrorGroupKey{DisplayClass: "OtherJob", ErrorClass: "RuntimeError", Queue: "critical"}])
	}
}

func TestGetErrorGroupWindowPagedAcrossDeadAndRetry(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	groupKey := ErrorGroupKey{
		DisplayClass: "CleanupJob",
		ErrorClass:   "ArgumentError",
		Queue:        "default",
	}

	addSortedSetJob(t, mr, deadSetKey, 1, errorPayload("dead1", "CleanupJob", "default", "ArgumentError", "dead one", ""))
	addSortedSetJob(t, mr, deadSetKey, 2, errorPayload("dead2", "CleanupJob", "default", "ArgumentError", "dead two", ""))
	addSortedSetJob(t, mr, deadSetKey, 3, errorPayload("dead3", "CleanupJob", "default", "ArgumentError", "dead three", ""))
	addSortedSetJob(t, mr, retrySetKey, 10, errorPayload("retry1", "CleanupJob", "default", "ArgumentError", "retry one", ""))
	addSortedSetJob(t, mr, retrySetKey, 20, errorPayload("retry2", "CleanupJob", "default", "ArgumentError", "retry two", ""))
	addSortedSetJob(t, mr, retrySetKey, 30, errorPayload("retry3", "CleanupJob", "default", "ArgumentError", "retry three", ""))
	addSortedSetJob(t, mr, retrySetKey, 40, errorPayload("other", "OtherJob", "critical", "RuntimeError", "ignore", ""))

	window, err := client.GetErrorGroupWindow(ctx, groupKey, "", 2, 3)
	if err != nil {
		t.Fatalf("GetErrorGroupWindow failed: %v", err)
	}

	if window.Total != 6 {
		t.Fatalf("window.Total = %d, want 6", window.Total)
	}
	if window.WindowStart != 2 {
		t.Fatalf("window.WindowStart = %d, want 2", window.WindowStart)
	}
	if len(window.Entries) != 3 {
		t.Fatalf("len(window.Entries) = %d, want 3", len(window.Entries))
	}

	got := []string{
		window.Entries[0].Source + ":" + window.Entries[0].Entry.JID(),
		window.Entries[1].Source + ":" + window.Entries[1].Entry.JID(),
		window.Entries[2].Source + ":" + window.Entries[2].Entry.JID(),
	}
	want := []string{"dead:dead1", "retry:retry1", "retry:retry2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("window entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func newErrorsTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client, err := NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client, mr
}

func addSortedSetJob(t *testing.T, mr *miniredis.Miniredis, key string, score float64, payload string) {
	t.Helper()
	if _, err := mr.ZAdd(key, score, payload); err != nil {
		t.Fatalf("ZAdd(%s) failed: %v", key, err)
	}
}

func errorPayload(jid, class, queue, errorClass, errorMessage, marker string) string {
	return fmt.Sprintf(
		`{"jid":"%s","class":"%s","queue":"%s","args":["%s"],"error_class":"%s","error_message":"%s"}`,
		jid,
		class,
		queue,
		marker,
		errorClass,
		errorMessage,
	)
}

func TestErrorMetadataMatchesJobRecord(t *testing.T) {
	fixtures := errorMetadataFixtures()
	for _, fixture := range serializedActiveJobTests() {
		fixtures[fixture.name] = fixture.value
	}
	for name, payload := range fixtures {
		t.Run(name, func(t *testing.T) {
			for _, queue := range []string{"", "override"} {
				for _, parsed := range []bool{false, true} {
					entry := NewSortedEntry(payload, 1)
					entry.queue = queue
					if parsed {
						entry.ensureParsed()
					}
					want := legacyErrorMetadata(NewJobRecord(payload, queue))
					if got := errorMetadataFromEntry(entry); got != want {
						t.Fatalf("queue=%q parsed=%v: got %#v, want %#v", queue, parsed, got, want)
					}
					if got := normalizedErrorGroupKeyFromEntry(entry); got != want.groupKey() {
						t.Fatalf("key = %#v, want %#v", got, want.groupKey())
					}
					// Grouping must not change later ordinary JobRecord access.
					if got := legacyErrorMetadata(entry.JobRecord); got != want {
						t.Fatalf("later JobRecord access = %#v, want %#v", got, want)
					}
				}
			}
		})
	}
	for _, entry := range []*SortedEntry{nil, {}} {
		if got := errorMetadataFromEntry(entry); got != (errorJobMetadata{}) {
			t.Fatalf("nil entry metadata = %#v", got)
		}
	}
}

func TestErrorMetadataLeavesUnusedPayloadLazy(t *testing.T) {
	for _, field := range []string{"args", "context", "error_backtrace"} {
		t.Run(field, func(t *testing.T) {
			payload := fmt.Sprintf(`{"class":"Worker","queue":"default","%s":["%s"],"error_class":"Error","error_message":" boom "}`, field, strings.Repeat("x", 65536))
			entry := NewSortedEntry(payload, 1)
			rows := make(map[ErrorGroupKey]*errorSummaryState)
			addErrorSummaryEntry(rows, entry, "dead")
			if entry.parsed || entry.item != nil || entry.args != nil || entry.errorBacktraceSet {
				t.Fatal("grouping materialized unused job data")
			}
			state := rows[ErrorGroupKey{DisplayClass: "Worker", Queue: "default", ErrorClass: "Error"}]
			if state == nil || state.row.Count != 1 || state.row.ErrorMessage != "boom" {
				t.Fatalf("unexpected grouped state: %#v", state)
			}
			if got := entry.Item()[field]; got == nil {
				t.Fatalf("later JobRecord access lost %s", field)
			}
		})
	}
}

func TestErrorMetadataLeavesWrapperPayloadLazy(t *testing.T) {
	for name, tt := range map[string]struct {
		fields string
		class  string
	}{
		"wrapped":       {`"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Worker","args":["%s"]`, "Worker"},
		"adapter":       {`"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"Worker","args":["%s"]`, "Worker"},
		"legacy":        {`"class":"Sidekiq::ActiveJob::Wrapper","args":["Worker","%s"]`, "Worker"},
		"mailer":        {`"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["Mailer","welcome","%s"]}]`, "Mailer#welcome"},
		"global IDs":    {`"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":[{"_aj_globalid":"Mailer"},{"_aj_globalid":"welcome"},"%s"]}]`, "Mailer#welcome"},
		"escaped class": {`"cl\u0061ss":"Sidekiq::\u0041ctiveJob::Wrapper","wrapped":"Worker","args":["%s"]`, "Worker"},
		"class last":    {`"args":["%s"],"wrapped":"Worker","class":"Sidekiq::ActiveJob::Wrapper"`, "Worker"},
		"duplicate":     {`"class":"Worker","args":["%s"],"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Worker"`, "Worker"},
		"no name":       {`"class":"Sidekiq::ActiveJob::Wrapper","args":[null,"%s"]`, activeJobWrapper},
	} {
		t.Run(name, func(t *testing.T) {
			payload := "{" + fmt.Sprintf(tt.fields, strings.Repeat("x", 65536)) + `,"queue":"default","error_class":"Error","error_message":"boom"}`
			entry := NewSortedEntry(payload, 1)
			metadata := errorMetadataFromEntry(entry)
			if metadata != legacyErrorMetadata(NewJobRecord(payload, "")) || metadata.class != tt.class {
				t.Fatalf("unexpected wrapper metadata: %#v", metadata)
			}
			if entry.parsed || entry.item != nil || entry.args != nil {
				t.Fatal("grouping materialized wrapper arguments")
			}
			if got := entry.DisplayClass(); got != tt.class {
				t.Fatalf("later DisplayClass = %q, want %q", got, tt.class)
			}
		})
	}
}

func TestErrorSummaryRepresentativePrecedence(t *testing.T) {
	for name, tt := range map[string]struct {
		sources []string
		message string
	}{
		"dead then retry": {[]string{"dead", "retry"}, "dead 3"},
		"retry then dead": {[]string{"retry", "dead"}, "dead 3"},
		"only retry":      {[]string{"retry"}, "retry 1"},
	} {
		t.Run(name, func(t *testing.T) {
			rows := make(map[ErrorGroupKey]*errorSummaryState)
			for _, source := range tt.sources {
				for _, score := range []int{2, 1, 3} {
					payload := errorPayload("id", " Worker ", " default ", " Error ", fmt.Sprintf(" %s %d ", source, score), "")
					addErrorSummaryEntry(rows, NewSortedEntry(payload, float64(score)), source)
				}
			}
			state := rows[ErrorGroupKey{DisplayClass: "Worker", Queue: "default", ErrorClass: "Error"}]
			if state == nil || state.row.Count != int64(3*len(tt.sources)) || state.row.ErrorMessage != tt.message {
				t.Fatalf("unexpected representative: %#v, want %q", state, tt.message)
			}
		})
	}
}

func TestErrorSummaryMalformedAndWrappedJobs(t *testing.T) {
	client, mr := newErrorsTestClient(t)
	for i, payload := range []string{
		`{"class":"Worker","queue":"default","error_class":"Error","error_message":" older "}`,
		`{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Worker","queue":"default","error_class":"Error","error_message":" newest "}`,
		`{"class":" ","queue":"\t","error_class":null,"error_message":" "}`,
		`{"class":"Worker","error_class":"Error","args":[1e999]}`,
		`{"class":"Worker",`,
	} {
		addSortedSetJob(t, mr, deadSetKey, float64(i), payload)
	}
	rows, meta, err := client.GetErrorSummary(testContext(t), "")
	want := []ErrorSummaryRow{
		{DisplayClass: "Worker", Queue: "default", ErrorClass: "Error", Count: 2, ErrorMessage: "newest"},
		{DisplayClass: "unknown", Queue: "unknown", ErrorClass: "unknown", Count: 3, ErrorMessage: "unknown"},
	}
	if err != nil || meta.DeadCount != 5 || meta.RetryCount != 0 || !slices.Equal(rows, want) {
		t.Fatalf("summary = %#v, %#v, %v; want %#v", rows, meta, err, want)
	}
	window, err := client.GetErrorGroupWindow(testContext(t), ErrorGroupKey{}, "", 99, 2)
	if err != nil || window.Total != 3 || window.WindowStart != 1 || len(window.Entries) != 2 {
		t.Fatalf("clamped unknown group = %#v, %v", window, err)
	}
	for _, entry := range window.Entries {
		if normalizedErrorGroupKeyFromEntry(entry.Entry) != normalizedErrorGroupKey(ErrorGroupKey{}) {
			t.Fatal("group included a nonmatching job")
		}
	}
}

func TestErrorGroupingCancellation(t *testing.T) {
	client, _ := newErrorsTestClient(t)
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if _, _, err := client.GetErrorSummary(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("summary error = %v, want context.Canceled", err)
	}
	if _, err := client.GetErrorGroupWindow(ctx, ErrorGroupKey{}, "", 0, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("group window error = %v, want context.Canceled", err)
	}
}

func TestErrorGroupWindowOptions(t *testing.T) {
	client, mr := newErrorsTestClient(t)
	for i := range 3 {
		addSortedSetJob(t, mr, deadSetKey, float64(i), errorPayload(fmt.Sprintf("dead%d", i), "Worker", "default", "Error", "boom", "needle"))
		addSortedSetJob(t, mr, retrySetKey, float64(i), errorPayload(fmt.Sprintf("retry%d", i), "Worker", "default", "Error", "boom", "needle"))
	}
	key := ErrorGroupKey{DisplayClass: " Worker ", Queue: "default", ErrorClass: "Error"}
	for name, tt := range map[string]struct {
		start, count int
		query        string
		wantStart    int
		wantTotal    int64
		want         []string
	}{
		"negative start": {-10, 2, "", 0, 6, []string{"dead2", "dead1"}},
		"cross sets":     {2, 2, "", 2, 6, []string{"dead0", "retry0"}},
		"tail clamp":     {99, 2, "", 4, 6, []string{"retry1", "retry2"}},
		"all remaining":  {2, -1, "", 2, 6, []string{"dead0", "retry0", "retry1", "retry2"}},
		"count only":     {99, 0, "", 99, 6, nil},
		"unused args":    {2, 2, "needle", 2, 6, []string{"dead0", "retry0"}},
		"no matches":     {0, 2, "missing", 0, 0, nil},
	} {
		t.Run(name, func(t *testing.T) {
			window, err := client.GetErrorGroupWindow(testContext(t), key, tt.query, tt.start, tt.count)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, entry := range window.Entries {
				got = append(got, entry.Entry.JID())
			}
			if window.Total != tt.wantTotal || window.WindowStart != tt.wantStart || !slices.Equal(got, tt.want) {
				t.Fatalf("window = %#v, jobs = %v; want start=%d total=%d jobs=%v", window, got, tt.wantStart, tt.wantTotal, tt.want)
			}
		})
	}
}

func FuzzErrorMetadataMatchesJobRecord(f *testing.F) {
	for _, payload := range errorMetadataFixtures() {
		f.Add(payload)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		if len(payload) > 1<<20 {
			t.Skip()
		}
		entry := NewSortedEntry(payload, 0)
		want := legacyErrorMetadata(NewJobRecord(payload, ""))
		if got := errorMetadataFromEntry(entry); got != want {
			t.Fatalf("metadata = %#v, want %#v", got, want)
		}
	})
}

func legacyErrorMetadata(jr *JobRecord) errorJobMetadata {
	return errorJobMetadata{
		class:      jr.DisplayClass(),
		queue:      jr.Queue(),
		errorClass: jr.ErrorClass(),
		message:    jr.ErrorMessage(),
	}
}

func errorMetadataFixtures() map[string]string {
	return map[string]string{
		"ordinary":            `{"class":"Worker","queue":"default","args":[1,{"nested":[true,false,null]}],"error_class":"Error","error_message":" bad "}`,
		"whitespace":          " \n { \"class\" : \" Worker \", \"queue\" : \" \", \"error_class\" : \"\\t\" } \r\n",
		"wrong types":         `{"class":[],"queue":true,"error_class":{},"error_message":5}`,
		"null fields":         `{"class":null,"queue":null,"error_class":null,"error_message":null}`,
		"duplicate":           `{"class":"Old","class":"New","queue":"old","queue":null,"error_class":"Old","error_class":0,"error_message":"old","error_message":false}`,
		"case sensitive":      `{"Class":"Wrong","QUEUE":"Wrong","ERROR_CLASS":"Wrong","error_MESSAGE":"Wrong","class":"Right"}`,
		"escaped keys":        `{"cl\u0061ss":"Escaped","qu\u0065ue":"default","error_\u0063lass":"Error","error_\u006dessage":"bad"}`,
		"escaped strings":     `{"class":"A\\B\"C","queue":"a\/b","error_class":"\ud83d\ude00","error_message":"\ud800"}`,
		"invalid UTF8":        "{\"class\":\"\xff\"}",
		"overflow":            `{"class":"Worker","args":[{"unused":[1e309]}]}`,
		"negative overflow":   `{"class":"Worker","context":{"a":-1e309}}`,
		"duplicate overflow":  `{"class":"Worker","args":[1e309],"args":[]}`,
		"underflow":           `{"class":"Worker","args":[1e-999,0.001,-0,1.7976931348623157e308]}`,
		"trailing garbage":    `{"class":"Worker"} []`,
		"truncated":           `{"class":"Worker","args":[`,
		"invalid string":      `{"class":"Worker","args":["\x"]}`,
		"null":                `null`,
		"array":               `[{"class":"Worker"}]`,
		"empty":               ``,
		"empty object":        `{}`,
		"wrapper":             `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Wrapped"}`,
		"wrapper arg":         `{"class":"Sidekiq::ActiveJob::Wrapper","args":["Wrapped",1]}`,
		"wrapper null":        `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":null,"args":["Wrapped"]}`,
		"wrapper blank":       `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"","args":["Ignored"]}`,
		"wrapper duplicates":  `{"class":"Old","class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Old","wrapped":false,"args":["Old"],"args":["New"]}`,
		"wrapper replaced":    `{"class":"Sidekiq::ActiveJob::Wrapper","class":"Ordinary","wrapped":"Ignored"}`,
		"mailer":              `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["Mailer","welcome",{"unused":true}]}]}`,
		"mailer malformed":    `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":[null,12]}]}`,
		"mailer global IDs":   `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":[{"_aj_globalid":"Mailer"},{"_aj_globalid":"welcome"}]}]}`,
		"mailer ID array":     `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":{"_aj_globalid":["Mailer","welcome"]}}]}`,
		"mailer raw ID array": `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":{"_aj_globalid":[{"_aj_globalid":"Mailer"},"welcome"]}}]}`,
		"mailer ID extra":     `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":[{"_aj_globalid":"Mailer","_aj_ignore":true},"welcome"]}]}`,
		"mailer duplicates":   `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::DeliveryJob","args":[{"arguments":["Old","old"],"arguments":[{"_aj_globalid":"Old","_aj_globalid":"Mailer"},"welcome"]}]}`,
	}
}

var benchmarkErrorMetadata errorJobMetadata

func BenchmarkErrorMetadataWrappers(b *testing.B) {
	for name, fields := range map[string]string{
		"sidekiq": `"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"Worker","args":["%s"]`,
		"adapter": `"class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"Worker","args":["%s"]`,
		"legacy":  `"class":"Sidekiq::ActiveJob::Wrapper","args":["Worker","%s"]`,
		"mailer":  `"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":["Mailer","welcome","deliver_now","%s"]}]`,
	} {
		for _, width := range []int{32, 65536} {
			payload := "{" + fmt.Sprintf(fields, strings.Repeat("x", width)) + `,"queue":"default","error_class":"Error","error_message":"bad"}`
			for _, mode := range []string{"legacy", "metadata"} {
				b.Run(fmt.Sprintf("%s/bytes=%d/%s", name, width, mode), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if mode == "legacy" {
							benchmarkErrorMetadata = legacyErrorMetadata(NewJobRecord(payload, ""))
						} else {
							benchmarkErrorMetadata = errorMetadataFromEntry(NewSortedEntry(payload, 0))
						}
					}
				})
			}
		}
	}
}

func BenchmarkErrorMetadataNumericMailer(b *testing.B) {
	payload := `{"class":"Sidekiq::ActiveJob::Wrapper","wrapped":"ActionMailer::MailDeliveryJob","args":[{"arguments":["Mailer","welcome",[` + strings.Repeat("123456789.123456,", 4095) + `1]]}],"queue":"default","error_class":"Error","error_message":"bad"}`
	for _, mode := range []string{"legacy", "metadata"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "legacy" {
					benchmarkErrorMetadata = legacyErrorMetadata(NewJobRecord(payload, ""))
				} else {
					benchmarkErrorMetadata = errorMetadataFromEntry(NewSortedEntry(payload, 0))
				}
			}
		})
	}
}
