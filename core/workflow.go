package core

import (
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"
)

type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Steps       []WorkflowStep `json:"steps"`
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type WorkflowStep struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	JobType      string          `json:"job_type"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Dependencies []string        `json:"dependencies"`
	Status       string          `json:"status"`
	JobID        *string         `json:"job_id,omitempty"`
}

func NewWorkflow(name string) *Workflow {
	return &Workflow{
		ID:        ulid.Make().String(),
		Name:      name,
		Steps:     make([]WorkflowStep, 0),
		CreatedAt: time.Now().UTC(),
	}
}

func (w *Workflow) AddStep(name, jobType string, payload json.RawMessage, deps []string) WorkflowStep {
	step := WorkflowStep{
		ID:           ulid.Make().String(),
		Name:         name,
		JobType:      jobType,
		Payload:      payload,
		Dependencies: deps,
		Status:       "pending",
	}
	w.Steps = append(w.Steps, step)
	return step
}
