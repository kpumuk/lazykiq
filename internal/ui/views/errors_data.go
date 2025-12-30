package views

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type errorSummaryRow struct {
	displayClass string
	errorClass   string
	queue        string
	count        int64
	errorMessage string
}

type errorSummaryKey struct {
	displayClass string
	errorClass   string
	queue        string
}

type errorGroupJob struct {
	entry  *sidekiq.SortedEntry
	source string
}

func fetchErrorJobs(ctx context.Context, client *sidekiq.Client, query string) ([]*sidekiq.SortedEntry, []*sidekiq.SortedEntry, error) {
	if query != "" {
		deadJobs, err := client.ScanDeadJobs(ctx, query)
		if err != nil {
			return nil, nil, err
		}

		retryJobs, err := client.ScanRetryJobs(ctx, query)
		if err != nil {
			return nil, nil, err
		}

		return deadJobs, retryJobs, nil
	}

	deadJobs, _, err := client.GetDeadJobs(ctx, 0, -1)
	if err != nil {
		return nil, nil, err
	}

	retryJobs, _, err := client.GetRetryJobs(ctx, 0, -1)
	if err != nil {
		return nil, nil, err
	}

	return deadJobs, retryJobs, nil
}

func buildErrorSummary(deadJobs, retryJobs []*sidekiq.SortedEntry) ([]errorSummaryRow, map[errorSummaryKey][]errorGroupJob) {
	rowsByKey := make(map[errorSummaryKey]*errorSummaryRow)
	groups := make(map[errorSummaryKey][]errorGroupJob)

	addJobs := func(jobs []*sidekiq.SortedEntry, source string) {
		for _, job := range jobs {
			if job == nil || job.JobRecord == nil {
				continue
			}

			displayClass := strings.TrimSpace(job.DisplayClass())
			if displayClass == "" {
				displayClass = "unknown"
			}

			errorClass := strings.TrimSpace(job.ErrorClass())
			if errorClass == "" {
				errorClass = "unknown"
			}

			queue := strings.TrimSpace(job.Queue())
			if queue == "" {
				queue = "unknown"
			}

			errorMessage := errorMessageOnly(job)

			key := errorSummaryKey{
				displayClass: displayClass,
				errorClass:   errorClass,
				queue:        queue,
			}
			groups[key] = append(groups[key], errorGroupJob{
				entry:  job,
				source: source,
			})
			if row, ok := rowsByKey[key]; ok {
				row.count++
				continue
			}

			rowsByKey[key] = &errorSummaryRow{
				displayClass: displayClass,
				errorClass:   errorClass,
				queue:        queue,
				count:        1,
				errorMessage: errorMessage,
			}
		}
	}

	addJobs(deadJobs, "dead")
	addJobs(retryJobs, "retry")

	rows := make([]errorSummaryRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].displayClass != rows[j].displayClass {
			return rows[i].displayClass < rows[j].displayClass
		}
		if rows[i].errorClass != rows[j].errorClass {
			return rows[i].errorClass < rows[j].errorClass
		}
		if rows[i].queue != rows[j].queue {
			return rows[i].queue < rows[j].queue
		}
		return rows[i].errorMessage < rows[j].errorMessage
	})

	return rows, groups
}

func errorDisplay(job *sidekiq.SortedEntry) string {
	if job == nil || job.JobRecord == nil {
		return ""
	}
	if !job.HasError() {
		return ""
	}
	errorStr := fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
	if len(errorStr) > 100 {
		errorStr = errorStr[:99] + "…"
	}
	return errorStr
}

func errorMessageOnly(job *sidekiq.SortedEntry) string {
	if job == nil || job.JobRecord == nil {
		return "unknown"
	}
	message := strings.TrimSpace(job.ErrorMessage())
	if message == "" {
		return "unknown"
	}
	return message
}
