package core

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type Worker struct {
	id          string
	queue       *PriorityQueue
	jobStore    JobStore
	executors   *ExecutorRegistry
	stopCh      chan struct{}
	wg          sync.WaitGroup
	activeJob   *Job
	activeJobMu sync.Mutex
	status      string
	heartbeatCh chan Heartbeat
}

func NewWorker(queue *PriorityQueue, jobStore JobStore, executors *ExecutorRegistry) *Worker {
	return &Worker{
		id:          ulid.Make().String(),
		queue:       queue,
		jobStore:    jobStore,
		executors:   executors,
		stopCh:      make(chan struct{}),
		status:      "idle",
		heartbeatCh: make(chan Heartbeat, 1),
	}
}

func (w *Worker) ID() string {
	return w.id
}

func (w *Worker) Status() string {
	return w.status
}

func (w *Worker) HeartbeatCh() chan Heartbeat {
	return w.heartbeatCh
}

func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
}

func (w *Worker) run(ctx context.Context) {
	log.Printf("Worker %s started", w.id)
	defer log.Printf("Worker %s stopped", w.id)

	heartbeatTicker := time.NewTicker(5 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.waitForActiveJob(ctx)
			return
		case <-w.stopCh:
			w.waitForActiveJob(context.Background())
			return
		case <-heartbeatTicker.C:
			w.sendHeartbeat()
		default:
			w.processNextJob(ctx)
		}
	}
}

func (w *Worker) processNextJob(ctx context.Context) {
	job, ok := w.queue.DequeueWithTimeout(100 * time.Millisecond)
	if !ok || job == nil {
		return
	}

	job.Status = JobStatusRunning
	now := time.Now().UTC()
	job.StartedAt = &now
	w.jobStore.Put(job)

	w.activeJobMu.Lock()
	w.activeJob = job
	w.status = "running"
	w.activeJobMu.Unlock()

	w.executeJob(ctx, job)

	w.activeJobMu.Lock()
	w.activeJob = nil
	w.status = "idle"
	w.activeJobMu.Unlock()
}

func (w *Worker) executeJob(ctx context.Context, job *Job) {
	log.Printf("Worker %s executing job %s (type: %s)", w.id, job.ID, job.Type)

	executor, ok := w.executors.Get(job.Type)
	if !ok {
		log.Printf("No executor found for job type: %s", job.Type)
		job.Status = JobStatusFailed
		now := time.Now().UTC()
		job.FinishedAt = &now
		w.jobStore.Put(job)
		return
	}

	execution, err := executor.Execute(ctx, job)
	if err != nil {
		log.Printf("Job %s failed: %v", job.ID, err)
		job.Status = JobStatusFailed
		now := time.Now().UTC()
		job.FinishedAt = &now
		w.jobStore.Put(job)
		return
	}

	if execution.Error != nil {
		job.Status = JobStatusFailed
	} else {
		job.Status = JobStatusSucceeded
	}

	now := time.Now().UTC()
	job.FinishedAt = &now
	w.jobStore.Put(job)

	log.Printf("Job %s completed with status: %s", job.ID, job.Status)
}

func (w *Worker) waitForActiveJob(ctx context.Context) {
	w.activeJobMu.Lock()
	job := w.activeJob
	w.activeJobMu.Unlock()

	if job == nil {
		return
	}

	log.Printf("Worker %s waiting for active job %s to complete", w.id, job.ID)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %s context cancelled, abandoning job %s", w.id, job.ID)
			return
		case <-ticker.C:
			w.activeJobMu.Lock()
			if w.activeJob == nil {
				w.activeJobMu.Unlock()
				log.Printf("Worker %s active job completed", w.id)
				return
			}
			w.activeJobMu.Unlock()
		}
	}
}

func (w *Worker) sendHeartbeat() {
	w.activeJobMu.Lock()
	activeCount := 0
	if w.activeJob != nil {
		activeCount = 1
	}
	w.activeJobMu.Unlock()

	hb := Heartbeat{
		WorkerID:   w.id,
		Timestamp:  time.Now().UTC(),
		ActiveJobs: activeCount,
		Status:     w.status,
	}

	select {
	case w.heartbeatCh <- hb:
	default:
	}
}

func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

type Heartbeat struct {
	WorkerID   string    `json:"worker_id"`
	Timestamp  time.Time `json:"timestamp"`
	ActiveJobs int       `json:"active_jobs"`
	Status     string    `json:"status"`
}

type WorkerPool struct {
	workers    []*Worker
	mu         sync.RWMutex
	heartbeats map[string]chan Heartbeat
}

func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		workers:    make([]*Worker, 0),
		heartbeats: make(map[string]chan Heartbeat),
	}
}

func (p *WorkerPool) Add(worker *Worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.workers = append(p.workers, worker)
	p.heartbeats[worker.ID()] = worker.HeartbeatCh()
}

func (p *WorkerPool) Workers() []*Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	workers := make([]*Worker, len(p.workers))
	copy(workers, p.workers)
	return workers
}

func (p *WorkerPool) GetHeartbeat(workerID string) (chan Heartbeat, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ch, ok := p.heartbeats[workerID]
	return ch, ok
}

func (p *WorkerPool) Start(ctx context.Context) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, w := range p.workers {
		w.Start(ctx)
	}
}

func (p *WorkerPool) Stop() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, w := range p.workers {
		w.Stop()
	}
}
