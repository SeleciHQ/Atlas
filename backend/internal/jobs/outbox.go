package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Event is one transactional-outbox row.
type Event struct {
	ID             string
	OrganizationID string
	EventType      string
	AggregateType  string
	AggregateID    string
	Payload        json.RawMessage
	DedupeKey      *string
	Attempts       int
	PublishedAt    *time.Time
	CreatedAt      time.Time
}

// EnqueueEvent appends an outbox row using exec. Pass the transaction that
// performs the business change so both commit atomically. A repeated dedupe key
// for the organization is ignored.
func EnqueueEvent(ctx context.Context, exec Querier, event Event) error {
	if event.OrganizationID == "" || event.EventType == "" || event.AggregateType == "" || event.AggregateID == "" {
		return fmt.Errorf("outbox event requires organization, event type, aggregate type and id")
	}
	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return fmt.Errorf("outbox payload is not valid JSON")
	}
	key := any(nil)
	if event.DedupeKey != nil && *event.DedupeKey != "" {
		key = *event.DedupeKey
	}
	_, err := exec.Exec(ctx, `
		INSERT INTO outbox_events (organization_id, event_type, aggregate_type, aggregate_id, payload, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (organization_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`,
		event.OrganizationID, event.EventType, event.AggregateType, event.AggregateID, payload, key)
	if err != nil {
		return fmt.Errorf("enqueue outbox event: %w", err)
	}
	return nil
}

// Dispatch locks unpublished outbox rows and calls publish for each inside one
// transaction, then marks them published. Rows stay unpublished if publish
// fails, so they are retried; dedupe keys keep re-enqueueing safe. The publish
// callback must write through the supplied transaction to stay atomic.
func (q *Queue) Dispatch(ctx context.Context, limit int, publish func(context.Context, Event, Querier) error) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin outbox dispatch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, organization_id, event_type, aggregate_type, aggregate_id,
		payload, dedupe_key, attempts, published_at, created_at
		FROM atlas_claim_outbox($1)`, limit)
	if err != nil {
		return 0, fmt.Errorf("claim outbox events: %w", err)
	}
	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.OrganizationID, &event.EventType, &event.AggregateType,
			&event.AggregateID, &event.Payload, &event.DedupeKey, &event.Attempts, &event.PublishedAt,
			&event.CreatedAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, event)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read outbox events: %w", err)
	}

	dispatched := 0
	for _, event := range events {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, event.OrganizationID); err != nil {
			return dispatched, fmt.Errorf("bind outbox organization: %w", err)
		}
		if err := publish(ctx, event, tx); err != nil {
			return dispatched, fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE outbox_events SET published_at = now(), attempts = attempts + 1
			WHERE id = $1 AND organization_id = $2`, event.ID, event.OrganizationID); err != nil {
			return dispatched, fmt.Errorf("mark outbox event published: %w", err)
		}
		dispatched++
	}
	if err := tx.Commit(ctx); err != nil {
		return dispatched, fmt.Errorf("commit outbox dispatch: %w", err)
	}
	return dispatched, nil
}

// EnqueueFromEvent is the default outbox publisher: each event becomes a job of
// the same type, keyed by the event dedupe key.
func (q *Queue) EnqueueFromEvent(ctx context.Context, exec Querier, event Event) error {
	key := ""
	if event.DedupeKey != nil {
		key = *event.DedupeKey
	}
	_, _, err := q.Enqueue(ctx, exec, NewJob{
		OrganizationID: event.OrganizationID,
		Type:           event.EventType,
		SchemaVersion:  1,
		Payload:        event.Payload,
		IdempotencyKey: key,
	})
	return err
}
