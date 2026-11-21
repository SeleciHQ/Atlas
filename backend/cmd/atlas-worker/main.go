// Command atlas-worker claims and executes Atlas jobs from the PostgreSQL
// queue. Execution is at-least-once; handlers must be idempotent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"atlas/internal/connectors"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/platform/db"
	"atlas/internal/secrets"
)

type handler func(context.Context, *jobs.Job) (json.RawMessage, error)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-worker:", err)
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
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		host, _ := os.Hostname()
		workerID = fmt.Sprintf("%s-%d", host, os.Getpid())
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "atlas-worker", "worker_id", workerID)
	lease := envDuration("JOB_LEASE_SECONDS", 60, time.Second)
	poll := envDuration("JOB_POLL_MS", 1000, time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, databaseURL, 8)
	if err != nil {
		return err
	}
	defer pool.Close()
	queue := jobs.NewQueue(pool)

	handlers := map[string]handler{
		"system.noop": func(context.Context, *jobs.Job) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	if rawKey := os.Getenv("ATLAS_SECRET_KEY"); rawKey != "" {
		sealer, err := secrets.NewSealer(rawKey)
		if err != nil {
			return fmt.Errorf("configure secret key: %w", err)
		}
		registry := connectors.NewRegistry()
		registry.Register("mock", connectors.NewMock)
		registry.Register("document_inbox", connectors.NewDocumentInbox)
		ingestionService := ingestion.NewService(pool, registry, sealer)
		handlers["connector.sync"] = func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
			var request struct {
				OrganizationID string `json:"organization_id"`
				IntegrationID  string `json:"integration_id"`
			}
			if err := json.Unmarshal(job.Payload, &request); err != nil {
				return nil, fmt.Errorf("decode connector sync payload: %w", err)
			}
			if request.OrganizationID == "" || request.IntegrationID == "" {
				return nil, fmt.Errorf("connector sync payload requires organization_id and integration_id")
			}
			summary, err := ingestionService.RunSync(ctx, request.OrganizationID, request.IntegrationID, 25)
			if err != nil {
				return nil, err
			}
			return json.Marshal(summary)
		}
	} else {
		logger.Warn("connector_sync_disabled", "detail", "set ATLAS_SECRET_KEY to enable connector jobs")
	}

	logger.Info("worker_started", "lease", lease.String(), "poll", poll.String())
	lastReap := time.Time{}
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker_stopped")
			return nil
		default:
		}

		if time.Since(lastReap) > 30*time.Second {
			lastReap = time.Now()
			if reaped, err := queue.ReapExpiredLeases(ctx); err != nil {
				logger.Warn("lease_reap_failed", "error", err.Error())
			} else if reaped > 0 {
				logger.Info("leases_reaped", "count", reaped)
			}
		}

		job, err := queue.Claim(ctx, workerID, lease)
		if err != nil {
			logger.Error("claim_failed", "error", err.Error())
			if !sleep(ctx, poll) {
				logger.Info("worker_stopped")
				return nil
			}
			continue
		}
		if job == nil {
			if !sleep(ctx, poll) {
				logger.Info("worker_stopped")
				return nil
			}
			continue
		}

		runHandler(ctx, logger, queue, job, handlers, lease)
	}
}

func runHandler(ctx context.Context, logger *slog.Logger, queue *jobs.Queue, job *jobs.Job, handlers map[string]handler, lease time.Duration) {
	started := time.Now()
	handle, ok := handlers[job.Type]
	if !ok {
		logger.Warn("job_type_unknown", "job_id", job.ID, "type", job.Type)
		if err := queue.Fail(ctx, job, fmt.Errorf("no handler registered for job type %q", job.Type), false); err != nil {
			logger.Error("job_fail_record_failed", "job_id", job.ID, "error", err.Error())
		}
		return
	}

	jobCtx, cancel := context.WithTimeout(ctx, lease)
	defer cancel()
	result, err := handle(jobCtx, job)
	if err != nil {
		logger.Warn("job_failed", "job_id", job.ID, "type", job.Type, "attempt", job.Attempts, "error", err.Error())
		if failErr := queue.Fail(ctx, job, err, true); failErr != nil {
			logger.Error("job_fail_record_failed", "job_id", job.ID, "error", failErr.Error())
		}
		return
	}
	if err := queue.Complete(ctx, job, result); err != nil {
		logger.Error("job_complete_failed", "job_id", job.ID, "error", err.Error())
		return
	}
	logger.Info("job_completed", "job_id", job.ID, "type", job.Type, "attempt", job.Attempts,
		"duration_ms", time.Since(started).Milliseconds())
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
