package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultCapsuleName matches Sidekiq's implicit capsule name.
const DefaultCapsuleName = "default"

// Process represents a Sidekiq worker process.
type Process struct {
	client      *Client
	Identity    string             // hostname:pid:nonce (e.g., "be4860dbdb68:14:96908d62200c")
	Hostname    string             // Parsed from identity (e.g., "be4860dbdb68")
	PID         int                // Parsed from identity (e.g., 14)
	Tag         string             // From info.tag (e.g., "myapp")
	Concurrency int                // From info.concurrency
	Busy        int                // From busy field (converted to int)
	Beat        time.Time          // From beat field (heartbeat timestamp)
	Quiet       bool               // From quiet field
	Status      string             // Running, Pausing, Quiet, Stopping
	Capsules    map[string]Capsule // From info.capsules (Sidekiq 8+)
	RSS         int64              // From rss field in KB, convert to bytes (*1024)
	RTTUS       int64              // From rtt_us field (microseconds)
	StartedAt   time.Time          // From info.started_at (timestamp)
}

// Process status values.
const (
	ProcessStatusRunning  = "running"
	ProcessStatusPausing  = "pausing"
	ProcessStatusQuiet    = "quiet"
	ProcessStatusStopping = "stopping"
)

// Job represents an active Sidekiq job (currently running).
type Job struct {
	*JobRecord             // embedded job data from payload
	ProcessIdentity string // process identity running this job
	ThreadID        string // Base-36 encoded TID
	RunAt           time.Time
}

type workData struct {
	Queue   string  `json:"queue"`
	Payload string  `json:"payload"`
	RunAt   float64 `json:"run_at"`
}

// BusyData holds process and job information.
type BusyData struct {
	Processes []Process
	Jobs      []Job
}

// Capsule describes a Sidekiq capsule from process metadata.
type Capsule struct {
	Concurrency int
	Mode        string
	Weights     map[string]int
}

type processInfo struct {
	Hostname    string                 `json:"hostname"`
	StartedAt   float64                `json:"started_at"`
	PID         int                    `json:"pid"`
	Tag         string                 `json:"tag"`
	Concurrency int                    `json:"concurrency"`
	Queues      []string               `json:"queues"`
	Weights     json.RawMessage        `json:"weights"`
	Capsules    map[string]capsuleInfo `json:"capsules"`
	Labels      []string               `json:"labels"`
	Identity    string                 `json:"identity"`
	Version     string                 `json:"version"`
	Embedded    bool                   `json:"embedded"`
}

type capsuleInfo struct {
	Concurrency int            `json:"concurrency"`
	Mode        string         `json:"mode"`
	Weights     map[string]int `json:"weights"`
}

// NewProcess creates a new Process instance for the given identity.
func (c *Client) NewProcess(identity string) *Process {
	return &Process{
		client:   c,
		Identity: identity,
	}
}

// GetProcesses fetches all process identities from Redis, sorted alphabetically.
func (c *Client) GetProcesses(ctx context.Context) ([]*Process, error) {
	identities, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	sort.Strings(identities)

	processes := make([]*Process, len(identities))
	for i, identity := range identities {
		processes[i] = c.NewProcess(identity)
	}

	return processes, nil
}

// GetBusyData fetches detailed process and active job information from Redis.
// Uses pipelining to batch all Redis requests for optimal performance on large systems.
// If filter is non-empty, only jobs whose raw payload contains the substring are returned.
func (c *Client) GetBusyData(ctx context.Context, filter string) (BusyData, error) {
	var data BusyData

	// Step 1: Get all process identities
	identities, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return data, err
	}

	if len(identities) == 0 {
		return data, nil
	}

	sort.Strings(identities)

	// Step 2: Pipeline all process metadata fetches
	processResults, err := c.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, identity := range identities {
			pipe.HMGet(ctx, identity, "info", "busy", "beat", "quiet", "rss", "rtt_us")
		}
		return nil
	})
	if err != nil {
		return data, err
	}

	// Step 3: Pipeline all signal fetches
	signalResults, err := c.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, identity := range identities {
			pipe.LRange(ctx, identity+"-signals", 0, -1)
		}
		return nil
	})
	if err != nil {
		return data, err
	}

	// Step 4: Pipeline all work data fetches
	workResults, err := c.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, identity := range identities {
			pipe.HGetAll(ctx, identity+":work")
		}
		return nil
	})
	if err != nil {
		return data, err
	}

	// Step 5: Parse results
	data.Processes = make([]Process, 0, len(identities))
	data.Jobs = make([]Job, 0)

	for i, identity := range identities {
		process := c.NewProcess(identity)
		if cmd, ok := processResults[i].(*redis.SliceCmd); ok {
			fields, err := cmd.Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				continue
			}
			process.refreshFromFields(fields)
		}
		if cmd, ok := signalResults[i].(*redis.StringSliceCmd); ok {
			signals, err := cmd.Result()
			if err == nil || errors.Is(err, redis.Nil) {
				process.updateStatus(signals)
			}
		}

		// Only include processes that have valid info
		if process.Hostname == "" || process.PID == 0 {
			continue
		}

		data.Processes = append(data.Processes, *process)

		// Parse work data
		if cmd, ok := workResults[i].(*redis.MapStringStringCmd); ok {
			work, err := cmd.Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				continue
			}

			jobs := process.parseJobsFromWork(work, filter)
			data.Jobs = append(data.Jobs, jobs...)
		}
	}

	return data, nil
}

