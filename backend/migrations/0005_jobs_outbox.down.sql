DROP FUNCTION IF EXISTS atlas_claim_outbox(integer);
DROP FUNCTION IF EXISTS atlas_reap_expired_leases();
DROP FUNCTION IF EXISTS atlas_claim_job(text, integer);

DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS job_results;
DROP TABLE IF EXISTS job_attempts;
DROP TABLE IF EXISTS jobs;
