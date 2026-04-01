package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

type Executor interface {
	Name() string
	Execute(ctx context.Context, job *Job) (*Execution, error)
}

type ExecutorRegistry struct {
	mu       sync.RWMutex
	handlers map[string]Executor
}

func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		handlers: make(map[string]Executor),
	}
}

func (r *ExecutorRegistry) Register(ex Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[ex.Name()] = ex
}

func (r *ExecutorRegistry) Get(name string) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ex, ok := r.handlers[name]
	return ex, ok
}

type ShellExecutor struct{}

func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{}
}

func (e *ShellExecutor) Name() string {
	return "shell"
}

func (e *ShellExecutor) Execute(ctx context.Context, job *Job) (*Execution, error) {
	var payload struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("invalid shell payload: %w", err)
	}

	if payload.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", payload.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("command failed: %w", err)
		}
	}

	ex := NewExecution(job.ID, "")
	ex.AddLog(fmt.Sprintf("Command: %s", payload.Command))
	if stdout.Len() > 0 {
		ex.AddLog(fmt.Sprintf("stdout: %s", stdout.String()))
	}
	if stderr.Len() > 0 {
		ex.AddLog(fmt.Sprintf("stderr: %s", stderr.String()))
	}
	ex.AddLog(fmt.Sprintf("Exit code: %d", cmd.ProcessState.ExitCode()))

	finished := time.Now().UTC()
	ex.FinishedAt = &finished

	return ex, nil
}

type EchoExecutor struct{}

func NewEchoExecutor() *EchoExecutor {
	return &EchoExecutor{}
}

func (e *EchoExecutor) Name() string {
	return "echo"
}

func (e *EchoExecutor) Execute(ctx context.Context, job *Job) (*Execution, error) {
	ex := NewExecution(job.ID, "")
	ex.AddLog(fmt.Sprintf("Echo executor received job %s", job.ID))
	ex.AddLog(fmt.Sprintf("Job type: %s", job.Type))
	ex.AddLog(fmt.Sprintf("Payload: %s", string(job.Payload)))

	var payload map[string]any
	if json.Unmarshal(job.Payload, &payload) == nil {
		for k, v := range payload {
			ex.AddLog(fmt.Sprintf("  %s: %v", k, v))
		}
	}

	finished := time.Now().UTC()
	ex.FinishedAt = &finished

	return ex, nil
}

type HTTPExecutor struct {
	client *http.Client
}

func NewHTTPExecutor() *HTTPExecutor {
	return &HTTPExecutor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *HTTPExecutor) Name() string {
	return "http"
}

type HTTPPayload struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (e *HTTPExecutor) Execute(ctx context.Context, job *Job) (*Execution, error) {
	var payload HTTPPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("invalid http payload: %w", err)
	}

	if payload.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	method := payload.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, payload.URL, bytes.NewBufferString(payload.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	ex := NewExecution(job.ID, "")
	ex.AddLog(fmt.Sprintf("HTTP %s %s", method, payload.URL))

	resp, err := e.client.Do(req)
	if err != nil {
		ex.SetError(err.Error())
		return ex, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	ex.AddLog(fmt.Sprintf("Status: %d %s", resp.StatusCode, resp.Status))
	ex.AddLog(fmt.Sprintf("Response body: %s", string(body)))

	finished := time.Now().UTC()
	ex.FinishedAt = &finished

	if resp.StatusCode >= 400 {
		err := fmt.Errorf("HTTP %d", resp.StatusCode)
		ex.SetError(err.Error())
		return ex, err
	}

	return ex, nil
}
