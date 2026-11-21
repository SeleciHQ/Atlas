-- Phase 3: durable job queue, attempts, results and transactional outbox.
--
-- Execution semantics are at-least-once. Handlers must be idempotent using the
-- job idempotency key (unique per organization + type) and type-specific
-- natural keys.

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'retry_scheduled', 'completed', 'failed', 'dead_letter')),
    priority integer NOT NULL DEFAULT 0,
    run_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    idempotency_key text,
    locked_by text,
    lease_expires_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    dead_lettered_at timestamptz,
    CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 200)
);

CREATE UNIQUE INDEX jobs_idempotency_unique
    ON jobs (organization_id, type, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX jobs_claimable ON jobs (priority DESC, run_at ASC)
    WHERE status IN ('pending', 'retry_scheduled');
CREATE INDEX jobs_organization_status ON jobs (organization_id, status, created_at DESC);

CREATE TABLE job_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt integer NOT NULL CHECK (attempt > 0),
    worker_id text NOT NULL,
    outcome text CHECK (outcome IN ('completed', 'failed', 'lease_expired')),
    error text,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX job_attempts_organization ON job_attempts (organization_id, started_at DESC);
CREATE INDEX job_attempts_job ON job_attempts (job_id, started_at);

CREATE TABLE job_results (
    job_id uuid PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX job_results_organization ON job_results (organization_id, created_at DESC);

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    dedupe_key text,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX outbox_events_dedupe_unique
    ON outbox_events (organization_id, dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX outbox_events_unpublished ON outbox_events (created_at ASC)
    WHERE published_at IS NULL;

ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY jobs_tenant ON jobs FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY job_attempts_tenant ON job_attempts FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY job_results_tenant ON job_results FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY outbox_events_tenant ON outbox_events FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

-- Cross-organization claim. Workers cannot select jobs directly without an
-- organization context, so claiming is a security-definer function owned by
-- the migration role (which does not bypass RLS for runtime roles).
CREATE FUNCTION atlas_claim_job(p_worker text, p_lease_seconds integer)
RETURNS SETOF jobs
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    claimed jobs;
BEGIN
    UPDATE jobs SET
        status = 'running',
        locked_by = p_worker,
        lease_expires_at = now() + make_interval(secs => p_lease_seconds),
        attempts = attempts + 1,
        updated_at = now()
    WHERE id = (
        SELECT id FROM jobs
        WHERE status IN ('pending', 'retry_scheduled') AND run_at <= now()
        ORDER BY priority DESC, run_at ASC
        FOR UPDATE SKIP LOCKED
        LIMIT 1
    )
    RETURNING * INTO claimed;

    IF claimed.id IS NULL THEN
        RETURN;
    END IF;

    INSERT INTO job_attempts (organization_id, job_id, attempt, worker_id)
    VALUES (claimed.organization_id, claimed.id, claimed.attempts, p_worker);

    RETURN NEXT claimed;
END;
$$;

-- Expired leases return to the queue. The claim already counted an attempt, so
-- an exhausted job goes straight to the dead-letter state.
CREATE FUNCTION atlas_reap_expired_leases()
RETURNS integer
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    expired integer;
BEGIN
    UPDATE job_attempts a SET outcome = 'lease_expired', finished_at = now()
    FROM jobs j
    WHERE a.job_id = j.id AND a.attempt = j.attempts AND a.finished_at IS NULL
      AND j.status = 'running' AND j.lease_expires_at < now();

    UPDATE jobs SET
        status = CASE WHEN attempts >= max_attempts THEN 'dead_letter' ELSE 'retry_scheduled' END,
        locked_by = NULL,
        lease_expires_at = NULL,
        last_error = 'lease_expired',
        run_at = now(),
        updated_at = now(),
        dead_lettered_at = CASE WHEN attempts >= max_attempts THEN now() ELSE NULL END
    WHERE status = 'running' AND lease_expires_at < now();
    GET DIAGNOSTICS expired = ROW_COUNT;
    RETURN expired;
END;
$$;

-- Outbox rows are locked by the dispatcher transaction. Dedupe keys make
-- re-enqueueing after a crash harmless (job idempotency is keyed the same way).
CREATE FUNCTION atlas_claim_outbox(p_limit integer)
RETURNS SETOF outbox_events
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT * FROM outbox_events
    WHERE published_at IS NULL
    ORDER BY created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT p_limit
$$;

REVOKE ALL ON FUNCTION atlas_claim_job(text, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION atlas_reap_expired_leases() FROM PUBLIC;
REVOKE ALL ON FUNCTION atlas_claim_outbox(integer) FROM PUBLIC;
