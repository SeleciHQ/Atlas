// Command atlas-devdb starts a disposable local PostgreSQL instance, applies
// migrations, seeds an Atlas organization plus an invitation for the WorkOS
// sandbox account, prints the login URL, and stays running until interrupted.
// It is development tooling only — never use it in production.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"

	"atlas/internal/identity"
	"atlas/internal/platform/db"
	"atlas/internal/platform/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-devdb:", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 55432, "PostgreSQL port")
	email := flag.String("email", "ayombarudi@gmail.com", "invited email (must match the WorkOS sandbox account)")
	orgName := flag.String("org", "Atlas Sandbox", "Atlas organization name")
	role := flag.String("role", "admin", "invitation role (admin|member)")
	migrationsDir := flag.String("migrations", "migrations", "migrations directory")
	flag.Parse()

	workDir, err := os.MkdirTemp("", "atlas-devdb-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	config := embeddedpostgres.DefaultConfig().
		Username("atlas").
		Password("atlas").
		Database("atlas_dev").
		Port(uint32(*port)).
		DataPath(filepath.Join(workDir, "data")).
		RuntimePath(filepath.Join(workDir, "runtime")).
		Logger(io.Discard).
		StartTimeout(120 * time.Second)
	instance := embeddedpostgres.NewDatabase(config)
	if err := instance.Start(); err != nil {
		return fmt.Errorf("start embedded postgres: %w", err)
	}
	defer instance.Stop()

	ctx := context.Background()
	databaseURL := fmt.Sprintf("postgres://atlas:atlas@127.0.0.1:%d/atlas_dev?sslmode=disable", *port)
	pool, err := db.Connect(ctx, databaseURL, 5)
	if err != nil {
		return err
	}
	defer pool.Close()

	if _, err := migrate.Up(ctx, pool, os.DirFS(*migrationsDir)); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	var organizationID string
	err = pool.QueryRow(ctx, `SELECT id FROM organizations WHERE name = $1 ORDER BY created_at LIMIT 1`, *orgName).Scan(&organizationID)
	if err != nil {
		if err := pool.QueryRow(ctx, `INSERT INTO organizations (name) VALUES ($1) RETURNING id`, *orgName).Scan(&organizationID); err != nil {
			return fmt.Errorf("create organization: %w", err)
		}
	}

	invitations := identity.NewInvitationService(pool, 72*time.Hour)
	invitation, token, err := invitations.Create(ctx, organizationID, *email, *role, "", "Atlas Dev Seed")
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}

	startURL := fmt.Sprintf(
		"http://localhost:8080/v1/auth/oidc/start?organization_id=%s&login_hint=%s&invitation_token=%s",
		organizationID, invitation.Email, token,
	)

	fmt.Println("Atlas dev database ready (keep this process running):")
	fmt.Println()
	fmt.Println("  DATABASE_URL=" + databaseURL)
	fmt.Println("  AUTH_DATABASE_URL=" + databaseURL)
	fmt.Println()
	fmt.Println("  organization_id=" + organizationID)
	fmt.Println("  invitation_email=" + invitation.Email)
	fmt.Println("  invitation_role=" + invitation.Role)
	fmt.Println("  invitation_token=" + token)
	fmt.Println("  invitation_expires_at=" + invitation.ExpiresAt.UTC().Format(time.RFC3339))
	fmt.Println()
	fmt.Println("  OIDC start URL (run atlas-api on :8080 first):")
	fmt.Println("  " + startURL)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop and discard this temporary database.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	fmt.Println("Stopping Atlas dev database.")
	return nil
}