// Refresh updates process data from Redis.
func (p *Process) Refresh(ctx context.Context) error {
	if p.client == nil {
		return errors.New("process client is nil")
	}

	fields, err := p.client.redis.HMGet(ctx, p.Identity, "info", "busy", "beat", "quiet", "rss", "rtt_us").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	p.refreshFromFields(fields)
	return nil
}

// GetJobs fetches active jobs for the process.
// If filter is non-empty, only jobs whose raw work payload contains the substring are returned.
func (p *Process) GetJobs(ctx context.Context, filter string) ([]Job, error) {
	if p.client == nil {
		return nil, errors.New("process client is nil")
	}

	work, err := p.client.redis.HGetAll(ctx, p.Identity+":work").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	return p.parseJobsFromWork(work, filter), nil
}

// Pause signals the process to stop accepting new jobs.
func (p *Process) Pause(ctx context.Context) error {
	return p.signal(ctx, "TSTP")
}

// Stop signals the process to shutdown.
func (p *Process) Stop(ctx context.Context) error {
	return p.signal(ctx, "TERM")
}

func (p *Process) signal(ctx context.Context, sig string) error {
	if p.client == nil {
		return errors.New("process client is nil")
	}
	if p.Identity == "" {
		return errors.New("process identity is empty")
	}

	key := p.Identity + "-signals"
	_, err := p.client.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, key, sig)
		pipe.Expire(ctx, key, time.Minute)
		return nil
	})
	return err
}

// refreshFromFields updates process fields from HMGET results.
func (p *Process) refreshFromFields(fields []any) {
	p.Hostname = ""
	p.PID = 0
	p.Tag = ""
	p.Concurrency = 0
	p.Capsules = nil
	p.StartedAt = time.Time{}

	p.Busy = 0
	p.Beat = time.Time{}
	p.Quiet = false
	p.Status = ProcessStatusRunning
	p.RSS = 0
	p.RTTUS = 0

	parseProcessInfo(fieldAt(fields, 0), p)
	if p.Hostname == "" || p.PID == 0 {
		parts := strings.Split(p.Identity, ":")
		if len(parts) >= 2 {
			p.Hostname = parts[0]
			if pid, err := strconv.Atoi(parts[1]); err == nil {
				p.PID = pid
			}
		}
	}

	if busyCount, ok := parseOptionalInt64(fieldAt(fields, 1)); ok {
		p.Busy = int(busyCount)
	}

	if beat, ok := parseOptionalFloat64(fieldAt(fields, 2)); ok {
		p.Beat = timeFromSeconds(beat)
	}

	if quiet, ok := parseOptionalBool(fieldAt(fields, 3)); ok {
		p.Quiet = quiet
	}

	if rss, ok := parseOptionalInt64(fieldAt(fields, 4)); ok {
		p.RSS = rss * 1024
	}

	if rtt, ok := parseOptionalInt64(fieldAt(fields, 5)); ok {
		p.RTTUS = rtt
	}

	p.updateStatus(nil)
}

func (p *Process) updateStatus(signals []string) {
	status := ProcessStatusRunning
	switch {
	case slices.Contains(signals, "TERM"):
		status = ProcessStatusStopping
	case p.Quiet:
		status = ProcessStatusQuiet
	case slices.Contains(signals, "TSTP"):
		status = ProcessStatusPausing
	}
	p.Status = status
}

// parseJobsFromWork parses work hash data into jobs.
func (p *Process) parseJobsFromWork(work map[string]string, filter string) []Job {
	jobs := make([]Job, 0, len(work))
	for tid, workJSON := range work {
		if filter != "" && !strings.Contains(workJSON, filter) {
			continue
		}

		var wd workData
		if err := json.Unmarshal([]byte(workJSON), &wd); err != nil {
			continue
		}

		job := Job{
			ProcessIdentity: p.Identity,
			ThreadID:        tid,
		}

		if wd.RunAt > 0 {
			job.RunAt = timeFromSeconds(wd.RunAt)
		}

		if wd.Payload != "" {
			job.JobRecord = NewJobRecord(wd.Payload, wd.Queue)
		}

		jobs = append(jobs, job)
	}
	return jobs
}

