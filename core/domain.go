package core

import (
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"
)

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusRetrying  JobStatus = "retrying"
	JobStatusDead      JobStatus = "dead"
)

type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Priority    int             `json:"priority"`
	Status      JobStatus       `json:"status"`
	RetryCount  int             `json:"retry_count"`
	MaxRetries  int             `json:"max_retries"`
	CreatedAt   time.Time       `json:"created_at"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	NextRetryAt *time.Time      `json:"next_retry_at,omitempty"`
}

func NewJob(jobType string, payload json.RawMessage, priority int, maxRetries int) *Job {
	return &Job{
		ID:         ulid.Make().String(),
		Type:       jobType,
		Payload:    payload,
		Priority:   priority,
		Status:     JobStatusQueued,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now().UTC(),
	}
}

func (j *Job) CanRetry() bool {
	return j.RetryCount < j.MaxRetries
}

func (j *Job) BackoffDuration() time.Duration {
	base := 2 * time.Second
	return base * time.Duration(1<<j.RetryCount)
}

type Execution struct {
	ID         string     `json:"id"`
	JobID      string     `json:"job_id"`
	WorkerID   string     `json:"worker_id"`
	Logs       []string   `json:"logs"`
	Error      *string    `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func NewExecution(jobID, workerID string) *Execution {
	return &Execution{
		ID:        ulid.Make().String(),
		JobID:     jobID,
		WorkerID:  workerID,
		StartedAt: time.Now().UTC(),
		Logs:      make([]string, 0),
	}
}

func (e *Execution) AddLog(msg string) {
	e.Logs = append(e.Logs, msg)
}

func (e *Execution) SetError(err string) {
	e.Error = &err
	now := time.Now().UTC()
	e.FinishedAt = &now
}
