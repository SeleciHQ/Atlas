// Package jobs implements Atlas's durable PostgreSQL job queue: transactional
// enqueue, leased claims, retries with backoff, dead-lettering and the
// transactional outbox dispatcher.
//
// Execution is at-least-once. Handlers must be idempotent using the job
// idempotency key and type-specific natural keys.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status is the job lifecycle state.
type Status string

const (
	StatusPending        Status = "pending"
	StatusRunning        Status = "running"
	StatusRetryScheduled Status = "retry_scheduled"
	StatusCompleted      Status = "completed"
	StatusFailed         Status = "failed"
	StatusDeadLetter     Status = "dead_letter"
)

// DefaultMaxAttempts is used when an enqueue omits a maximum.
const DefaultMaxAttempts = 5

// Querier is satisfied by pgxpool.Pool and pgx.Tx. Enqueue accepts a
// transaction so business writes and their jobs/outbox rows commit atomically.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

// Job is one durable unit of asynchronous work.
type Job struct {
	ID             string
	OrganizationID string
	Type           string
	SchemaVersion  int
	Payload        json.RawMessage
	Status         Status
	Priority       int
	RunAt          time.Time
	Attempts       int
	MaxAttempts    int
	IdempotencyKey *string
	LockedBy       *string
	LeaseExpiresAt *time.Time
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
	DeadLetteredAt *time.Time
}

// NewJob describes a job to enqueue.
type NewJob struct {
	OrganizationID string
	Type           string
	SchemaVersion  int
	Payload        json.RawMessage
	Priority       int
	RunAt          time.Time
	MaxAttempts    int
	IdempotencyKey string
}

// Queue is the PostgreSQL-backed queue implementation. The zero value is not
// usable; construct it with New.
type Queue struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewQueue constructs a queue on pool.
func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool, now: time.Now}
}

const jobColumns = `id, organization_id, type, schema_version, payload, status, priority,
	run_at, attempts, max_attempts, idempotency_key, locked_by, lease_expires_at, last_error,
	created_at, updated_at, completed_at, dead_lettered_at`

func scanJob(row pgx.Row) (*Job, error) {
	var job Job
	var rawStatus string
	err := row.Scan(
		&job.ID, &job.OrganizationID, &job.Type, &job.SchemaVersion, &job.Payload, &rawStatus,
		&job.Priority, &job.RunAt, &job.Attempts, &job.MaxAttempts, &job.IdempotencyKey,
		&job.LockedBy, &job.LeaseExpiresAt, &job.LastError, &job.CreatedAt, &job.UpdatedAt,
		&job.CompletedAt, &job.DeadLetteredAt,
	)
	if err != nil {
		return nil, err
	}
	job.Status = Status(rawStatus)
	return &job, nil
}

// Enqueue inserts a job. With a non-empty idempotency key a duplicate returns
// the existing job and created=false. Pass a transaction to commit the job with
// a business change (the outbox does this).
func (q *Queue) Enqueue(ctx context.Context, exec Querier, in NewJob) (*Job, bool, error) {
	if in.OrganizationID == "" || in.Type == "" {
		return nil, false, fmt.Errorf("job requires organization and type")
	}
	if in.SchemaVersion <= 0 {
		return nil, false, fmt.Errorf("job requires a positive schema version")
	}
	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return nil, false, fmt.Errorf("job payload is not valid JSON")
	}
	maxAttempts := in.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	runAt := in.RunAt
	if runAt.IsZero() {
		runAt = q.now().UTC()
	}
	idempotencyKey := any(nil)
	if in.IdempotencyKey != "" {
		idempotencyKey = in.IdempotencyKey
	}

	row := exec.QueryRow(ctx, `
		INSERT INTO jobs (organization_id, type, schema_version, payload, priority, run_at,
		                  max_attempts, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, type, idempotency_key)
			WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING `+jobColumns,
		in.OrganizationID, in.Type, in.SchemaVersion, payload, in.Priority, runAt, maxAttempts, idempotencyKey)
	job, err := scanJob(row)
	if err == nil {
		return job, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("enqueue job: %w", err)
	}
	existing, err := scanJob(exec.QueryRow(ctx, `
		SELECT `+jobColumns+` FROM jobs
		WHERE organization_id = $1 AND type = $2 AND idempotency_key = $3`,
		in.OrganizationID, in.Type, in.IdempotencyKey))
	if err != nil {
		return nil, false, fmt.Errorf("load idempotent job: %w", err)
	}
	return existing, false, nil
}

