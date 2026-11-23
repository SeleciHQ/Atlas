-- Phase 8: sweep expired pending approvals across every organization.
-- The function is security-definer so the scheduler's worker role can expire
-- approvals without an organization context; each expiry is audited.

CREATE INDEX approvals_expiry ON approvals (expires_at)
    WHERE status = 'pending' AND expires_at IS NOT NULL;

CREATE FUNCTION atlas_expire_approvals()
RETURNS integer
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    expired integer;
BEGIN
    WITH candidates AS (
        UPDATE approvals
        SET status = 'expired', decided_at = now(), decision_reason = 'approval expired'
        WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at <= now()
        RETURNING organization_id, id
    ), logged AS (
        INSERT INTO audit_events
            (organization_id, actor_kind, action, resource_type, resource_id, decision, metadata)
        SELECT organization_id, 'system', 'approval.expired', 'approval', id::text, 'deny',
               jsonb_build_object('reason', 'approval expired')
        FROM candidates
    )
    SELECT count(*) INTO expired FROM candidates;
    RETURN expired;
END;
$$;

REVOKE ALL ON FUNCTION atlas_expire_approvals() FROM PUBLIC;
