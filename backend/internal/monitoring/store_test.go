package monitoring

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "test", Name: "Test", Type: "api", Target: "https://example.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected state file to exist: %v", err)
	}

	// Should have checks
	state := store.Snapshot()
	if len(state.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(state.Checks))
	}
}

func TestNewFileStoreLoadsExistingState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Create initial state
	existingState := State{
		Checks: []CheckConfig{
			{ID: "existing", Name: "Existing", Type: "api", Target: "https://existing.com"},
		},
		Results: []CheckResult{
			{ID: "r1", CheckID: "existing", Status: "healthy", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
		},
		LastRunAt: time.Now().UTC().Add(-1 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}

	data := mustMarshalJSON(t, existingState)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write existing state: %v", err)
	}

	// Create store with same check in config — should preserve existing results and timestamps
	store, err := NewFileStore(path, []CheckConfig{{ID: "existing", Name: "Existing", Type: "api", Target: "https://existing.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 1 {
		t.Errorf("expected 1 check from existing state, got %d", len(state.Checks))
	}
	if state.Checks[0].ID != "existing" {
		t.Errorf("expected existing check, got %q", state.Checks[0].ID)
	}
	if len(state.Results) != 1 {
		t.Errorf("expected 1 result from existing state, got %d", len(state.Results))
	}
	if state.LastRunAt.IsZero() {
		t.Error("expected LastRunAt from existing state")
	}
}

func TestNewFileStoreIgnoresConfigWhenStateExists(t *testing.T) {
	// Config checks are SEED ONLY: when a state file already exists, the
	// persisted state is the single source of truth. Config additions,
	// updates, and removals must NOT touch existing state. This guarantees
	// that checks added at runtime via the API survive restarts.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	existingState := State{
		Checks: []CheckConfig{
			{ID: "user-added", Name: "User added via API", Type: "api", Target: "https://user.com"},
		},
	}
	data := mustMarshalJSON(t, existingState)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write existing state: %v", err)
	}

	// Config has a totally different check — must be ignored.
	store, err := NewFileStore(path, []CheckConfig{{ID: "config-only", Name: "Config", Type: "api", Target: "https://cfg.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 1 {
		t.Fatalf("expected 1 check (existing only), got %d", len(state.Checks))
	}
	if state.Checks[0].ID != "user-added" {
		t.Errorf("expected user-added preserved, got %q", state.Checks[0].ID)
	}
}

func TestNewFileStoreInitializesWithChecksIfEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	checks := []CheckConfig{
		{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"},
		{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"},
	}

	store, err := NewFileStore(path, checks)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(state.Checks))
	}
	if !state.UpdatedAt.IsZero() {
		// UpdatedAt should be set on initialization
	}
}

func TestFileStoreSnapshotReturnsClone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "test", Name: "Test", Type: "api", Target: "https://example.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	snapshot1 := store.Snapshot()
	snapshot2 := store.Snapshot()

	// Verify they're independent
	if &snapshot1.Checks == &snapshot2.Checks {
		t.Error("snapshots should have independent check slices")
	}

	// Modifying snapshot1 should not affect snapshot2 or the store
	snapshot1.Checks[0].Name = "Modified"

	snapshot3 := store.Snapshot()
	if snapshot3.Checks[0].Name == "Modified" {
		t.Error("modifying snapshot should not affect store")
	}
}

