package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TFMV/gopherflow/api"
	"github.com/TFMV/gopherflow/core"
	"github.com/TFMV/gopherflow/tui"
	tea "github.com/charmbracelet/bubbletea"
)

type Command string

const (
	CmdServer  Command = "server"
	CmdUI      Command = "ui"
	CmdWorker  Command = "worker"
	CmdSubmit  Command = "submit"
	CmdVersion Command = "version"
)

var (
	flagPort      = flag.String("port", "8080", "API server port")
	flagWorkers   = flag.Int("workers", 3, "number of workers")
	flagServerURL = flag.String("server", "http://localhost:8080", "server URL for UI/submit")
	flagType      = flag.String("type", "echo", "job type (echo, shell, http)")
	flagPayload   = flag.String("payload", "{}", "job payload JSON")
	flagPriority  = flag.Int("priority", 1, "job priority")
	flagRetries   = flag.Int("retries", 3, "max retries")
	flagVersion   = flag.Bool("version", false, "show version")
)

const version = "0.1.0"

func main() {
	flag.Parse()

	if *flagVersion {
		fmt.Printf("GopherFlow %s\n", version)
		os.Exit(0)
	}

	mode := Command(flag.Arg(0))
	if mode == "" {
		mode = CmdServer
	}

	switch mode {
	case CmdServer:
		runServer()
	case CmdUI:
		runUI()
	case CmdWorker:
		runWorker()
	case CmdSubmit:
		runSubmit()
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		fmt.Printf("Available modes: server, ui, worker, submit\n")
		os.Exit(1)
	}
}

func runServer() {
	queue := core.NewPriorityQueue()
	jobStore := core.NewInMemoryJobStore()
	state := core.NewRuntimeState()
	executors := core.NewExecutorRegistry()

	executors.Register(core.NewShellExecutor())
	executors.Register(core.NewEchoExecutor())
	executors.Register(core.NewHTTPExecutor())

	workerPool := core.NewWorkerPool()
	for i := 0; i < *flagWorkers; i++ {
		worker := core.NewWorker(queue, jobStore, executors)
		workerPool.Add(worker)
		state.UpdateWorker(core.WorkerState{
			ID:       worker.ID(),
			Status:   "idle",
			LastSeen: time.Now().UTC(),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerPool.Start(ctx)
	state.Metrics().SetWorkerCount(int64(len(workerPool.Workers())))
	log.Printf("Worker pool started with %d workers", *flagWorkers)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         ":" + *flagPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	handler := api.NewHandler(queue, jobStore)
	handler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(state.Metrics().String()))
	})

	mux.HandleFunc("DELETE /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "job id required"})
			return
		}
		job, ok := jobStore.Get(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "job not found"})
			return
		}
		if job.Status == core.JobStatusRunning || job.Status == core.JobStatusQueued {
			job.Status = core.JobStatusFailed
			jobStore.Put(job)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		type resp struct {
			Jobs    []map[string]any `json:"jobs"`
			Workers []map[string]any `json:"workers"`
			Stats   map[string]any   `json:"stats"`
		}
		jobs := jobStore.List()
		jobMaps := make([]map[string]any, len(jobs))
		for i, job := range jobs {
			jobMaps[i] = map[string]any{
				"id":          job.ID,
				"type":        job.Type,
				"status":      string(job.Status),
				"priority":    job.Priority,
				"age":         time.Since(job.CreatedAt).Round(time.Second).String(),
				"retry_count": job.RetryCount,
				"max_retries": job.MaxRetries,
			}
		}
		workers := state.ListWorkers()
		workerMaps := make([]map[string]any, len(workers))
		for i, w := range workers {
			workerMaps[i] = map[string]any{
				"id":        w.ID,
				"status":    w.Status,
				"active":    w.ActiveJob,
				"last_seen": w.LastSeen.Format(time.RFC3339),
			}
		}
		stats := map[string]any{
			"total_jobs":   len(jobs),
			"queue_length": queue.Len(),
			"worker_count": len(workers),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Jobs:    jobMaps,
			Workers: workerMaps,
			Stats:   stats,
		})
	})

	go func() {
		log.Printf("API server listening on :%s", *flagPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	<-shutdownCh

	log.Println("Shutting down...")
	workerPool.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	log.Println("Shutdown complete")
}

func runUI() {
	p := tea.NewProgram(tui.NewModelWithServer(*flagServerURL), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running UI: %v\n", err)
		os.Exit(1)
	}
}

func runWorker() {
	queue := core.NewPriorityQueue()
	jobStore := core.NewInMemoryJobStore()
	executors := core.NewExecutorRegistry()

	executors.Register(core.NewShellExecutor())
	executors.Register(core.NewEchoExecutor())
	executors.Register(core.NewHTTPExecutor())

	worker := core.NewWorker(queue, jobStore, executors)
	log.Printf("Worker %s starting", worker.ID())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx)

	go func() {
		<-ctx.Done()
		worker.Stop()
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	<-shutdownCh

	log.Println("Worker shutting down")
	cancel()
}

func runSubmit() {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(*flagPayload), &payload); err != nil {
		log.Fatalf("Invalid payload JSON: %v", err)
	}

	job := core.NewJob(*flagType, marshalJSON(payload), *flagPriority, *flagRetries)

	body := bytes.NewBuffer(nil)
	enc := json.NewEncoder(body)
	_ = enc.Encode(map[string]any{
		"type":        job.Type,
		"payload":     payload,
		"priority":    job.Priority,
		"max_retries": job.MaxRetries,
	})

	resp, err := http.Post(*flagServerURL+"/jobs", "application/json", body)
	if err != nil {
		log.Fatalf("Failed to submit job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		log.Fatalf("Job submission failed: %s - %s", resp.Status, respBody)
	}

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)

	fmt.Printf("Job submitted: %s\n", result["id"])
	fmt.Printf("Status: %s\n", result["status"])
}

func marshalJSON(v map[string]any) []byte {
	data, _ := json.Marshal(v)
	return data
}
