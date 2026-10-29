package views

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type readClient struct {
	sidekiq.API
	value string
	err   error
}

func (c *readClient) GetBusyData(context.Context, string) (sidekiq.BusyData, error) {
	return sidekiq.BusyData{Processes: []sidekiq.Process{{Identity: c.value}}}, c.err
}

func (c *readClient) GetErrorSummary(context.Context, string) ([]sidekiq.ErrorSummaryRow, sidekiq.ErrorSummaryMeta, error) {
	return []sidekiq.ErrorSummaryRow{{DisplayClass: c.value}}, sidekiq.ErrorSummaryMeta{}, c.err
}

func (c *readClient) GetStatsHistory(_ context.Context, days int) (sidekiq.StatsHistory, error) {
	return sidekiq.StatsHistory{Processed: []int64{int64(days)}}, c.err
}

func (c *readClient) GetRedisInfo(context.Context) (sidekiq.RedisInfo, error) {
	return sidekiq.RedisInfo{Version: c.value}, c.err
}

func (c *readClient) GetMetricsJobDetail(context.Context, string, sidekiq.MetricsPeriod) (sidekiq.MetricsJobDetailResult, error) {
	return sidekiq.MetricsJobDetailResult{Hist: map[string][]int64{c.value: {1}}, BucketCount: 1}, c.err
}

func (c *readClient) GetProcessesData(context.Context) ([]sidekiq.Process, error) {
	return []sidekiq.Process{{Identity: c.value}}, c.err
}
func (c *readClient) MetricsPeriodOrder(context.Context) []string { return []string{"1h", "2h"} }
func (c *readClient) GetMetricsTopJobs(context.Context, sidekiq.MetricsPeriod, string) (sidekiq.MetricsTopJobsResult, error) {
	return sidekiq.MetricsTopJobsResult{Jobs: map[string]sidekiq.MetricsJobTotals{c.value: {Processed: 1}}}, c.err
}

func (c *readClient) GetCapabilities() sidekiq.Capabilities               { return sidekiq.Capabilities{} }
func (c *readClient) GetQueues(context.Context) ([]*sidekiq.Queue, error) { return nil, c.err }

type readFixture struct {
	view    View
	fetch   func() tea.Cmd
	replace func() tea.Cmd
}

func readFixtures(c *readClient) map[string]readFixture {
	b := NewBusy(c)
	e := NewErrorsSummary(c)
	h := NewDashboard(c)
	d := NewDashboard(c)
	j := NewJobMetrics(c)
	j.SetJobMetrics("OldJob", "1h")
	p := NewProcessesList(c)
	m := NewMetrics(c)
	q := NewQueuesList(c)
	filtered := func(v View) func() tea.Cmd {
		return func() tea.Cmd {
			_, cmd := v.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "new"})
			return cmd
		}
	}
	return map[string]readFixture{
		"busy":        {b, b.fetchDataCmd, filtered(b)},
		"errors":      {e, func() tea.Cmd { return e.fetchDataCmd(true) }, filtered(e)},
		"history":     {h, h.fetchHistoryCmd, func() tea.Cmd { _, cmd := h.Update(tea.KeyPressMsg{Code: '}', Text: "}"}); return cmd }},
		"redis info":  {d, d.fetchRedisInfoCmd, d.fetchRedisInfoCmd},
		"job metrics": {j, j.fetchCmd, func() tea.Cmd { j.SetJobMetrics("NewJob", "2h"); return j.fetchCmd() }},
		"processes":   {p, p.fetchDataCmd, filtered(p)},
		"metrics":     {m, m.fetchListCmd, filtered(m)},
		"queues":      {q, q.fetchDataCmd, filtered(q)},
	}
}

// Capture the observable snapshot plus lifecycle state that must not be changed
// by an old completion, including pending state before the current reply arrives.
func readSnapshot(v View) string {
	switch v := v.(type) {
	case *Busy:
		return fmt.Sprintf("%v %v %d %v", v.ready, v.data, v.selectedProcess, v.table.Rows())
	case *ErrorsSummary:
		return fmt.Sprintf("%v %v %v %v %v", v.ready, v.refreshing, v.rows, v.meta, v.fetchedAt)
	case *Dashboard:
		return fmt.Sprintf("%v %v %v %v", v.historyProcessed, v.redisInfo, v.historyRangeIdx, v.historyChart)
	case *JobMetrics:
		return fmt.Sprintf("%v %v %v", v.jobName, v.result, v.processed)
	case *ProcessesList:
		return fmt.Sprintf("%v %v %v", v.ready, v.processes, v.table.Rows())
	case *Metrics:
		return fmt.Sprintf("%v %v %v %v", v.ready, v.result, v.rows, v.resetScroll)
	case *QueuesList:
		return fmt.Sprintf("%v %v %v", v.ready, v.queues, v.capabilities)
	default:
		panic("missing fixture")
	}
}