func TestFileStoreUpdateMutatesState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	err = store.Update(func(state *State) error {
		state.Checks = append(state.Checks, CheckConfig{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"})
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks after update, got %d", len(state.Checks))
	}
}

func TestFileStoreUpdateWritesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	err = store.Update(func(state *State) error {
		state.Checks = append(state.Checks, CheckConfig{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"})
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Create a new store instance - should load the updated state
	store2, err := NewFileStore(path, nil)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	state := store2.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks loaded from file, got %d", len(state.Checks))
	}
}

func TestFileStoreUpdateAbortedOnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	err = store.Update(func(state *State) error {
		state.Checks = append(state.Checks, CheckConfig{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"})
		return fmt.Errorf("intentional error")
	})
	if err == nil {
		t.Fatal("expected error from update")
	}

	// State should not have changed
	state := store.Snapshot()
	if len(state.Checks) != 1 {
		t.Errorf("expected 1 check after failed update, got %d", len(state.Checks))
	}
}

func TestFileStoreReplaceChecks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	newChecks := []CheckConfig{
		{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"},
		{ID: "c3", Name: "C3", Type: "api", Target: "https://three.com"},
	}

	err = store.ReplaceChecks(newChecks)
	if err != nil {
		t.Fatalf("replace checks: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks after replace, got %d", len(state.Checks))
	}
	if state.Checks[0].ID != "c2" {
		t.Errorf("expected first check to be c2, got %q", state.Checks[0].ID)
	}
}

func TestFileStoreUpsertCheckNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	newCheck := CheckConfig{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"}
	err = store.UpsertCheck(newCheck)
	if err != nil {
		t.Fatalf("upsert check: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks after upsert, got %d", len(state.Checks))
	}
}

func TestFileStoreUpsertCheckExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	updatedCheck := CheckConfig{ID: "c1", Name: "C1 Updated", Type: "api", Target: "https://updated.com"}
	err = store.UpsertCheck(updatedCheck)
	if err != nil {
		t.Fatalf("upsert check: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 1 {
		t.Errorf("expected 1 check after upsert, got %d", len(state.Checks))
	}
	if state.Checks[0].Name != "C1 Updated" {
		t.Errorf("expected updated name, got %q", state.Checks[0].Name)
	}
	if state.Checks[0].Target != "https://updated.com" {
		t.Errorf("expected updated target, got %q", state.Checks[0].Target)
	}
}

func TestFileStoreDeleteCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{
		{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"},
		{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"},
		{ID: "c3", Name: "C3", Type: "api", Target: "https://three.com"},
	})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	err = store.DeleteCheck("c2")
	if err != nil {
		t.Fatalf("delete check: %v", err)
	}

	state := store.Snapshot()
	if len(state.Checks) != 2 {
		t.Errorf("expected 2 checks after delete, got %d", len(state.Checks))
	}
	for _, c := range state.Checks {
		if c.ID == "c2" {
			t.Error("c2 should have been deleted")
		}
	}
}

func TestFileStoreAppendResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	now := time.Now().UTC()
	newResults := []CheckResult{
		{ID: "r1", CheckID: "c1", Status: "healthy", StartedAt: now, FinishedAt: now},
		{ID: "r2", CheckID: "c1", Status: "healthy", StartedAt: now, FinishedAt: now},
	}

	err = store.AppendResults(newResults, 7)
	if err != nil {
		t.Fatalf("append results: %v", err)
	}

	state := store.Snapshot()
	if len(state.Results) != 2 {
		t.Errorf("expected 2 results after append, got %d", len(state.Results))
	}
}

func TestFileStoreAppendResultsPrunes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	now := time.Now().UTC()

	// Create store with old results
	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	// Add some old results first
	oldResults := []CheckResult{
		{ID: "r1", CheckID: "c1", Status: "healthy", StartedAt: now.Add(-10 * 24 * time.Hour), FinishedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "r2", CheckID: "c1", Status: "healthy", StartedAt: now.Add(-8 * 24 * time.Hour), FinishedAt: now.Add(-8 * 24 * time.Hour)},
	}

	err = store.AppendResults(oldResults, 7)
	if err != nil {
		t.Fatalf("append old results: %v", err)
	}

	// Now append new results - old ones should be pruned
	newResults := []CheckResult{
		{ID: "r3", CheckID: "c1", Status: "healthy", StartedAt: now, FinishedAt: now},
	}

	err = store.AppendResults(newResults, 7)
	if err != nil {
		t.Fatalf("append new results: %v", err)
	}

	state := store.Snapshot()
	// Only r3 should remain (r1 and r2 are older than 7 days)
	if len(state.Results) != 1 {
		t.Errorf("expected 1 result after pruning, got %d", len(state.Results))
	}
	if len(state.Results) > 0 && state.Results[0].ID != "r3" {
		t.Errorf("expected r3 to remain, got %q", state.Results[0].ID)
	}
}

func TestFileStoreSetLastRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewFileStore(path, []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}})
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	now := time.Now().UTC()
	err = store.SetLastRun(now)
	if err != nil {
		t.Fatalf("set last run: %v", err)
	}

	state := store.Snapshot()
	if state.LastRunAt.IsZero() {
		t.Error("expected LastRunAt to be set")
	}
	// Time should be in UTC
	if state.LastRunAt.Location() != time.UTC {
		t.Errorf("expected LastRunAt in UTC, got %v", state.LastRunAt.Location())
	}
}

func TestFileStoreDashboardSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	checks := []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}}

	store, err := NewFileStore(path, checks)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	snapshot := store.DashboardSnapshot()

	if snapshot.State.Checks[0].ID != "c1" {
		t.Errorf("expected check c1 in snapshot, got %q", snapshot.State.Checks[0].ID)
	}
	if snapshot.Summary.TotalChecks != 1 {
		t.Errorf("expected TotalChecks=1, got %d", snapshot.Summary.TotalChecks)
	}
	if snapshot.GeneratedAt.IsZero() {
		t.Error("expected GeneratedAt to be set")
	}
}

