// Command atlas-migrate applies, reverts and reports Atlas database migrations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"atlas/internal/platform/config"
	"atlas/internal/platform/db"
	"atlas/internal/platform/logging"
	"atlas/internal/platform/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		usage()
		return fmt.Errorf("missing command")
	}
	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = logging.New(cfg) // initializes log formatting for future use

	flags := flag.NewFlagSet("atlas-migrate", flag.ContinueOnError)
	directory := flags.String("dir", cfg.MigrationsDir, "directory containing migration files")
	steps := flags.Int("steps", 1, "number of migrations to revert (down only)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 5)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrationsFS := os.DirFS(*directory)

	switch command {
	case "up":
		applied, err := migrate.Up(ctx, pool, migrationsFS)
		if err != nil {
			return err
		}
		fmt.Printf("applied %d migration(s)\n", applied)
	case "down":
		reverted, err := migrate.Down(ctx, pool, migrationsFS, *steps)
		if err != nil {
			return err
		}
		fmt.Printf("reverted %d migration(s)\n", reverted)
	case "status":
		status, err := migrate.Status(ctx, pool, migrationsFS)
		if err != nil {
			return err
		}
		fmt.Printf("applied: %d\n", len(status.Applied))
		for _, record := range status.Applied {
			fmt.Printf("  %s  %s  %s\n", record.Version, record.Name, record.AppliedAt.UTC().Format(time.RFC3339))
		}
		fmt.Printf("pending: %d\n", status.PendingCount)
		for _, version := range status.Pending {
			fmt.Printf("  %s\n", version)
		}
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: atlas-migrate [up|down|status] [-dir migrations] [-steps 1]")
}
