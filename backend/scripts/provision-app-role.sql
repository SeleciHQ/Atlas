-- Provisions Atlas's least-privilege database roles.
--
-- The script is plain SQL so it can be executed by any PostgreSQL client. It
-- requires four session settings before execution (passwords are never stored
-- in the file):
--   SET atlas.app_password = '...';
--   SET atlas.auth_password = '...';
--   SET atlas.migrator_password = '...';
--   SET atlas.worker_password = '...';
--
-- Operator example without psql (uses the Atlas Go tooling):
--   ATLAS_APP_PASSWORD=... ATLAS_AUTH_PASSWORD=... \
--   ATLAS_MIGRATOR_PASSWORD=... ATLAS_WORKER_PASSWORD=... \
--   go run ./cmd/atlas-provision
--
-- psql example:
--   psql "$DATABASE_URL" \
--     -c "SET atlas.app_password = '$ATLAS_APP_PASSWORD'; \
--         SET atlas.auth_password = '$ATLAS_AUTH_PASSWORD'; \
--         SET atlas.migrator_password = '$ATLAS_MIGRATOR_PASSWORD'; \
--         SET atlas.worker_password = '$ATLAS_WORKER_PASSWORD';" \
--     -f scripts/provision-app-role.sql
--
-- Roles:
--   atlas_app      runtime API/MCP: tenant tables only; no users/sessions; no RLS bypass.
--   atlas_auth     identity/session operations only: users, sessions, memberships/orgs read, audit insert.
--   atlas_migrator schema owner for migrations and security-definer functions.
--   atlas_worker   background workers; Phase 3 narrows grants to queue/job tables.
-- Audit rows are append-only for app/worker roles (UPDATE/DELETE revoked).
-- Re-run this script after applying migrations so new tables receive grants.

DO $$
DECLARE pw text;
BEGIN
    pw := nullif(current_setting('atlas.app_password', true), '');
    IF pw IS NULL THEN RAISE EXCEPTION 'atlas.app_password must be set'; END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'atlas_app') THEN
        EXECUTE format('CREATE ROLE atlas_app LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', pw);
    ELSE
        EXECUTE format('ALTER ROLE atlas_app PASSWORD %L', pw);
    END IF;
END $$;

DO $$
DECLARE pw text;
BEGIN
    pw := nullif(current_setting('atlas.auth_password', true), '');
    IF pw IS NULL THEN RAISE EXCEPTION 'atlas.auth_password must be set'; END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'atlas_auth') THEN
        EXECUTE format('CREATE ROLE atlas_auth LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', pw);
    ELSE
        EXECUTE format('ALTER ROLE atlas_auth PASSWORD %L', pw);
    END IF;
END $$;

DO $$
DECLARE pw text;
BEGIN
    pw := nullif(current_setting('atlas.migrator_password', true), '');
    IF pw IS NULL THEN RAISE EXCEPTION 'atlas.migrator_password must be set'; END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'atlas_migrator') THEN
        EXECUTE format('CREATE ROLE atlas_migrator LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', pw);
    ELSE
        EXECUTE format('ALTER ROLE atlas_migrator PASSWORD %L', pw);
    END IF;
END $$;

DO $$
DECLARE pw text;
BEGIN
    pw := nullif(current_setting('atlas.worker_password', true), '');
    IF pw IS NULL THEN RAISE EXCEPTION 'atlas.worker_password must be set'; END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'atlas_worker') THEN
        EXECUTE format('CREATE ROLE atlas_worker LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', pw);
    ELSE
        EXECUTE format('ALTER ROLE atlas_worker PASSWORD %L', pw);
    END IF;
END $$;

ALTER ROLE atlas_app NOSUPERUSER NOBYPASSRLS;
ALTER ROLE atlas_auth NOSUPERUSER NOBYPASSRLS;
ALTER ROLE atlas_migrator NOSUPERUSER NOBYPASSRLS;
ALTER ROLE atlas_worker NOSUPERUSER NOBYPASSRLS;

GRANT USAGE ON SCHEMA public TO atlas_app, atlas_auth, atlas_worker;
GRANT USAGE, CREATE ON SCHEMA public TO atlas_migrator;

-- The migrator owns schema objects so future DDL works when migrations run as
-- this role. Ownership does not bypass RLS for runtime roles.
DO $$
DECLARE relation record;
BEGIN
    FOR relation IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' LOOP
        EXECUTE format('ALTER TABLE public.%I OWNER TO atlas_migrator', relation.tablename);
    END LOOP;
    FOR relation IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
        EXECUTE format('ALTER SEQUENCE public.%I OWNER TO atlas_migrator', relation.sequencename);
    END LOOP;
    FOR relation IN
        SELECT p.oid::regprocedure AS signature
        FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'public' AND p.proname LIKE 'atlas_%'
    LOOP
        EXECUTE format('ALTER FUNCTION %s OWNER TO atlas_migrator', relation.signature);
    END LOOP;
END $$;

-- Runtime roles: tenant data only.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO atlas_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO atlas_worker;
REVOKE ALL ON TABLE users, sessions, oidc_identities FROM atlas_app;
REVOKE ALL ON TABLE users, sessions, oidc_identities, agent_tokens FROM atlas_worker;
REVOKE UPDATE, DELETE ON audit_events FROM atlas_app;
REVOKE UPDATE, DELETE ON audit_events FROM atlas_worker;

-- Identity/session role: narrow, pre-tenant access path.
GRANT SELECT ON TABLE users, organizations, memberships TO atlas_auth;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sessions TO atlas_auth;
GRANT INSERT ON TABLE audit_events TO atlas_auth;
GRANT SELECT, INSERT, UPDATE ON TABLE oidc_identities TO atlas_auth;
GRANT SELECT, UPDATE ON TABLE organization_invitations TO atlas_auth;
GRANT INSERT ON TABLE users TO atlas_auth;
GRANT INSERT ON TABLE memberships TO atlas_auth;

-- Only the member-listing function is available to the runtime role. Grants are
-- explicit (no default privileges) so new tables/functions fail closed until
-- this script is re-run.
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM atlas_worker;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM atlas_auth;
GRANT EXECUTE ON FUNCTION atlas_list_members() TO atlas_app;
GRANT EXECUTE ON FUNCTION atlas_memberships_for_user(uuid) TO atlas_auth;
-- Queue functions are the worker's cross-organization entry points. They are
-- the only way to claim jobs/outbox rows without an organization context.
GRANT EXECUTE ON FUNCTION atlas_claim_job(text, integer) TO atlas_worker;
GRANT EXECUTE ON FUNCTION atlas_reap_expired_leases() TO atlas_worker;
GRANT EXECUTE ON FUNCTION atlas_claim_outbox(integer) TO atlas_worker;
GRANT EXECUTE ON FUNCTION atlas_expire_approvals() TO atlas_worker;
GRANT EXECUTE ON FUNCTION atlas_resolve_agent_token(text) TO atlas_app;
