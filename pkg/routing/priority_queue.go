package routing

import (
	"container/heap"
	"sync"
	"time"
)

// PriorityQueue manages priority-based route selection with aging
type PriorityQueue struct {
	entries     *priorityHeap
	agingFactor float64
	mu          sync.RWMutex
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue(agingFactor float64) *PriorityQueue {
	pq := &PriorityQueue{
		entries:     &priorityHeap{},
		agingFactor: agingFactor,
	}
	heap.Init(pq.entries)
	return pq
}

// Enqueue adds a route to the queue
func (pq *PriorityQueue) Enqueue(route *Route) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	entry := &QueueEntry{
		Route:      route,
		EnqueuedAt: time.Now(),
	}

	heap.Push(pq.entries, entry)
}

// Dequeue removes and returns the highest priority route
func (pq *PriorityQueue) Dequeue() *Route {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.entries.Len() == 0 {
		return nil
	}

	entry := heap.Pop(pq.entries).(*QueueEntry)
	return entry.Route
}

// Peek returns the highest priority route without removing it
func (pq *PriorityQueue) Peek() *Route {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if pq.entries.Len() == 0 {
		return nil
	}

	return (*pq.entries)[0].Route
}

// Size returns the number of entries in the queue
func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.entries.Len()
}

// Clear removes all entries from the queue
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.entries = &priorityHeap{}
	heap.Init(pq.entries)
}

// Remove removes a specific route from the queue
func (pq *PriorityQueue) Remove(routeID string) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i := 0; i < pq.entries.Len(); i++ {
		if (*pq.entries)[i].Route.ID == routeID {
			heap.Remove(pq.entries, i)
			return
		}
	}
}

// CalculateEffectivePriority calculates effective priority with aging
func (pq *PriorityQueue) CalculateEffectivePriority(entry *QueueEntry) float64 {
	basePriority := float64(entry.Route.Priority)
	waitTime := time.Since(entry.EnqueuedAt).Seconds()
	return basePriority + (waitTime * pq.agingFactor)
}

// GetQueueDepths returns the number of entries at each priority level
func (pq *PriorityQueue) GetQueueDepths() map[int]int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	depths := make(map[int]int)
	for i := 0; i < pq.entries.Len(); i++ {
		priority := (*pq.entries)[i].Route.Priority
		depths[priority]++
	}
	return depths
}

// priorityHeap implements heap.Interface for priority queue
type priorityHeap []*QueueEntry

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	// Higher effective priority comes first
	now := time.Now()

	// Calculate effective priorities with aging
	waitTimeI := now.Sub(h[i].EnqueuedAt).Seconds()
	waitTimeJ := now.Sub(h[j].EnqueuedAt).Seconds()

	effectivePriorityI := float64(h[i].Route.Priority) + (waitTimeI * 0.1)
	effectivePriorityJ := float64(h[j].Route.Priority) + (waitTimeJ * 0.1)

	return effectivePriorityI > effectivePriorityJ
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *priorityHeap) Push(x interface{}) {
	*h = append(*h, x.(*QueueEntry))
}

func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	*h = old[0 : n-1]
	return entry
}
