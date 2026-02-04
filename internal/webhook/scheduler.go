package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"hooks.dx314.com/internal/db"
)

const (
	// JobInterval is how often background jobs run.
	JobInterval = time.Hour
	// DeadLetterAge is how long before pending webhooks become dead letters.
	DeadLetterAge = 7 * 24 * time.Hour
)

// Scheduler runs background maintenance jobs for webhooks.
type Scheduler struct {
	queries *db.Queries
	onDeadLetter func(count int64) // Callback when webhooks are dead-lettered

	mu       sync.Mutex
	running  bool
	cancelFn context.CancelFunc
}

// NewScheduler creates a new webhook scheduler.
func NewScheduler(queries *db.Queries) *Scheduler {
	return &Scheduler{
		queries: queries,
	}
}

// SetDeadLetterCallback sets a callback to be invoked when webhooks are dead-lettered.
func (s *Scheduler) SetDeadLetterCallback(fn func(count int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDeadLetter = fn
}

// Start begins the background scheduler. Blocks until context is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true

	ctx, s.cancelFn = context.WithCancel(ctx)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// Run immediately on startup
	s.runJobs(ctx)

	ticker := time.NewTicker(JobInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.runJobs(ctx)
		}
	}
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
	}
}

func (s *Scheduler) runJobs(ctx context.Context) {
	slog.Debug("running webhook maintenance jobs")

	// Process dead letters
	s.processDeadLetters(ctx)

	// Run retention cleanup
	s.runCleanup(ctx)
}

// processDeadLetters marks old pending webhooks as dead letters.
func (s *Scheduler) processDeadLetters(ctx context.Context) {
	count, err := s.queries.MarkDeadLetter(ctx)
	if err != nil {
		slog.Error("failed to mark dead letters", "error", err)
		return
	}

	if count > 0 {
		slog.Info("marked webhooks as dead letter", "count", count)

		s.mu.Lock()
		callback := s.onDeadLetter
		s.mu.Unlock()

		if callback != nil {
			callback(count)
		}
	}
}

// runCleanup deletes old webhooks per retention policy.
func (s *Scheduler) runCleanup(ctx context.Context) {
	// Delete old delivered webhooks (7 days)
	delivered, err := s.queries.DeleteDeliveredWebhooks(ctx)
	if err != nil {
		slog.Error("failed to delete delivered webhooks", "error", err)
	} else if delivered > 0 {
		slog.Info("deleted old delivered webhooks", "count", delivered)
	}

	// Delete old failed webhooks (7 days from last attempt)
	failed, err := s.queries.DeleteFailedWebhooks(ctx)
	if err != nil {
		slog.Error("failed to delete failed webhooks", "error", err)
	} else if failed > 0 {
		slog.Info("deleted old failed webhooks", "count", failed)
	}

	// Delete old dead letter webhooks (14 days)
	deadLetter, err := s.queries.DeleteDeadLetterWebhooks(ctx)
	if err != nil {
		slog.Error("failed to delete dead letter webhooks", "error", err)
	} else if deadLetter > 0 {
		slog.Info("deleted old dead letter webhooks", "count", deadLetter)
	}
}
