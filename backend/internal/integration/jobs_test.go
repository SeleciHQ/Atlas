package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/jobs"
	"atlas/internal/tenancy"
)

func seedQueueOrganization(t *testing.T, admin *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := admin.QueryRow(context.Background(),
		`INSERT INTO organizations (name) VALUES ('Queue Test Org') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	return id
}

func enqueueTestJob(t *testing.T, workerPool *pgxpool.Pool, organizationID, key string, maxAttempts int) *jobs.Job {
	t.Helper()
	queue := jobs.NewQueue(workerPool)
	var job *jobs.Job
	err := tenancy.WithOrganizationTx(context.Background(), workerPool, organizationID, func(tx pgx.Tx) error {
		var err error
		job, _, err = queue.Enqueue(context.Background(), tx, jobs.NewJob{
			OrganizationID: organizationID,
			Type:           "system.noop",
			SchemaVersion:  1,
			IdempotencyKey: key,
			MaxAttempts:    maxAttempts,
		})
		return err
	})
	if err != nil {
		t.Fatalf("enqueue job: %v", err)
	}
	return job
}

func TestJobQueueLifecycle(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	workerPool := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	queue := jobs.NewQueue(workerPool)

	first := enqueueTestJob(t, workerPool, organizationID, "lifecycle-1", 3)
	duplicate := enqueueTestJob(t, workerPool, organizationID, "lifecycle-1", 3)
	if duplicate.ID != first.ID {
		t.Fatalf("duplicate idempotency key produced a new job: %s != %s", duplicate.ID, first.ID)
	}
	second := enqueueTestJob(t, workerPool, organizationID, "lifecycle-2", 3)

	claimed, err := queue.Claim(context.Background(), "worker-a", 30*time.Second)
	if err != nil || claimed == nil || claimed.ID != first.ID {
		t.Fatalf("first claim = %+v, %v", claimed, err)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("attempts after claim = %d, want 1", claimed.Attempts)
	}
	claimed2, err := queue.Claim(context.Background(), "worker-b", 30*time.Second)
	if err != nil || claimed2 == nil || claimed2.ID != second.ID {
		t.Fatalf("second claim = %+v, %v", claimed2, err)
	}
	if extra, err := queue.Claim(context.Background(), "worker-c", 30*time.Second); err != nil || extra != nil {
		t.Fatalf("empty claim = %+v, %v", extra, err)
	}

	if err := queue.Complete(context.Background(), claimed, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	var status string
	var result []byte
	if err := admin.QueryRow(context.Background(), `
		SELECT j.status, r.payload FROM jobs j JOIN job_results r ON r.job_id = j.id WHERE j.id = $1`,
		claimed.ID).Scan(&status, &result); err != nil {
		t.Fatalf("read completed job: %v", err)
	}
	if status != "completed" || string(result) != `{"ok": true}` {
		t.Fatalf("completed job status=%q result=%s", status, result)
	}

	if err := queue.Fail(context.Background(), claimed2, errors.New("transient"), true); err != nil {
		t.Fatalf("fail retryable: %v", err)
	}
	var runAt time.Time
	if err := admin.QueryRow(context.Background(), `SELECT status, run_at FROM jobs WHERE id = $1`, claimed2.ID).
		Scan(&status, &runAt); err != nil {
		t.Fatalf("read retry job: %v", err)
	}
	if status != "retry_scheduled" || !runAt.After(time.Now()) {
		t.Fatalf("retry state status=%q run_at=%s", status, runAt)
	}

	if _, err := admin.Exec(context.Background(), `UPDATE jobs SET run_at = now() WHERE id = $1`, claimed2.ID); err != nil {
		t.Fatalf("make retry runnable: %v", err)
	}
	claimed3, err := queue.Claim(context.Background(), "worker-d", 30*time.Second)
	if err != nil || claimed3 == nil || claimed3.Attempts != 2 {
		t.Fatalf("retry claim = %+v, %v", claimed3, err)
	}
	if err := queue.Fail(context.Background(), claimed3, errors.New("permanent"), false); err != nil {
		t.Fatalf("fail non-retryable: %v", err)
	}
	if err := admin.QueryRow(context.Background(), `SELECT status FROM jobs WHERE id = $1`, claimed3.ID).Scan(&status); err != nil {
		t.Fatalf("read dead letter: %v", err)
	}
	if status != "dead_letter" {
		t.Fatalf("status = %q, want dead_letter", status)
	}

	if err := queue.Requeue(context.Background(), organizationID, claimed3.ID); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	claimed4, err := queue.Claim(context.Background(), "worker-e", 30*time.Second)
	if err != nil || claimed4 == nil || claimed4.ID != claimed3.ID || claimed4.Attempts != 1 {
		t.Fatalf("requeued claim = %+v, %v", claimed4, err)
	}
}

func TestLeaseExpiryRecovery(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	workerPool := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	queue := jobs.NewQueue(workerPool)

	job := enqueueTestJob(t, workerPool, organizationID, "lease-1", 5)
	claimed, err := queue.Claim(context.Background(), "worker-a", 30*time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if _, err := admin.Exec(context.Background(),
		`UPDATE jobs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, claimed.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	reaped, err := queue.ReapExpiredLeases(context.Background())
	if err != nil || reaped != 1 {
		t.Fatalf("reap = %d, %v", reaped, err)
	}
	var status string
	if err := admin.QueryRow(context.Background(), `SELECT status FROM jobs WHERE id = $1`, job.ID).Scan(&status); err != nil {
		t.Fatalf("read reaped job: %v", err)
	}
	if status != "retry_scheduled" {
		t.Fatalf("status after reap = %q", status)
	}
	again, err := queue.Claim(context.Background(), "worker-b", 30*time.Second)
	if err != nil || again == nil || again.ID != job.ID || again.Attempts != 2 {
		t.Fatalf("reclaimed = %+v, %v", again, err)
	}
}

func TestConcurrentClaimsDoNotDuplicate(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	workerPool := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	queue := jobs.NewQueue(workerPool)

	const total = 10
	for i := 0; i < total; i++ {
		enqueueTestJob(t, workerPool, organizationID, fmt.Sprintf("concurrent-%d", i), 5)
	}

	var mutex sync.Mutex
	seen := map[string]int{}
	var wait sync.WaitGroup
	for i := 0; i < total; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			claimed, err := queue.Claim(context.Background(), fmt.Sprintf("worker-%d", i), 30*time.Second)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if claimed == nil {
				return
			}
			mutex.Lock()
			seen[claimed.ID]++
			mutex.Unlock()
		}(i)
	}
	wait.Wait()
	if len(seen) != total {
		t.Fatalf("distinct claims = %d, want %d (%v)", len(seen), total, seen)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("job %s claimed %d times", id, count)
		}
	}
}

