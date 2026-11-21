"""Minimal Atlas Python worker skeleton (Phase 3).

Claims jobs from the PostgreSQL queue, writes a job result, and never touches
canonical tables. Execution is at-least-once: handlers must be idempotent.

Run:
    pip install "psycopg[binary]"
    WORKER_DATABASE_URL=postgres://atlas_worker:...@localhost:5432/atlas \
        python atlas_worker.py
"""

from __future__ import annotations

import hashlib
import json
import os
import signal
import sys
import time
import urllib.request

try:
    import psycopg
except ImportError:  # pragma: no cover - operator guidance
    sys.exit("atlas-python-worker: install psycopg with `pip install \"psycopg[binary]\"`")

STOP = False


def _handle_signal(signum, _frame):
    global STOP
    STOP = True


def extract(job: dict) -> dict:
    """Deterministic document.extract skeleton.

    The payload carries the document metadata and a short-lived presigned
    download URL issued by the Go API after authorization. The worker reads the
    bytes, verifies the SHA-256 when supplied, runs its provider, and returns a
    schema_version 1 result. It never writes canonical tables.
    """
    payload = job.get("payload") or {}
    document_id = payload.get("document_id", job["id"])
    if payload.get("download_url"):
        with urllib.request.urlopen(payload["download_url"], timeout=30) as response:
            data = response.read()
        expected = payload.get("sha256", "")
        if expected and hashlib.sha256(data).hexdigest() != expected:
            raise ValueError("document checksum mismatch")
    return {
        "schema_version": 1,
        "provider": "python-mock",
        "document_id": document_id,
        "supplier_name": "Python Mock Supplier",
        "invoice_number": f"INV-{document_id[:8].upper()}",
        "currency": "EUR",
        "total_minor": 12000,
        "confidence": 0.95,
        "lines": [{"description": "Extracted line", "quantity": "1.000000",
                   "unit_price_minor": 12000, "currency": "EUR", "confidence": 0.95}],
        "warnings": [],
    }


HANDLERS = {"document.extract": extract}


def run() -> None:
    dsn = os.environ.get("WORKER_DATABASE_URL") or os.environ.get("DATABASE_URL")
    if not dsn:
        sys.exit("atlas-python-worker: WORKER_DATABASE_URL or DATABASE_URL is required")
    worker_id = os.environ.get("WORKER_ID") or f"atlas-python-{os.getpid()}"
    lease_seconds = int(os.environ.get("JOB_LEASE_SECONDS", "60"))
    poll_seconds = float(os.environ.get("JOB_POLL_MS", "1000")) / 1000.0

    signal.signal(signal.SIGINT, _handle_signal)
    signal.signal(signal.SIGTERM, _handle_signal)

    with psycopg.connect(dsn, autocommit=True) as connection:
        while not STOP:
            with connection.cursor() as cursor:
                cursor.execute("SELECT * FROM atlas_claim_job(%s, %s)", (worker_id, lease_seconds))
                row = cursor.fetchone()
                if row is None:
                    time.sleep(poll_seconds)
                    continue
                columns = [description.name for description in cursor.description]
                job = dict(zip(columns, row))

            handler = HANDLERS.get(job["type"])
            if handler is None:
                _fail(connection, job, f"no Python handler for job type {job['type']!r}", retryable=False)
                continue
            try:
                result = handler(job)
            except Exception as error:  # noqa: BLE001 - worker boundary
                _fail(connection, job, str(error), retryable=True)
                continue
            _complete(connection, job, result)
    print("atlas-python-worker: stopped")


def _complete(connection, job: dict, result: dict) -> None:
    with connection.transaction(), connection.cursor() as cursor:
        cursor.execute("SELECT set_config('app.organization_id', %s, true)", (job["organization_id"],))
        cursor.execute(
            """UPDATE jobs SET status = 'completed', completed_at = now(), updated_at = now(),
                      locked_by = NULL, lease_expires_at = NULL
               WHERE id = %s AND organization_id = %s AND status = 'running' AND locked_by = %s""",
            (job["id"], job["organization_id"], job["locked_by"]),
        )
        cursor.execute(
            """INSERT INTO job_results (job_id, organization_id, schema_version, payload)
               VALUES (%s, %s, %s, %s)
               ON CONFLICT (job_id) DO UPDATE SET payload = EXCLUDED.payload""",
            (job["id"], job["organization_id"], result.get("schema_version", 1), json.dumps(result)),
        )
        cursor.execute(
            """UPDATE job_attempts SET outcome = 'completed', finished_at = now()
               WHERE job_id = %s AND attempt = %s AND finished_at IS NULL""",
            (job["id"], job["attempts"]),
        )


def _fail(connection, job: dict, message: str, retryable: bool) -> None:
    dead_letter = not retryable or job["attempts"] >= job["max_attempts"]
    with connection.transaction(), connection.cursor() as cursor:
        cursor.execute("SELECT set_config('app.organization_id', %s, true)", (job["organization_id"],))
        if dead_letter:
            cursor.execute(
                """UPDATE jobs SET status = 'dead_letter', last_error = %s, updated_at = now(),
                          dead_lettered_at = now(), locked_by = NULL, lease_expires_at = NULL
                   WHERE id = %s AND organization_id = %s AND status = 'running' AND locked_by = %s""",
                (message[:2000], job["id"], job["organization_id"], job["locked_by"]),
            )
        else:
            cursor.execute(
                """UPDATE jobs SET status = 'retry_scheduled', last_error = %s,
                          run_at = now() + (power(2, least(attempts, 8)) * interval '2 seconds'),
                          updated_at = now(), locked_by = NULL, lease_expires_at = NULL
                   WHERE id = %s AND organization_id = %s AND status = 'running' AND locked_by = %s""",
                (message[:2000], job["id"], job["organization_id"], job["locked_by"]),
            )
        cursor.execute(
            """UPDATE job_attempts SET outcome = 'failed', error = %s, finished_at = now()
               WHERE job_id = %s AND attempt = %s AND finished_at IS NULL""",
            (message[:2000], job["id"], job["attempts"]),
        )


if __name__ == "__main__":
    run()

