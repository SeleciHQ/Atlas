// Command atlas-provision executes the real role-provisioning SQL against
// PostgreSQL using passwords from the environment. It exists so environments
// without psql can still provision the least-privilege roles.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-provision:", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	file := flag.String("file", "scripts/provision-app-role.sql", "provisioning SQL file")
	flag.Parse()

	secrets := map[string]string{
		"atlas.app_password":      os.Getenv("ATLAS_APP_PASSWORD"),
		"atlas.auth_password":     os.Getenv("ATLAS_AUTH_PASSWORD"),
		"atlas.migrator_password": os.Getenv("ATLAS_MIGRATOR_PASSWORD"),
		"atlas.worker_password":   os.Getenv("ATLAS_WORKER_PASSWORD"),
	}
	for setting, value := range secrets {
		if value == "" {
			return fmt.Errorf("%s must be provided in the environment", setting)
		}
	}

	script, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read provisioning script: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	for setting, value := range secrets {
		if _, err := conn.Exec(ctx, `SELECT set_config($1, $2, false)`, setting, value); err != nil {
			return fmt.Errorf("set %s: %w", setting, err)
		}
	}
	if _, err := conn.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("execute provisioning script: %w", err)
	}

	fmt.Println("Atlas database roles provisioned.")
	return nil
}