func TestOutboxDispatch(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	workerPool := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	queue := jobs.NewQueue(workerPool)
	ctx := context.Background()

	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := jobs.EnqueueEvent(ctx, tx, jobs.Event{
		OrganizationID: organizationID,
		EventType:      "system.noop",
		AggregateType:  "test",
		AggregateID:    "aggregate-1",
		DedupeKey:      ptr("event-key-1"),
	}); err != nil {
		t.Fatalf("enqueue event: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var count int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&count); err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if count != 0 {
		t.Fatalf("outbox row survived rollback: %d", count)
	}

	if _, err := admin.Exec(ctx, `
		INSERT INTO outbox_events (organization_id, event_type, aggregate_type, aggregate_id, dedupe_key)
		VALUES ($1, 'system.noop', 'test', 'aggregate-1', 'event-key-1')`, organizationID); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	failing := func(context.Context, jobs.Event, jobs.Querier) error { return errors.New("publish failed") }
	if _, err := queue.Dispatch(ctx, 10, failing); err == nil {
		t.Fatal("dispatch with failing publisher should error")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count unpublished: %v", err)
	}
	if count != 1 {
		t.Fatalf("unpublished rows = %d, want 1", count)
	}

	dispatched, err := queue.Dispatch(ctx, 10, func(ctx context.Context, event jobs.Event, exec jobs.Querier) error {
		return queue.EnqueueFromEvent(ctx, exec, event)
	})
	if err != nil || dispatched != 1 {
		t.Fatalf("dispatch = %d, %v", dispatched, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count after dispatch: %v", err)
	}
	if count != 0 {
		t.Fatalf("unpublished rows after dispatch = %d", count)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE idempotency_key = 'event-key-1'`).Scan(&count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("jobs for event = %d, want 1", count)
	}
	if dispatched, err := queue.Dispatch(ctx, 10, func(ctx context.Context, event jobs.Event, exec jobs.Querier) error {
		return queue.EnqueueFromEvent(ctx, exec, event)
	}); err != nil || dispatched != 0 {
		t.Fatalf("second dispatch = %d, %v", dispatched, err)
	}
}

func ptr(value string) *string { return &value }
