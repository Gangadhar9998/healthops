package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mongoStoreFakeMirror struct {
	state     State
	dashboard DashboardSnapshot

	readErr      error
	dashboardErr error
	syncErr      error

	readCalls      int
	dashboardCalls int
	syncCalls      int
	pingCalls      int
	syncedStates   []State
}

func (f *mongoStoreFakeMirror) SyncState(_ context.Context, state State) error {
	f.syncCalls++
	f.syncedStates = append(f.syncedStates, state)
	if f.syncErr != nil {
		return f.syncErr
	}
	f.state = state
	return nil
}

func (f *mongoStoreFakeMirror) ReadState(context.Context) (State, error) {
	f.readCalls++
	return f.state, f.readErr
}

func (f *mongoStoreFakeMirror) ReadDashboardSnapshot(context.Context) (DashboardSnapshot, error) {
	f.dashboardCalls++
	return f.dashboard, f.dashboardErr
}

func (f *mongoStoreFakeMirror) Ping(context.Context) error {
	f.pingCalls++
	return f.readErr
}

func TestNewMongoStoreRejectsNilMirror(t *testing.T) {
	store, err := NewMongoStore(nil, nil)
	if err == nil {
		t.Fatal("expected nil mirror error")
	}
	if store != nil {
		t.Fatalf("expected nil store, got %#v", store)
	}
}

func TestNewMongoStoreSeedsEmptyStateWithClonedChecks(t *testing.T) {
	enabled := true
	seed := []CheckConfig{
		{
			ID:       "seed",
			Name:     "Seed",
			Type:     "api",
			Target:   "https://example.com/health",
			Enabled:  &enabled,
			Tags:     []string{"prod"},
			Metadata: map[string]string{"team": "ops"},
			MySQL:    &MySQLCheckConfig{Host: "db.internal"},
			SSH:      &SSHCheckConfig{Host: "host.internal", Metrics: []string{"cpu"}},
		},
	}
	mirror := &mongoStoreFakeMirror{}

	store, err := NewMongoStore(mirror, seed)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}
	if mirror.syncCalls != 1 {
		t.Fatalf("expected seed sync, got %d sync calls", mirror.syncCalls)
	}
	if mirror.state.UpdatedAt.IsZero() {
		t.Fatal("expected seeded state UpdatedAt to be set")
	}

	seed[0].Name = "Changed"
	seed[0].Tags[0] = "changed"
	seed[0].Metadata["team"] = "changed"
	*seed[0].Enabled = false
	seed[0].MySQL.Host = "changed"
	seed[0].SSH.Metrics[0] = "changed"

	got := store.Snapshot()
	if got.Checks[0].Name != "Seed" {
		t.Fatalf("expected cloned seed name, got %q", got.Checks[0].Name)
	}
	if got.Checks[0].Tags[0] != "prod" {
		t.Fatalf("expected cloned seed tags, got %#v", got.Checks[0].Tags)
	}
	if got.Checks[0].Metadata["team"] != "ops" {
		t.Fatalf("expected cloned seed metadata, got %#v", got.Checks[0].Metadata)
	}
	if got.Checks[0].Enabled == nil || *got.Checks[0].Enabled != true {
		t.Fatalf("expected cloned enabled flag, got %#v", got.Checks[0].Enabled)
	}
	if got.Checks[0].MySQL == nil || got.Checks[0].MySQL.Host != "db.internal" {
		t.Fatalf("expected cloned MySQL config, got %#v", got.Checks[0].MySQL)
	}
	if got.Checks[0].SSH == nil || got.Checks[0].SSH.Metrics[0] != "cpu" {
		t.Fatalf("expected cloned SSH config, got %#v", got.Checks[0].SSH)
	}
}

func TestNewMongoStoreDoesNotSeedWhenStateExists(t *testing.T) {
	existingUpdatedAt := time.Now().UTC().Add(-time.Hour)
	mirror := &mongoStoreFakeMirror{
		state: State{
			Checks:    []CheckConfig{{ID: "existing", Name: "Existing", Type: "api"}},
			UpdatedAt: existingUpdatedAt,
		},
	}

	store, err := NewMongoStore(mirror, []CheckConfig{{ID: "seed", Name: "Seed", Type: "api"}})
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}
	if mirror.syncCalls != 0 {
		t.Fatalf("expected no seed sync, got %d sync calls", mirror.syncCalls)
	}

	got := store.Snapshot()
	if len(got.Checks) != 1 || got.Checks[0].ID != "existing" {
		t.Fatalf("expected existing checks to remain authoritative, got %#v", got.Checks)
	}
	if !got.UpdatedAt.Equal(existingUpdatedAt) {
		t.Fatalf("expected existing UpdatedAt %v, got %v", existingUpdatedAt, got.UpdatedAt)
	}
}

