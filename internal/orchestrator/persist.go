package orchestrator

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

// PersistOpts selects file vs Postgres persistence for runs/events.
type PersistOpts struct {
	RunsPath      string
	EventsPath    string
	DatabaseURL   string
	MigrationsDir string
	DB            *sql.DB // optional shared connection
}

// OpenPersistentRuns returns a RunRegistry + EventSink.
// With opts.DB or opts.DatabaseURL: Postgres. Otherwise: FileRunStore.
// Ambient DATABASE_URL is not read here — pass it explicitly from cmd/config so
// unit tests stay file/memory-backed under CI.
func OpenPersistentRuns(ctx context.Context, opts PersistOpts) (runs RunRegistry, sink EventSink, cleanup func() error, err error) {
	noop := func() error { return nil }
	if opts.DB != nil {
		pg := NewPGRunStore(opts.DB)
		return pg, pg, noop, nil
	}
	if opts.DatabaseURL != "" {
		store, err := postgres.Open(ctx, opts.DatabaseURL)
		if err != nil {
			return nil, nil, noop, fmt.Errorf("run postgres: %w", err)
		}
		if opts.MigrationsDir != "" {
			store.MigrationsDir = opts.MigrationsDir
			if err := store.Migrate(ctx); err != nil {
				_ = store.Close()
				return nil, nil, noop, err
			}
		}
		pg := NewPGRunStore(store.DB())
		return pg, pg, store.Close, nil
	}
	fs, err := OpenFileRunStore(opts.RunsPath, opts.EventsPath)
	if err != nil {
		return nil, nil, noop, err
	}
	return fs.Memory(), fs, noop, nil
}
