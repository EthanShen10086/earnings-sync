package job

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

// OpenOptions selects file vs Postgres job persistence.
type OpenOptions struct {
	Path          string
	DatabaseURL   string
	MigrationsDir string
	// DB reuses an already-migrated connection (preferred when app also opens postgres for sheets).
	DB *sql.DB
}

// OpenAuto returns a Postgres job store when opts.DB or opts.DatabaseURL is set;
// otherwise the file-backed store at Path (default JOB_STORE_PATH / data/jobs.json).
// Callers that want env-driven PG must pass DatabaseURL explicitly (do not rely on
// ambient DATABASE_URL here — CI sets it globally while unit tests stay file-backed).
func OpenAuto(ctx context.Context, opts OpenOptions) (Store, func() error, error) {
	noop := func() error { return nil }
	if opts.DB != nil {
		return NewPG(opts.DB), noop, nil
	}
	if opts.DatabaseURL != "" {
		pg, err := postgres.Open(ctx, opts.DatabaseURL)
		if err != nil {
			return nil, noop, fmt.Errorf("job postgres: %w", err)
		}
		if opts.MigrationsDir != "" {
			pg.MigrationsDir = opts.MigrationsDir
			if err := pg.Migrate(ctx); err != nil {
				_ = pg.Close()
				return nil, noop, err
			}
		}
		return NewPG(pg.DB()), pg.Close, nil
	}
	path := opts.Path
	if path == "" {
		path = os.Getenv("JOB_STORE_PATH")
	}
	if path == "" {
		path = "data/jobs.json"
	}
	fs, err := Open(path)
	if err != nil {
		return nil, noop, err
	}
	return fs, noop, nil
}