func TestMongoStoreDashboardSnapshotUsesMirrorOrBuildsFallback(t *testing.T) {
	generatedAt := time.Now().UTC().Add(-time.Minute)
	mirror := &mongoStoreFakeMirror{
		state: State{
			Checks: []CheckConfig{{ID: "state-check", Name: "State Check", Type: "api"}},
		},
		dashboard: DashboardSnapshot{
			State:       State{Checks: []CheckConfig{{ID: "dashboard-check", Name: "Dashboard Check", Type: "tcp"}}},
			Summary:     Summary{TotalChecks: 99},
			GeneratedAt: generatedAt,
		},
	}
	store, err := NewMongoStore(mirror, nil)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}

	got := store.DashboardSnapshot()
	if got.Summary.TotalChecks != 99 {
		t.Fatalf("expected mirror dashboard snapshot, got %#v", got.Summary)
	}
	got.State.Checks[0].ID = "mutated"
	again := store.DashboardSnapshot()
	if again.State.Checks[0].ID != "dashboard-check" {
		t.Fatalf("dashboard snapshot should be cloned, got %#v", again.State.Checks)
	}

	mirror.dashboard = DashboardSnapshot{}
	fallback := store.DashboardSnapshot()
	if fallback.Summary.TotalChecks != 1 {
		t.Fatalf("expected fallback summary to be built from state, got %#v", fallback.Summary)
	}
	if fallback.State.Checks[0].ID != "state-check" {
		t.Fatalf("expected fallback state snapshot, got %#v", fallback.State.Checks)
	}
	if fallback.GeneratedAt.IsZero() {
		t.Fatal("expected fallback GeneratedAt to be set")
	}
}

func TestMongoStorePingUsesMirrorHealthCheck(t *testing.T) {
	mirror := &mongoStoreFakeMirror{}
	store, err := NewMongoStore(mirror, nil)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}

	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if mirror.pingCalls != 1 {
		t.Fatalf("expected mirror ping, got %d calls", mirror.pingCalls)
	}
}

func TestMongoStoreCheckAndResultOperationsUpdateMirror(t *testing.T) {
	now := time.Now().UTC()
	mirror := &mongoStoreFakeMirror{
		state: State{
			Checks: []CheckConfig{{ID: "c0", Name: "Original", Type: "api"}},
		},
	}
	store, err := NewMongoStore(mirror, nil)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}

	if err := store.ReplaceChecks([]CheckConfig{
		{ID: "c1", Name: "One", Type: "api"},
		{ID: "c2", Name: "Two", Type: "tcp"},
	}); err != nil {
		t.Fatalf("replace checks: %v", err)
	}
	if err := store.UpsertCheck(CheckConfig{ID: "c1", Name: "One Updated", Type: "api"}); err != nil {
		t.Fatalf("upsert check: %v", err)
	}
	if err := store.DeleteCheck("c2"); err != nil {
		t.Fatalf("delete check: %v", err)
	}
	if err := store.AppendResults([]CheckResult{
		{ID: "old", CheckID: "c1", Status: "healthy", StartedAt: now.Add(-10 * 24 * time.Hour), FinishedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "new", CheckID: "c1", Status: "healthy", StartedAt: now, FinishedAt: now},
	}, 7); err != nil {
		t.Fatalf("append results: %v", err)
	}
	localZone := time.FixedZone("local", 2*60*60)
	lastRun := time.Date(2026, 5, 9, 10, 30, 0, 0, localZone)
	if err := store.SetLastRun(lastRun); err != nil {
		t.Fatalf("set last run: %v", err)
	}

	if mirror.syncCalls != 5 {
		t.Fatalf("expected each operation to sync through Update, got %d sync calls", mirror.syncCalls)
	}
	got := store.Snapshot()
	if len(got.Checks) != 1 || got.Checks[0].ID != "c1" || got.Checks[0].Name != "One Updated" {
		t.Fatalf("unexpected checks after operations: %#v", got.Checks)
	}
	if len(got.Results) != 1 || got.Results[0].ID != "new" {
		t.Fatalf("expected only retained new result, got %#v", got.Results)
	}
	if got.LastRunAt.Location() != time.UTC {
		t.Fatalf("expected UTC last run, got %v", got.LastRunAt.Location())
	}
	if !got.LastRunAt.Equal(lastRun.UTC()) {
		t.Fatalf("expected last run %v, got %v", lastRun.UTC(), got.LastRunAt)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestMongoStoreUpdateAbortsOnMutatorError(t *testing.T) {
	updateErr := errors.New("stop update")
	mirror := &mongoStoreFakeMirror{
		state: State{
			Checks: []CheckConfig{
				{
					ID:       "c1",
					Name:     "One",
					Type:     "api",
					Tags:     []string{"prod"},
					Metadata: map[string]string{"team": "ops"},
				},
			},
		},
	}
	store, err := NewMongoStore(mirror, nil)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}

	err = store.Update(func(state *State) error {
		state.Checks[0].Name = "Mutated"
		state.Checks[0].Tags[0] = "mutated"
		state.Checks[0].Metadata["team"] = "mutated"
		state.Checks = append(state.Checks, CheckConfig{ID: "c2", Name: "Two", Type: "tcp"})
		return updateErr
	})
	if !errors.Is(err, updateErr) {
		t.Fatalf("expected mutator error, got %v", err)
	}
	if mirror.syncCalls != 0 {
		t.Fatalf("expected no sync on mutator error, got %d sync calls", mirror.syncCalls)
	}

	got := store.Snapshot()
	if len(got.Checks) != 1 || got.Checks[0].Name != "One" {
		t.Fatalf("expected state to remain unchanged, got %#v", got.Checks)
	}
	if got.Checks[0].Tags[0] != "prod" || got.Checks[0].Metadata["team"] != "ops" {
		t.Fatalf("expected nested check data to remain unchanged, got %#v", got.Checks[0])
	}
}

