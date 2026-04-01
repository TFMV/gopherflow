package core

import (
	"encoding/json"
	"testing"
	"time"
)

func BenchmarkPriorityQueue_Enqueue(b *testing.B) {
	q := NewPriorityQueue()
	jobs := make([]*Job, b.N)
	for i := 0; i < b.N; i++ {
		jobs[i] = NewJob("test", json.RawMessage(`{}`), i%10, 3)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(jobs[i])
	}
}

func BenchmarkPriorityQueue_Dequeue(b *testing.B) {
	q := NewPriorityQueue()
	for i := 0; i < b.N; i++ {
		q.Enqueue(NewJob("test", json.RawMessage(`{}`), i%10, 3))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.DequeueWithTimeout(time.Millisecond)
	}
}

func BenchmarkPriorityQueue_EnqueueDequeue(b *testing.B) {
	q := NewPriorityQueue()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job := NewJob("test", json.RawMessage(`{}`), i%10, 3)
		q.Enqueue(job)
		q.DequeueWithTimeout(time.Millisecond)
	}
}

func BenchmarkJobStore_Put(b *testing.B) {
	s := NewInMemoryJobStore()
	jobs := make([]*Job, b.N)
	for i := 0; i < b.N; i++ {
		jobs[i] = NewJob("test", json.RawMessage(`{}`), 1, 3)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Put(jobs[i])
	}
}

func BenchmarkJobStore_Get(b *testing.B) {
	s := NewInMemoryJobStore()
	for i := 0; i < 1000; i++ {
		s.Put(NewJob("test", json.RawMessage(`{}`), 1, 3))
	}

	ids := make([]string, 1000)
	list := s.List()
	for i, job := range list {
		ids[i] = job.ID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get(ids[i%1000])
	}
}

func BenchmarkJobStore_List(b *testing.B) {
	s := NewInMemoryJobStore()
	for i := 0; i < 1000; i++ {
		s.Put(NewJob("test", json.RawMessage(`{}`), 1, 3))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.List()
	}
}

func BenchmarkRuntimeState_UpdateJob(b *testing.B) {
	s := NewRuntimeState()
	for i := 0; i < 100; i++ {
		s.AddJob(NewJob("test", json.RawMessage(`{}`), 1, 3))
	}

	list := s.ListJobs()
	job := list[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job.Status = JobStatusSucceeded
		s.UpdateJob(job)
	}
}

func BenchmarkRuntimeState_ListJobs(b *testing.B) {
	s := NewRuntimeState()
	for i := 0; i < 1000; i++ {
		s.AddJob(NewJob("test", json.RawMessage(`{}`), 1, 3))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ListJobs()
	}
}

func BenchmarkMetrics_IncJobsTotal(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IncJobsTotal("queued")
	}
}
