package core

import (
	"container/heap"
	"sync"
	"time"
)

type jobHeapItem struct {
	job   *Job
	index int
}

type JobHeap []*jobHeapItem

func (h JobHeap) Len() int { return len(h) }

func (h JobHeap) Less(i, j int) bool {
	if h[i].job.Priority != h[j].job.Priority {
		return h[i].job.Priority > h[j].job.Priority
	}
	return h[i].job.CreatedAt.Before(h[j].job.CreatedAt)
}

func (h JobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *JobHeap) Push(x any) {
	item := x.(*jobHeapItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *JobHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return item
}

type PriorityQueue struct {
	heap *JobHeap
	mu   sync.RWMutex
	cond *sync.Cond
}

func NewPriorityQueue() *PriorityQueue {
	h := make(JobHeap, 0)
	heap.Init(&h)
	pq := &PriorityQueue{
		heap: &h,
	}
	pq.cond = sync.NewCond(&pq.mu)
	return pq
}

func (pq *PriorityQueue) Enqueue(job *Job) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	heap.Push(pq.heap, &jobHeapItem{job: job, index: len(*pq.heap)})
	pq.cond.Signal()
}

func (pq *PriorityQueue) Dequeue() *Job {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for pq.heap.Len() == 0 {
		pq.cond.Wait()
	}

	item := heap.Pop(pq.heap).(*jobHeapItem)
	return item.job
}

func (pq *PriorityQueue) DequeueWithTimeout(timeout time.Duration) (*Job, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.heap.Len() == 0 {
		pq.cond.Wait()
	}

	if pq.heap.Len() == 0 {
		return nil, false
	}

	item := heap.Pop(pq.heap).(*jobHeapItem)
	return item.job, true
}

func (pq *PriorityQueue) Peek() *Job {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if pq.heap.Len() == 0 {
		return nil
	}
	return (*pq.heap)[0].job
}

func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.heap.Len()
}

func (pq *PriorityQueue) GetByID(id string) *Job {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	for _, item := range *pq.heap {
		if item.job.ID == id {
			return item.job
		}
	}
	return nil
}

func (pq *PriorityQueue) Remove(id string) *Job {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, item := range *pq.heap {
		if item.job.ID == id {
			heap.Remove(pq.heap, i)
			return item.job
		}
	}
	return nil
}
