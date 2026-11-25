package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/echoflying/nominatim-cache/backend/internal/store"
	"github.com/echoflying/nominatim-cache/backend/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("迁移失败", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	usage := errors.New("usage: nc-migrate schema | sqlite -snapshot /path/to/cache.db [-dry-run] | export-sqlite -output /path/to/cache.db")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "schema":
		if len(args) != 1 {
			return usage
		}
		target, err := store.Open(os.Getenv("MIGRATE_DSN"))
		if err != nil {
			return err
		}
		defer target.Close()
		if err := migrations.Apply(ctx, target); err != nil {
			return err
		}
		slog.Info("MariaDB 表结构已就绪")
		return nil
	case "sqlite":
		flags := flag.NewFlagSet("sqlite", flag.ContinueOnError)
		path := flags.String("snapshot", "", "path to a standalone SQLite backup")
		dryRun := flags.Bool("dry-run", false, "verify the snapshot without contacting MariaDB")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *path == "" || flags.NArg() > 0 {
			return usage
		}
		source, sum, err := openSnapshot(ctx, *path)
		if err != nil {
			return err
		}
		defer source.Close()
		reader, err := legacyReader(ctx, source)
		if err != nil {
			return err
		}
		if *dryRun {
			result, err := reader.copy(ctx, nil)
			if err != nil {
				return err
			}
			result.SHA256 = sum
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		target, err := store.Open(os.Getenv("MIGRATE_DSN"))
		if err != nil {
			return err
		}
		defer target.Close()
		if err := migrations.Verify(ctx, target); err != nil {
			return err
		}
		receipt, err := importSnapshot(ctx, reader, target, sum)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "export-sqlite":
		flags := flag.NewFlagSet("export-sqlite", flag.ContinueOnError)
		output := flags.String("output", "", "path for the new Node-compatible SQLite file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *output == "" || flags.NArg() > 0 {
			return usage
		}
		source, err := store.Open(os.Getenv("MIGRATE_DSN"))
		if err != nil {
			return err
		}
		defer source.Close()
		result, err := exportSQLite(ctx, source, *output)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return usage
	}
}
