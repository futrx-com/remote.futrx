package quota

import (
	"context"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// planUsageRefresher owns provider fan-out, freshness, and sharing one
// in-flight refresh between callers. Account snapshot ownership remains in
// accountReadings.
type planUsageRefresher struct {
	readings        *accountReadings
	readers         []agent.PlanUsageReader
	refreshInterval time.Duration
	readTimeout     time.Duration
	now             func() time.Time

	mu          sync.Mutex
	refreshedAt time.Time
	refreshing  chan struct{}
}

func newPlanUsageRefresher(
	readings *accountReadings,
	readers []agent.PlanUsageReader,
	refreshInterval time.Duration,
	readTimeout time.Duration,
	now func() time.Time,
) *planUsageRefresher {
	return &planUsageRefresher{
		readings:        readings,
		readers:         readers,
		refreshInterval: refreshInterval,
		readTimeout:     readTimeout,
		now:             now,
	}
}

func (r *planUsageRefresher) refresh(ctx context.Context) {
	if len(r.readers) == 0 {
		return
	}
	r.mu.Lock()
	done := r.refreshing
	if done == nil {
		if !r.refreshedAt.IsZero() && r.now().Sub(r.refreshedAt) < r.refreshInterval {
			r.mu.Unlock()
			return
		}
		done = make(chan struct{})
		r.refreshing = done
		go r.readAll(done)
	}
	r.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// readAll asks every reader at once and files each account's answer.
func (r *planUsageRefresher) readAll(done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), r.readTimeout)
	defer cancel()
	var readers sync.WaitGroup
	for _, reader := range r.readers {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for _, usage := range reader.ReadPlanUsage(ctx) {
				r.readings.replace(context.WithoutCancel(ctx), reader.ID(), usage)
			}
		}()
	}
	readers.Wait()

	r.mu.Lock()
	r.refreshedAt = r.now()
	r.refreshing = nil
	r.mu.Unlock()
	close(done)
}