func TestCloneState(t *testing.T) {
	now := time.Now().UTC()
	original := State{
		Checks: []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}},
		Results: []CheckResult{
			{ID: "r1", CheckID: "c1", Status: "healthy", Metrics: map[string]float64{"latency": 100}, Tags: []string{"api"}},
		},
		LastRunAt: now,
		UpdatedAt: now,
	}

	cloned := cloneState(original)

	// Verify checks are cloned
	if &cloned.Checks == &original.Checks {
		t.Error("checks should be a new slice")
	}
	cloned.Checks[0].Name = "Modified"
	if original.Checks[0].Name == "Modified" {
		t.Error("modifying clone should not affect original")
	}

	// Verify results are deeply cloned
	if &cloned.Results == &original.Results {
		t.Error("results should be a new slice")
	}
	cloned.Results[0].Metrics["latency"] = 999
	if original.Results[0].Metrics["latency"] == 999 {
		t.Error("modifying clone metrics should not affect original")
	}

	cloned.Results[0].Tags[0] = "modified"
	if original.Results[0].Tags[0] == "modified" {
		t.Error("modifying clone tags should not affect original")
	}
}

func TestCloneChecks(t *testing.T) {
	checks := []CheckConfig{
		{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"},
		{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"},
	}

	cloned := cloneChecks(checks)

	if len(cloned) != len(checks) {
		t.Errorf("length mismatch: %d vs %d", len(cloned), len(checks))
	}

	if &cloned[0] == &checks[0] {
		t.Error("should create new slice, not copy reference")
	}
}

func TestCloneResults(t *testing.T) {
	now := time.Now().UTC()
	results := []CheckResult{
		{ID: "r1", CheckID: "c1", Status: "healthy", Metrics: map[string]float64{"latency": 100}, Tags: []string{"api"}, StartedAt: now, FinishedAt: now},
	}

	cloned := cloneResults(results)

	if len(cloned) != len(results) {
		t.Errorf("length mismatch: %d vs %d", len(cloned), len(results))
	}

	// Verify deep clone of metrics
	cloned[0].Metrics["latency"] = 999
	if results[0].Metrics["latency"] == 999 {
		t.Error("metrics should be deeply cloned")
	}

	// Verify deep clone of tags
	cloned[0].Tags[0] = "modified"
	if results[0].Tags[0] == "modified" {
		t.Error("tags should be deeply cloned")
	}

	// Test nil
	if cloneResults(nil) != nil {
		t.Error("expected nil for nil input")
	}

	// Test empty
	empty := cloneResults([]CheckResult{})
	if len(empty) != 0 {
		t.Error("expected empty slice")
	}
}

func TestPruneResultsSorts(t *testing.T) {
	now := time.Now().UTC()
	results := []CheckResult{
		{ID: "r3", CheckID: "c1", FinishedAt: now.Add(-3 * time.Hour)},
		{ID: "r1", CheckID: "c1", FinishedAt: now.Add(-1 * time.Hour)},
		{ID: "r2", CheckID: "c1", FinishedAt: now.Add(-2 * time.Hour)},
	}

	pruneResults(&results, 7)

	// Should be sorted by FinishedAt (oldest first)
	if results[0].ID != "r3" {
		t.Errorf("expected r3 first, got %s", results[0].ID)
	}
	if results[1].ID != "r2" {
		t.Errorf("expected r2 second, got %s", results[1].ID)
	}
	if results[2].ID != "r1" {
		t.Errorf("expected r1 third, got %s", results[2].ID)
	}
}

func TestPruneResultsZeroRetention(t *testing.T) {
	now := time.Now().UTC()
	results := []CheckResult{
		{ID: "r1", CheckID: "c1", FinishedAt: now.Add(-365 * 24 * time.Hour)}, // Very old
		{ID: "r2", CheckID: "c1", FinishedAt: now},
	}

	pruneResults(&results, 0)

	// Zero retention means keep everything
	if len(results) != 2 {
		t.Errorf("expected 2 results with zero retention, got %d", len(results))
	}
}

func TestPruneResultsUsesStartedAtWhenFinishedAtZero(t *testing.T) {
	now := time.Now().UTC()
	results := []CheckResult{
		{ID: "r1", CheckID: "c1", StartedAt: now.Add(-48 * time.Hour), FinishedAt: now.Add(-48 * time.Hour)},
		{ID: "r2", CheckID: "c1", StartedAt: now.Add(-12 * time.Hour), FinishedAt: time.Time{}}, // Zero FinishedAt, within retention
	}

	pruneResults(&results, 1)

	// r2 should be kept because it uses StartedAt which is within retention period
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != "r2" {
		t.Errorf("expected r2 to remain, got %s", results[0].ID)
	}
}
