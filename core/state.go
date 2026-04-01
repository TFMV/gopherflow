package core

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RuntimeState struct {
	jobs    map[string]*Job
	workers map[string]WorkerState
	logs    map[string][]string
	mu      sync.RWMutex
	events  chan StateEvent
	metrics Metrics
}

type WorkerState struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
	ActiveJob string    `json:"active_job,omitempty"`
}

type StateEvent struct {
	Type    string
	Payload any
}

const (
	EventJobUpdated   = "job_updated"
	EventJobAdded     = "job_added"
	EventWorkerUpdate = "worker_update"
	EventJobLog       = "job_log"
)

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{
		jobs:    make(map[string]*Job),
		workers: make(map[string]WorkerState),
		logs:    make(map[string][]string),
		events:  make(chan StateEvent, 1000),
		metrics: NewMetrics(),
	}
}

func (s *RuntimeState) Subscribe() <-chan StateEvent {
	return s.events
}

func (s *RuntimeState) Publish(evt StateEvent) {
	select {
	case s.events <- evt:
	default:
	}
}

func (s *RuntimeState) AddJob(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	s.metrics.IncJobsTotal(string(job.Status))
	s.Publish(StateEvent{Type: EventJobAdded, Payload: job})
}

func (s *RuntimeState) UpdateJob(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldJob, ok := s.jobs[job.ID]
	if ok && oldJob.Status != job.Status {
		s.metrics.DecJobsTotal(string(oldJob.Status))
		s.metrics.IncJobsTotal(string(job.Status))
	}
	s.jobs[job.ID] = job
	s.Publish(StateEvent{Type: EventJobUpdated, Payload: job})
}

func (s *RuntimeState) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *RuntimeState) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (s *RuntimeState) UpdateWorker(state WorkerState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[state.ID] = state
	s.Publish(StateEvent{Type: EventWorkerUpdate, Payload: state})
}

func (s *RuntimeState) ListWorkers() []WorkerState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workers := make([]WorkerState, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	return workers
}

func (s *RuntimeState) AddLog(jobID, log string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs[jobID] = append(s.logs[jobID], log)
	s.Publish(StateEvent{Type: EventJobLog, Payload: map[string]string{"job_id": jobID, "log": log}})
}

func (s *RuntimeState) GetLogs(jobID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := make([]string, len(s.logs[jobID]))
	copy(logs, s.logs[jobID])
	return logs
}

func (s *RuntimeState) Metrics() *Metrics {
	return &s.metrics
}

type Metrics struct {
	jobsTotal      map[string]*int64
	jobsInFlight   atomic.Int64
	workerCount    atomic.Int64
	jobDurations   atomic.Int64
	jobDurationSum atomic.Int64
}

func NewMetrics() Metrics {
	return Metrics{
		jobsTotal: map[string]*int64{
			"queued":    new(int64),
			"running":   new(int64),
			"succeeded": new(int64),
			"failed":    new(int64),
			"retrying":  new(int64),
			"dead":      new(int64),
		},
	}
}

func (m *Metrics) IncJobsTotal(status string) {
	if v, ok := m.jobsTotal[status]; ok {
		atomic.AddInt64(v, 1)
	}
}

func (m *Metrics) DecJobsTotal(status string) {
	if v, ok := m.jobsTotal[status]; ok {
		atomic.AddInt64(v, -1)
	}
}

func (m *Metrics) SetJobsInFlight(n int64) {
	m.jobsInFlight.Store(n)
}

func (m *Metrics) IncJobsInFlight() {
	m.jobsInFlight.Add(1)
}

func (m *Metrics) DecJobsInFlight() {
	m.jobsInFlight.Add(-1)
}

func (m *Metrics) SetWorkerCount(n int64) {
	m.workerCount.Store(n)
}

func (m *Metrics) ObserveJobDuration(d time.Duration) {
	m.jobDurations.Add(1)
	m.jobDurationSum.Add(int64(d))
}

func (m *Metrics) GetJobsTotal(status string) int64 {
	if v, ok := m.jobsTotal[status]; ok {
		return atomic.LoadInt64(v)
	}
	return 0
}

func (m *Metrics) GetJobsInFlight() int64 {
	return m.jobsInFlight.Load()
}

func (m *Metrics) GetWorkerCount() int64 {
	return m.workerCount.Load()
}

func (m *Metrics) GetAvgJobDuration() float64 {
	count := m.jobDurations.Load()
	if count == 0 {
		return 0
	}
	return float64(m.jobDurationSum.Load()) / float64(count) / float64(time.Second)
}

func (m *Metrics) String() string {
	var lines []string
	lines = append(lines, "# HELP gopherflow_jobs_total Total jobs by status")
	lines = append(lines, "# TYPE gopherflow_jobs_total counter")
	for status, v := range m.jobsTotal {
		lines = append(lines, fmt.Sprintf(`gopherflow_jobs_total{status="%s"} %d`, status, atomic.LoadInt64(v)))
	}
	lines = append(lines, "")
	lines = append(lines, "# HELP gopherflow_jobs_in_flight Jobs currently being processed")
	lines = append(lines, "# TYPE gopherflow_jobs_in_flight gauge")
	lines = append(lines, fmt.Sprintf("gopherflow_jobs_in_flight %d", m.jobsInFlight.Load()))
	lines = append(lines, "")
	lines = append(lines, "# HELP gopherflow_worker_count Number of active workers")
	lines = append(lines, "# TYPE gopherflow_worker_count gauge")
	lines = append(lines, fmt.Sprintf("gopherflow_worker_count %d", m.workerCount.Load()))
	lines = append(lines, "")
	lines = append(lines, "# HELP gopherflow_job_duration_seconds_avg Average job duration in seconds")
	lines = append(lines, "# TYPE gopherflow_job_duration_seconds_avg gauge")
	lines = append(lines, fmt.Sprintf("gopherflow_job_duration_seconds_avg %.3f", m.GetAvgJobDuration()))
	return strings.Join(lines, "\n")
}
