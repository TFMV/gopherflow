package core

import (
	"context"
	"log"
	"sync"
	"time"
)

type SchedulerConfig struct {
	MaxRetries     int
	RetryBaseDelay time.Duration
}

func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MaxRetries:     3,
		RetryBaseDelay: 2 * time.Second,
	}
}

type Scheduler struct {
	queue     *PriorityQueue
	jobStore  JobStore
	dlq       *PriorityQueue
	handlers  map[string]JobHandler
	handlerMu sync.RWMutex
	config    SchedulerConfig
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewScheduler(queue *PriorityQueue, jobStore JobStore) *Scheduler {
	return &Scheduler{
		queue:    queue,
		jobStore: jobStore,
		dlq:      NewPriorityQueue(),
		handlers: make(map[string]JobHandler),
		config:   DefaultSchedulerConfig(),
		stopCh:   make(chan struct{}),
	}
}

func (s *Scheduler) RegisterHandler(jobType string, handler JobHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.handlers[jobType] = handler
}

func (s *Scheduler) Submit(job *Job) error {
	s.jobStore.Put(job)
	s.queue.Enqueue(job)
	return nil
}

func (s *Scheduler) SubmitWithID(job *Job) error {
	s.jobStore.Put(job)
	s.queue.Enqueue(job)
	return nil
}

func (s *Scheduler) GetJob(id string) (*Job, bool) {
	return s.jobStore.Get(id)
}

func (s *Scheduler) ListJobs() []*Job {
	return s.jobStore.List()
}

func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.dispatchLoop(ctx)

	s.wg.Add(1)
	go s.retryLoop(ctx)
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) dispatchLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			job, ok := s.queue.DequeueWithTimeout(100 * time.Millisecond)
			if !ok || job == nil {
				continue
			}

			s.executeJob(ctx, job)
		}
	}
}

func (s *Scheduler) executeJob(ctx context.Context, job *Job) {
	s.handlerMu.RLock()
	handler, ok := s.handlers[job.Type]
	s.handlerMu.RUnlock()

	if !ok {
		log.Printf("No handler registered for job type: %s", job.Type)
		job.Status = JobStatusFailed
		now := time.Now().UTC()
		job.FinishedAt = &now
		s.jobStore.Put(job)
		return
	}

	job.Status = JobStatusRunning
	now := time.Now().UTC()
	job.StartedAt = &now
	s.jobStore.Put(job)

	err := handler(ctx, job)

	if err != nil {
		s.handleJobFailure(ctx, job, err)
	} else {
		job.Status = JobStatusSucceeded
		now := time.Now().UTC()
		job.FinishedAt = &now
		s.jobStore.Put(job)
		log.Printf("Job %s completed successfully", job.ID)
	}
}

func (s *Scheduler) handleJobFailure(ctx context.Context, job *Job, err error) {
	job.RetryCount++

	if job.CanRetry() {
		job.Status = JobStatusRetrying
		backoff := job.BackoffDuration()
		nextRetry := time.Now().UTC().Add(backoff)
		job.NextRetryAt = &nextRetry
		s.jobStore.Put(job)
		log.Printf("Job %s failed, scheduling retry %d/%d after %v: %v",
			job.ID, job.RetryCount, job.MaxRetries, backoff, err)
	} else {
		job.Status = JobStatusDead
		now := time.Now().UTC()
		job.FinishedAt = &now
		s.jobStore.Put(job)
		s.dlq.Enqueue(job)
		log.Printf("Job %s moved to DLQ after %d retries: %v", job.ID, job.MaxRetries, err)
	}
}

func (s *Scheduler) retryLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processRetries(ctx)
		}
	}
}

func (s *Scheduler) processRetries(ctx context.Context) {
	jobs := s.jobStore.List()
	now := time.Now().UTC()

	for _, job := range jobs {
		if job.Status != JobStatusRetrying {
			continue
		}

		if job.NextRetryAt != nil && now.After(*job.NextRetryAt) {
			job.Status = JobStatusQueued
			job.NextRetryAt = nil
			s.jobStore.Put(job)
			s.queue.Enqueue(job)
			log.Printf("Requeued job %s for retry", job.ID)
		}
	}
}

func (s *Scheduler) DLQLength() int {
	return s.dlq.Len()
}

func (s *Scheduler) GetDLQJobs() []*Job {
	jobs := s.dlq.Len()
	result := make([]*Job, 0, jobs)
	for i := 0; i < jobs; i++ {
		job := s.dlq.Dequeue()
		result = append(result, job)
		s.dlq.Enqueue(job)
	}
	return result
}
