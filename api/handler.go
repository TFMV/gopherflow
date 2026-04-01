package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/TFMV/gopherflow/core"
)

type CreateJobRequest struct {
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	Priority   int            `json:"priority"`
	MaxRetries int            `json:"max_retries"`
}

type CreateJobResponse struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type Handler struct {
	queue    *core.PriorityQueue
	jobStore core.JobStore
}

func NewHandler(queue *core.PriorityQueue, jobStore core.JobStore) *Handler {
	return &Handler{queue: queue, jobStore: jobStore}
}

func encodeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Log error but can't do much at this point
		_ = err
	}
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		encodeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	payload, _ := json.Marshal(req.Payload)
	job := core.NewJob(req.Type, payload, req.Priority, req.MaxRetries)

	h.jobStore.Put(job)
	h.queue.Enqueue(job)

	encodeJSON(w, http.StatusCreated, CreateJobResponse{
		ID:     job.ID,
		Type:   job.Type,
		Status: string(job.Status),
	})
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		encodeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "job id required"})
		return
	}

	job, ok := h.jobStore.Get(id)
	if !ok {
		encodeJSON(w, http.StatusNotFound, ErrorResponse{Error: "job not found"})
		return
	}

	encodeJSON(w, http.StatusOK, job)
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	offset := 0

	if q := r.URL.Query(); q.Has("limit") {
		if n, _ := fmt.Sscanf(q.Get("limit"), "%d", &limit); n > 0 {
			if limit > 100 {
				limit = 100
			}
		}
	}
	if q := r.URL.Query(); q.Has("offset") {
		_, _ = fmt.Sscanf(q.Get("offset"), "%d", &offset)
	}

	allJobs := h.jobStore.List()

	if offset >= len(allJobs) {
		encodeJSON(w, http.StatusOK, []*core.Job{})
		return
	}

	end := offset + limit
	if end > len(allJobs) {
		end = len(allJobs)
	}

	encodeJSON(w, http.StatusOK, allJobs[offset:end])
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /jobs", h.CreateJob)
	mux.HandleFunc("GET /jobs/{id}", h.GetJob)
	mux.HandleFunc("GET /jobs", h.ListJobs)
}