func parseProcessInfo(field any, process *Process) {
	infoStr, ok := field.(string)
	if !ok || infoStr == "" {
		return
	}

	var info processInfo
	if err := json.Unmarshal([]byte(infoStr), &info); err != nil {
		return
	}

	process.Hostname = info.Hostname
	process.PID = info.PID
	if process.Identity == "" && info.Identity != "" {
		process.Identity = info.Identity
	}

	process.Concurrency = info.Concurrency
	if len(info.Capsules) > 0 {
		process.Capsules = parseProcessCapsules(info.Capsules)
	} else {
		queues, weights := parseProcessQueues(info.Queues, info.Weights)
		weights = normalizeCapsuleWeights(queues, weights)
		if len(queues) > 0 || len(weights) > 0 {
			process.Capsules = map[string]Capsule{
				DefaultCapsuleName: {
					Concurrency: info.Concurrency,
					Mode:        capsuleModeFromWeights(weights),
					Weights:     weights,
				},
			}
		}
	}
	process.Tag = info.Tag
	process.StartedAt = timeFromSeconds(info.StartedAt)
}

func parseProcessCapsules(capsules map[string]capsuleInfo) map[string]Capsule {
	if len(capsules) == 0 {
		return nil
	}

	parsed := make(map[string]Capsule, len(capsules))
	for name, capsule := range capsules {
		parsed[name] = Capsule{
			Concurrency: capsule.Concurrency,
			Mode:        capsule.Mode,
			Weights:     maps.Clone(capsule.Weights),
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	return parsed
}

// normalizeCapsuleWeights ensures each queue from legacy payloads has a weight.
//
// Sidekiq PR #6775 notes that process info now exposes capsule data and that the
// top-level "queues" and "weights" entries are deprecated and expected to be
// removed in a future release. Older payloads still rely on those fields, so we
// normalize them into a consistent weights map for the synthesized default
// capsule. See https://github.com/sidekiq/sidekiq/pull/6775 for details.
func normalizeCapsuleWeights(queues []string, weights map[string]int) map[string]int {
	if len(queues) == 0 && len(weights) == 0 {
		return nil
	}

	normalized := make(map[string]int, len(queues)+len(weights))
	if len(weights) > 0 {
		maps.Copy(normalized, weights)
	}
	for _, queue := range queues {
		if _, ok := normalized[queue]; !ok {
			normalized[queue] = 0
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func capsuleModeFromWeights(weights map[string]int) string {
	if len(weights) == 0 {
		return ""
	}

	allZero := true
	allOne := true
	for _, weight := range weights {
		if weight != 0 {
			allZero = false
		}
		if weight != 1 {
			allOne = false
		}
	}
	if allZero {
		return "strict"
	}
	if allOne {
		return "random"
	}
	return "weighted"
}

func parseProcessQueues(queues []string, weights json.RawMessage) ([]string, map[string]int) {
	parsedQueues, queueWeights := parseQueuesField(queues)
	weightMap := parseQueueWeightsRaw(weights)
	if len(queueWeights) == 0 && len(weightMap) == 0 {
		return parsedQueues, nil
	}

	merged := make(map[string]int)
	if len(queueWeights) > 0 {
		maps.Copy(merged, queueWeights)
	}
	if len(weightMap) > 0 {
		maps.Copy(merged, weightMap)
	}
	return parsedQueues, merged
}

func parseQueuesField(queues []string) ([]string, map[string]int) {
	if len(queues) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(queues))
	var weights map[string]int
	for _, q := range queues {
		name, weight, hasWeight := parseWeightedQueue(q)
		result = append(result, name)
		if hasWeight {
			if weights == nil {
				weights = make(map[string]int)
			}
			weights[name] = weight
		}
	}
	return result, weights
}

func parseWeightedQueue(queue string) (string, int, bool) {
	parts := strings.SplitN(queue, ",", 2)
	if len(parts) != 2 {
		return queue, 0, false
	}
	weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return queue, 0, false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return queue, 0, false
	}
	return name, weight, true
}

func parseQueueWeights(weights []map[string]int) map[string]int {
	if len(weights) == 0 {
		return nil
	}
	merged := make(map[string]int)
	for _, entry := range weights {
		maps.Copy(merged, entry)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func parseQueueWeightsRaw(raw json.RawMessage) map[string]int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var weights []map[string]int
	if err := json.Unmarshal(raw, &weights); err == nil && len(weights) > 0 {
		return parseQueueWeights(weights)
	}

	// Sidekiq 7.0.0 legacy format stored weights as a single hash.
	var legacy map[string]int
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy) > 0 {
		return parseQueueWeightsLegacy(legacy)
	}

	return nil
}

// parseQueueWeightsLegacy handles Sidekiq 7.0.0 weights format (single hash).
func parseQueueWeightsLegacy(weights map[string]int) map[string]int {
	if len(weights) == 0 {
		return nil
	}
	legacy := make(map[string]int, len(weights))
	maps.Copy(legacy, weights)
	return legacy
}

func timeFromSeconds(seconds float64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(seconds*float64(time.Second)))
}

func fieldAt(fields []any, idx int) any {
	if idx >= 0 && idx < len(fields) {
		return fields[idx]
	}
	return nil
}
