package commandqueue

import (
	"context"
	"testing"
	"time"
)

func TestDedupCache_Shutdown(t *testing.T) {
	cache := newDedupCache(context.Background(), 50*time.Millisecond)
	cache.Stop()

	select {
	case <-cache.done:
		// ok
	case <-time.After(1 * time.Second):
		t.Fatalf("dedup cache cleanup did not stop within timeout")
	}
}
