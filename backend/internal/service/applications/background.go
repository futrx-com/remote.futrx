package applications

import (
	"context"
	"log"
	"time"
)

type backgroundBackends struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartBackground restores only running instances whose manifests explicitly
// request background execution. Ordinary request-driven backends stay lazy.
// Composition binds host capabilities before starting this worker.
func (s *Service) StartBackground(ctx context.Context) {
	s.backgroundOnce.Do(func() {
		ctx, cancel := context.WithCancel(ctx)
		s.background = &backgroundBackends{cancel: cancel, done: make(chan struct{})}
		go func() {
			defer close(s.background.done)
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				s.RestoreBackground(ctx)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}
func (s *Service) RestoreBackground(ctx context.Context) {
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		log.Printf("applications: background restore: %v", err)
		return
	}
	for _, i := range instances {
		if ctx.Err() != nil {
			return
		}
		_, app, err := s.load(ctx, i.ID)
		if err != nil || i.Status != StatusRunning || app.Backend == nil || !app.Backend.Background {
			continue
		}
		if err := s.RestoreBackend(ctx, i.ID); err != nil {
			log.Printf("applications: restore background %s: %v", i.ID, err)
		}
	}
}
