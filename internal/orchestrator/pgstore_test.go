package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/store/postgres"
)

func TestPGRunStoreClientRequestIdempotentAcrossHandles(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mig := filepath.Join(root, "migrations")
	t.Setenv("MIGRATIONS_DIR", mig)

	pg, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	pg.MigrationsDir = mig
	if err := pg.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	a := NewPGRunStore(pg.DB())
	clientID := "client-req-" + time.Now().UTC().Format("20060102150405.000000000")
	state := &RunState{
		RunID:           "run-" + clientID,
		ClientRequestID: clientID,
		Title:           "pg idempotency",
		Status:          StatusSucceeded,
		Inputs:          map[string]any{"ticker": "AAPL"},
		Nodes: map[string]*NodeState{
			"n1": {ID: "n1", Type: "tool", Status: StatusSucceeded},
		},
		StartedAt:  time.Now().UTC().Add(-time.Second),
		EndedAt:    time.Now().UTC(),
		DurationMs: 1000,
	}
	a.Put(state)

	b := NewPGRunStore(pg.DB())
	got, ok := b.ByClientRequestID(clientID)
	if !ok || got.RunID != state.RunID {
		t.Fatalf("cross-handle ByClientRequestID got=%+v ok=%v", got, ok)
	}
	got, ok = b.Get(state.RunID)
	if !ok || got.Title != "pg idempotency" {
		t.Fatalf("get %+v", got)
	}
	if err := a.Append(ProductEvent{Name: "flow.success", RunID: state.RunID, Status: StatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	evs := b.Events(10)
	found := false
	for _, ev := range evs {
		if ev.RunID == state.RunID && ev.Name == "flow.success" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("product event missing: %+v", evs)
	}
}

func TestOpenPersistentRunsFallsBackToFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	dir := t.TempDir()
	runs, sink, cleanup, err := OpenPersistentRuns(t.Context(), PersistOpts{
		RunsPath:   filepath.Join(dir, "runs.json"),
		EventsPath: filepath.Join(dir, "events.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, ok := runs.(*RunStore); !ok {
		t.Fatalf("expected *RunStore, got %T", runs)
	}
	if _, ok := sink.(*FileRunStore); !ok {
		t.Fatalf("expected *FileRunStore sink, got %T", sink)
	}
	runs.Put(&RunState{
		RunID:     "r1",
		Status:    StatusRunning,
		StartedAt: time.Now().UTC(),
		Nodes:     map[string]*NodeState{},
	})
	if _, ok := runs.Get("r1"); !ok {
		t.Fatal("missing run")
	}
}
