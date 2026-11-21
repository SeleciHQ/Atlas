-- name: GetPlatformMetadata :one
SELECT key, value, updated_at
FROM platform_metadata
WHERE key = $1;

-- name: UpsertPlatformMetadata :one
INSERT INTO platform_metadata (key, value, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = now()
RETURNING key, value, updated_at;