func TestReadResultsRejectStaleCompletions(t *testing.T) {
	failure := errors.New("read failed")
	for _, name := range []string{"busy", "errors", "history", "redis info", "job metrics", "processes", "metrics", "queues"} {
		for scenario := range map[string]bool{"refresh": true, "replace inputs": true, "hide and reopen": true, "dispose": true} {
			for outcome, errs := range map[string][2]error{"success then success": {}, "failure then success": {failure, nil}, "success then failure": {nil, failure}, "failure then failure": {failure, failure}} {
				t.Run(name+"/"+scenario+"/"+outcome, func(t *testing.T) {
					c := &readClient{value: "old", err: errs[0]}
					f := readFixtures(c)[name]
					t.Cleanup(f.view.(RequestCanceler).CancelRequests)
					old := f.fetch()()
					if old == nil {
						t.Fatal("missing old completion")
					}
					var cmd tea.Cmd
					switch scenario {
					case "refresh":
						cmd = f.fetch()
					case "replace inputs":
						cmd = f.replace()
					case "hide and reopen":
						f.view.(RequestCanceler).CancelRequests()
						cmd = f.fetch()
					case "dispose":
						if disposable, ok := f.view.(Disposable); ok {
							disposable.Dispose()
						} else {
							f.view.(RequestCanceler).CancelRequests()
						}
						cmd = f.fetch()
					}
					before := readSnapshot(f.view)
					f.view.Update(old)
					if got := readSnapshot(f.view); got != before {
						t.Fatalf("old completion changed pending state:\n%s\n%s", before, got)
					}
					c.value, c.err = "new", errs[1]
					fresh := cmd()
					f.view.Update(fresh)
					after := readSnapshot(f.view)
					if errs[1] == nil && before == after {
						t.Fatal("current success did not install data")
					}
					f.view.Update(old)
					if got := readSnapshot(f.view); got != after {
						t.Fatalf("old completion changed current state:\n%s\n%s", after, got)
					}
				})
			}
		}
	}
}

func TestReadResultsRejectOtherOwners(t *testing.T) {
	a, b := readFixtures(&readClient{value: "other"}), readFixtures(&readClient{})
	for name, f := range a {
		t.Run(name, func(t *testing.T) {
			other := b[name]
			t.Cleanup(f.view.(RequestCanceler).CancelRequests)
			t.Cleanup(other.view.(RequestCanceler).CancelRequests)
			reply := f.fetch()()
			before := readSnapshot(other.view)
			other.view.Update(reply)
			if got := readSnapshot(other.view); got != before {
				t.Fatal("accepted another instance's reply")
			}
		})
	}
}

func TestErrorsSummaryFailedCompletionAllowsRetry(t *testing.T) {
	for name, failure := range map[string]error{"read error": errors.New("offline"), "cancellation": context.Canceled} {
		t.Run(name, func(t *testing.T) {
			c := &readClient{err: failure}
			e := NewErrorsSummary(c)
			t.Cleanup(e.CancelRequests)
			for range 2 {
				reply := e.fetchDataCmd(true)()
				e.Update(reply)
				if e.refreshing {
					t.Fatal("failed completion left refresh pending")
				}
				c.err = nil
				_, retry := e.Update(RefreshMsg{})
				if retry == nil {
					t.Fatal("automatic refresh did not retry")
				}
				e.Update(retry())
				if !e.ready || e.refreshing {
					t.Fatal("retry did not recover")
				}
				// Expire the successful snapshot before testing a later failure.
				e.fetchedAt = time.Now().Add(-2 * errorsSummaryRefreshInterval)
				c.err = failure
			}
		})
	}
}

func TestReadResultsInvalidateWithoutReplacement(t *testing.T) {
	for _, name := range []string{"busy", "errors", "history", "redis info", "job metrics", "processes", "metrics", "queues"} {
		for scenario := range map[string]bool{"hide": true, "dispose": true} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				f := readFixtures(&readClient{value: "old"})[name]
				reply := f.fetch()()
				if disposable, ok := f.view.(Disposable); ok && scenario == "dispose" {
					disposable.Dispose()
				} else {
					f.view.(RequestCanceler).CancelRequests()
				}
				before := readSnapshot(f.view)
				f.view.Update(reply)
				if readSnapshot(f.view) != before {
					t.Fatal("hidden or disposed view accepted its old reply")
				}
			})
		}
	}
}

func TestJobMetricsClassReplacementInvalidatesImmediately(t *testing.T) {
	j := NewJobMetrics(&readClient{value: "OldJob"})
	j.SetJobMetrics("OldJob", "1h")
	old := j.fetchCmd()()
	j.SetJobMetrics("NewJob", "1h")
	j.Update(old)
	if j.processed != nil || j.result.Hist != nil {
		t.Fatal("old class data reappeared before new class fetched")
	}
}
