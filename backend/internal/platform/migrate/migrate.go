// Package migrate implements versioned, advisory-locked SQL migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockID int64 = 7_331_000_001

var filenamePattern = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

// Migration is a parsed up/down migration pair.
type Migration struct {
	Version  string
	Name     string
	Up       string
	Down     string
	Checksum string
}

// Applied describes a migration recorded in the database.
type Applied struct {
	Version     string
	Name        string
	Checksum    string
	AppliedAt   time.Time
	ExecutionMS int32
}

// Status summarizes applied and pending migrations.
type State struct {
	Applied       []Applied
	Pending       []string
	PendingCount  int
	LatestApplied string
}

// Load reads and validates migration files from fsys.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	type pair struct {
		name    string
		up      *string
		down    *string
		version string
	}
	pairs := map[string]*pair{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := filenamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, name, direction := match[1], match[2], match[3]
		content, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		text := string(content)

		current, ok := pairs[version]
		if !ok {
			current = &pair{name: name, version: version}
			pairs[version] = current
		}
		if current.name != name {
			return nil, fmt.Errorf("migration version %s has conflicting names %q and %q", version, current.name, name)
		}
		if direction == "up" {
			if current.up != nil {
				return nil, fmt.Errorf("duplicate up migration for version %s", version)
			}
			current.up = &text
		} else {
			if current.down != nil {
				return nil, fmt.Errorf("duplicate down migration for version %s", version)
			}
			current.down = &text
		}
	}

	migrations := make([]Migration, 0, len(pairs))
	for _, item := range pairs {
		if item.up == nil {
			return nil, fmt.Errorf("migration %s is missing its .up.sql file", item.version)
		}
		if item.down == nil {
			return nil, fmt.Errorf("migration %s is missing its .down.sql file", item.version)
		}
		sum := sha256.Sum256([]byte(*item.up))
		migrations = append(migrations, Migration{
			Version:  item.version,
			Name:     item.name,
			Up:       *item.up,
			Down:     *item.down,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return versionNumber(migrations[i].Version) < versionNumber(migrations[j].Version)
	})
	return migrations, nil
}

func versionNumber(version string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimLeft(version, "0"), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// Up applies all pending migrations and returns how many were applied.
func Up(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (int, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return 0, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if err := lock(ctx, conn); err != nil {
		return 0, err
	}
	defer unlock(conn)

	if err := ensureTable(ctx, conn); err != nil {
		return 0, err
	}
	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return 0, err
	}

	appliedChecksums := make(map[string]string, len(applied))
	for _, record := range applied {
		appliedChecksums[record.Version] = record.Checksum
	}
	for _, migration := range migrations {
		if checksum, ok := appliedChecksums[migration.Version]; ok && checksum != migration.Checksum {
			return 0, fmt.Errorf(
				"migration %s checksum mismatch: applied=%s file=%s (migrations must not be edited after being applied)",
				migration.Version, checksum, migration.Checksum,
			)
		}
	}

	appliedCount := 0
	for _, migration := range migrations {
		if _, ok := appliedChecksums[migration.Version]; ok {
			continue
		}
		start := time.Now()
		tx, err := conn.Begin(ctx)
		if err != nil {
			return appliedCount, fmt.Errorf("begin migration %s: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, migration.Up, pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name, checksum, execution_ms) VALUES ($1, $2, $3, $4)`,
			migration.Version, migration.Name, migration.Checksum, time.Since(start).Milliseconds(),
		); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return appliedCount, fmt.Errorf("commit migration %s: %w", migration.Version, err)
		}
		appliedCount++
	}
	return appliedCount, nil
}

// Down reverts the most recently applied migrations, up to steps.
func Down(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, steps int) (int, error) {
	if steps < 1 {
		return 0, fmt.Errorf("steps must be at least 1")
	}
	migrations, err := Load(fsys)
	if err != nil {
		return 0, err
	}
	byVersion := make(map[string]Migration, len(migrations))
	for _, migration := range migrations {
		byVersion[migration.Version] = migration
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if err := lock(ctx, conn); err != nil {
		return 0, err
	}
	defer unlock(conn)
	if err := ensureTable(ctx, conn); err != nil {
		return 0, err
	}

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return 0, err
	}
	reverted := 0
	for i := len(applied) - 1; i >= 0 && reverted < steps; i-- {
		record := applied[i]
		migration, ok := byVersion[record.Version]
		if !ok {
			return reverted, fmt.Errorf("cannot revert migration %s: file not found", record.Version)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return reverted, fmt.Errorf("begin revert %s: %w", record.Version, err)
		}
		if _, err := tx.Exec(ctx, migration.Down, pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			return reverted, fmt.Errorf("revert migration %s: %w", record.Version, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, record.Version); err != nil {
			_ = tx.Rollback(ctx)
			return reverted, fmt.Errorf("remove migration record %s: %w", record.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return reverted, fmt.Errorf("commit revert %s: %w", record.Version, err)
		}
		reverted++
	}
	return reverted, nil
}

// Status reports applied and pending migrations without modifying state.
func Status(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (State, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return State{}, err
	}
	exists, err := tableExists(ctx, pool)
	if err != nil {
		return State{}, err
	}
	var applied []Applied
	if exists {
		applied, err = loadApplied(ctx, pool)
		if err != nil {
			return State{}, err
		}
	}

	appliedVersions := make(map[string]struct{}, len(applied))
	result := State{Applied: applied}
	for _, record := range applied {
		appliedVersions[record.Version] = struct{}{}
	}
	for _, migration := range migrations {
		if _, ok := appliedVersions[migration.Version]; !ok {
			result.Pending = append(result.Pending, migration.Version)
		}
	}
	result.PendingCount = len(result.Pending)
	if len(applied) > 0 {
		result.LatestApplied = applied[len(applied)-1].Version
	}
	return result, nil
}

// Pending counts migrations that have not been applied.
func Pending(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (int, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return 0, err
	}
	exists, err := tableExists(ctx, pool)
	if err != nil {
		return 0, err
	}
	if !exists {
		return len(migrations), nil
	}
	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return 0, err
	}
	appliedVersions := make(map[string]struct{}, len(applied))
	for _, record := range applied {
		appliedVersions[record.Version] = struct{}{}
	}
	pending := 0
	for _, migration := range migrations {
		if _, ok := appliedVersions[migration.Version]; !ok {
			pending++
		}
	}
	return pending, nil
}

func ensureTable(ctx context.Context, conn *pgxpool.Conn) error {
	_, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			name text NOT NULL,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now(),
			execution_ms integer NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	return nil
}

func tableExists(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check schema_migrations table: %w", err)
	}
	return exists, nil
}

type rowQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadApplied(ctx context.Context, q rowQuerier) ([]Applied, error) {
	rows, err := q.Query(ctx, `
		SELECT version, name, checksum, applied_at, execution_ms
		FROM schema_migrations
		ORDER BY applied_at ASC, version ASC`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	var applied []Applied
	for rows.Next() {
		var record Applied
		if err := rows.Scan(&record.Version, &record.Name, &record.Checksum, &record.AppliedAt, &record.ExecutionMS); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied = append(applied, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func lock(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	return nil
}

func unlock(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockID)
}
