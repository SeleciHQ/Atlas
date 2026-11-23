// Command atlas-scheduler dispatches transactional-outbox events and recovers
// jobs whose worker leases expired. It is safe to run more than one: both
// operations use row locks.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"atlas/internal/governance"
	"atlas/internal/jobs"
	"atlas/internal/platform/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-scheduler:", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("WORKER_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return fmt.Errorf("WORKER_DATABASE_URL or DATABASE_URL is required")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "atlas-scheduler")
	outboxPoll := envDuration("OUTBOX_POLL_MS", 2000, time.Millisecond)
	reapInterval := envDuration("LEASE_REAP_MS", 30000, time.Millisecond)
	expiryInterval := envDuration("APPROVAL_EXPIRY_MS", 60000, time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, databaseURL, 4)
	if err != nil {
		return err
	}
	defer pool.Close()
	queue := jobs.NewQueue(pool)
	governanceService := governance.NewService(pool, queue)

	logger.Info("scheduler_started", "outbox_poll", outboxPoll.String(),
		"reap_interval", reapInterval.String(), "approval_expiry_interval", expiryInterval.String())
	lastReap := time.Time{}
	lastExpiry := time.Time{}
	for {
		select {
		case <-ctx.Done():
			logger.Info("scheduler_stopped")
			return nil
		default:
		}

		dispatched, err := queue.Dispatch(ctx, 50, func(ctx context.Context, event jobs.Event, exec jobs.Querier) error {
			return queue.EnqueueFromEvent(ctx, exec, event)
		})
		if err != nil {
			logger.Error("outbox_dispatch_failed", "error", err.Error())
		} else if dispatched > 0 {
			logger.Info("outbox_dispatched", "count", dispatched)
		}

		if time.Since(lastReap) >= reapInterval {
			lastReap = time.Now()
			if reaped, err := queue.ReapExpiredLeases(ctx); err != nil {
				logger.Warn("lease_reap_failed", "error", err.Error())
			} else if reaped > 0 {
				logger.Info("leases_reaped", "count", reaped)
			}
		}

		if time.Since(lastExpiry) >= expiryInterval {
			lastExpiry = time.Now()
			if expired, err := governanceService.ExpireApprovals(ctx); err != nil {
				logger.Warn("approval_expiry_failed", "error", err.Error())
			} else if expired > 0 {
				logger.Info("approvals_expired", "count", expired)
			}
		}

		if !sleep(ctx, outboxPoll) {
			logger.Info("scheduler_stopped")
			return nil
		}
	}
}

func envDuration(name string, fallback int64, unit time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(fallback) * unit
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return time.Duration(fallback) * unit
	}
	return time.Duration(value) * unit
}

func sleep(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