// Claim atomically leases the next runnable job and records an attempt. It
// returns nil when no job is runnable.
func (q *Queue) Claim(ctx context.Context, workerID string, lease time.Duration) (*Job, error) {
	seconds := int(lease / time.Second)
	if seconds <= 0 {
		seconds = 30
	}
	job, err := scanJob(q.pool.QueryRow(ctx, `SELECT * FROM atlas_claim_job($1, $2)`, workerID, seconds))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

// Complete records a successful result and closes the attempt.
func (q *Queue) Complete(ctx context.Context, job *Job, result json.RawMessage) error {
	if len(result) == 0 {
		result = json.RawMessage(`{}`)
	}
	if !json.Valid(result) {
		return fmt.Errorf("job result is not valid JSON")
	}
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin complete job: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, job.OrganizationID); err != nil {
		return fmt.Errorf("bind job organization: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE jobs SET status = 'completed', completed_at = now(), updated_at = now(),
		                locked_by = NULL, lease_expires_at = NULL
		WHERE id = $1 AND organization_id = $2 AND status = 'running' AND locked_by = $3`,
		job.ID, job.OrganizationID, deref(job.LockedBy))
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("complete job: lease is no longer held")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_results (job_id, organization_id, schema_version, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO UPDATE SET payload = EXCLUDED.payload`, job.ID, job.OrganizationID, job.SchemaVersion, result); err != nil {
		return fmt.Errorf("store job result: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_attempts SET outcome = 'completed', finished_at = now()
		WHERE job_id = $1 AND attempt = $2 AND finished_at IS NULL`, job.ID, job.Attempts); err != nil {
		return fmt.Errorf("close job attempt: %w", err)
	}
	return tx.Commit(ctx)
}

// Fail records a failed attempt. Retryable failures return to the queue with
// capped exponential backoff; exhausted or non-retryable failures go to the
// dead-letter state.
func (q *Queue) Fail(ctx context.Context, job *Job, cause error, retryable bool) error {
	message := "unknown error"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	deadLetter := !retryable || job.Attempts >= job.MaxAttempts
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fail job: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, job.OrganizationID); err != nil {
		return fmt.Errorf("bind job organization: %w", err)
	}
	var tag pgconn.CommandTag
	if deadLetter {
		tag, err = tx.Exec(ctx, `
			UPDATE jobs SET status = 'dead_letter', last_error = $3, updated_at = now(),
			                dead_lettered_at = now(), locked_by = NULL, lease_expires_at = NULL
			WHERE id = $1 AND organization_id = $2 AND status = 'running' AND locked_by = $4`,
			job.ID, job.OrganizationID, message, deref(job.LockedBy))
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE jobs SET status = 'retry_scheduled', last_error = $3, run_at = $4, updated_at = now(),
			                locked_by = NULL, lease_expires_at = NULL
			WHERE id = $1 AND organization_id = $2 AND status = 'running' AND locked_by = $5`,
			job.ID, job.OrganizationID, message, q.now().UTC().Add(Backoff(job.Attempts)), deref(job.LockedBy))
	}
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("fail job: lease is no longer held")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_attempts SET outcome = 'failed', error = $3, finished_at = now()
		WHERE job_id = $1 AND attempt = $2 AND finished_at IS NULL`, job.ID, job.Attempts, message); err != nil {
		return fmt.Errorf("close job attempt: %w", err)
	}
	return tx.Commit(ctx)
}

// ReapExpiredLeases requeues (or dead-letters) jobs whose worker disappeared.
func (q *Queue) ReapExpiredLeases(ctx context.Context) (int, error) {
	var reaped int
	if err := q.pool.QueryRow(ctx, `SELECT atlas_reap_expired_leases()`).Scan(&reaped); err != nil {
		return 0, fmt.Errorf("reap expired leases: %w", err)
	}
	return reaped, nil
}

// Requeue moves a dead-letter job back to pending for operator-driven recovery.
func (q *Queue) Requeue(ctx context.Context, organizationID, jobID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE jobs SET status = 'pending', run_at = now(), attempts = 0, last_error = NULL,
		                dead_lettered_at = NULL, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND status = 'dead_letter'`, jobID, organizationID)
	if err != nil {
		return fmt.Errorf("requeue job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("requeue job: not a dead-letter job")
	}
	return tx.Commit(ctx)
}

// Backoff returns capped exponential backoff with jitter for attempt n (1-based).
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	const base = 2 * time.Second
	const max = 5 * time.Minute
	delay := base
	for i := 1; i < attempt && delay < max; i++ {
		delay *= 2
	}
	if delay > max {
		delay = max
	}
	jitter := time.Duration(0)
	if n, err := rand.Int(rand.Reader, big.NewInt(int64(delay/4)+1)); err == nil {
		jitter = time.Duration(n.Int64())
	}
	return delay + jitter
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
