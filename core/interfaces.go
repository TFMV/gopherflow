package core

import (
	"context"
	"sync"
)

type JobHandler func(ctx context.Context, job *Job) error

type Queue interface {
	Enqueue(job *Job) error
	Dequeue() *Job
	DequeueWithTimeout(timeout string) (*Job, bool)
	Peek() *Job
	Len() int
	GetByID(id string) *Job
	Remove(id string) *Job
}

type JobStore interface {
	Get(id string) (*Job, bool)
	Put(job *Job)
	List() []*Job
	Delete(id string) bool
}

type inMemoryJobStore struct {
	jobs map[string]*Job
	mu   sync.RWMutex
}

func NewInMemoryJobStore() JobStore {
	return &inMemoryJobStore{
		jobs: make(map[string]*Job),
	}
}

func (s *inMemoryJobStore) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *inMemoryJobStore) Put(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *inMemoryJobStore) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (s *inMemoryJobStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[id]; ok {
		delete(s.jobs, id)
		return true
	}
	return false
}