func TestMongoStoreClonesSnapshotsAndUpdateState(t *testing.T) {
	mirror := &mongoStoreFakeMirror{
		state: State{
			Checks: []CheckConfig{
				{
					ID:       "c1",
					Name:     "One",
					Type:     "api",
					Tags:     []string{"prod"},
					Metadata: map[string]string{"team": "ops"},
					SSH:      &SSHCheckConfig{Metrics: []string{"cpu"}},
				},
			},
			Results: []CheckResult{
				{
					ID:      "r1",
					CheckID: "c1",
					Status:  "healthy",
					Metrics: map[string]float64{"latency": 42},
					Tags:    []string{"api"},
				},
			},
		},
	}
	store, err := NewMongoStore(mirror, nil)
	if err != nil {
		t.Fatalf("new mongo store: %v", err)
	}

	snapshot := store.Snapshot()
	snapshot.Checks[0].Name = "Snapshot Mutated"
	snapshot.Checks[0].Tags[0] = "snapshot"
	snapshot.Checks[0].Metadata["team"] = "snapshot"
	snapshot.Checks[0].SSH.Metrics[0] = "snapshot"
	snapshot.Results[0].Metrics["latency"] = 999
	snapshot.Results[0].Tags[0] = "snapshot"

	again := store.Snapshot()
	if again.Checks[0].Name != "One" ||
		again.Checks[0].Tags[0] != "prod" ||
		again.Checks[0].Metadata["team"] != "ops" ||
		again.Checks[0].SSH.Metrics[0] != "cpu" ||
		again.Results[0].Metrics["latency"] != 42 ||
		again.Results[0].Tags[0] != "api" {
		t.Fatalf("snapshot mutation leaked into mirror state: %#v", again)
	}

	var mutatorState *State
	if err := store.Update(func(state *State) error {
		mutatorState = state
		state.Checks[0].Name = "Updated"
		state.Checks[0].Metadata["team"] = "platform"
		state.Results[0].Metrics["latency"] = 50
		return nil
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	mutatorState.Checks[0].Name = "Mutated After Update"
	mutatorState.Checks[0].Metadata["team"] = "mutated"
	mutatorState.Results[0].Metrics["latency"] = 777

	got := store.Snapshot()
	if got.Checks[0].Name != "Updated" {
		t.Fatalf("mutator-owned state leaked after sync, got check name %q", got.Checks[0].Name)
	}
	if got.Checks[0].Metadata["team"] != "platform" {
		t.Fatalf("mutator-owned check metadata leaked after sync: %#v", got.Checks[0].Metadata)
	}
	if got.Results[0].Metrics["latency"] != 50 {
		t.Fatalf("mutator-owned result metrics leaked after sync: %#v", got.Results[0].Metrics)
	}
}
