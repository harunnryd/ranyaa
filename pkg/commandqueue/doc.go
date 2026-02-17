// Package commandqueue provides lane-based task execution with FIFO ordering per lane.
//
// Invariants:
// - Tasks in the same lane execute in FIFO order.
// - Tasks in different lanes may execute concurrently.
// - Queue activity is observable through enqueued/completed events and metrics.
//
// Usage:
//
//	queue := commandqueue.New()
//	defer queue.Close()
//	result, err := queue.Enqueue("session:abc", func(ctx context.Context) (interface{}, error) {
//		return "ok", nil
//	}, nil)
package commandqueue
