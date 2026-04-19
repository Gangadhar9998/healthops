package monitoring

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSnapshotRepository_SaveAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.jsonl")

	repo, err := NewFileSnapshotRepository(path)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	snaps := []IncidentSnapshot{
		{SnapshotType: "latest_sample", Timestamp: now, PayloadJSON: `{"test":1}`},
		{SnapshotType: "processlist", Timestamp: now, PayloadJSON: `[{"id":1}]`},
	}

	if err := repo.SaveSnapshots("inc-1", snaps); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.GetSnapshots("inc-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(got))
	}
	for _, s := range got {
		if s.IncidentID != "inc-1" {
			t.Errorf("expected incidentID inc-1, got %s", s.IncidentID)
		}
	}
}

func TestFileSnapshotRepository_GetNonexistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.jsonl")

	repo, err := NewFileSnapshotRepository(path)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	got, err := repo.GetSnapshots("nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(got))
	}
}

func TestFileSnapshotRepository_PruneBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.jsonl")

	repo, err := NewFileSnapshotRepository(path)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	old := []IncidentSnapshot{
		{SnapshotType: "old", Timestamp: now.Add(-48 * time.Hour), PayloadJSON: `{}`},
	}
	recent := []IncidentSnapshot{
		{SnapshotType: "recent", Timestamp: now, PayloadJSON: `{}`},
	}

	_ = repo.SaveSnapshots("inc-old", old)
	_ = repo.SaveSnapshots("inc-new", recent)

	cutoff := now.Add(-24 * time.Hour)
	if err := repo.PruneBefore(cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	all, _ := repo.GetSnapshots("inc-old")
	if len(all) != 0 {
		t.Errorf("expected old snapshots pruned, got %d", len(all))
	}

	all, _ = repo.GetSnapshots("inc-new")
	if len(all) != 1 {
		t.Errorf("expected recent snapshot to survive, got %d", len(all))
	}
}

func TestFileSnapshotRepository_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.jsonl")

	repo1, _ := NewFileSnapshotRepository(path)
	_ = repo1.SaveSnapshots("inc-1", []IncidentSnapshot{
		{SnapshotType: "test", Timestamp: time.Now().UTC(), PayloadJSON: `{"persist":true}`},
	})

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("JSONL file not created")
	}

	// Reload
	repo2, err := NewFileSnapshotRepository(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	got, _ := repo2.GetSnapshots("inc-1")
	if len(got) != 1 {
		t.Fatalf("expected 1 snapshot after reload, got %d", len(got))
	}
	if got[0].PayloadJSON != `{"persist":true}` {
		t.Errorf("unexpected payload: %s", got[0].PayloadJSON)
	}
}

func TestMakeSnapshot(t *testing.T) {
	now := time.Now().UTC()
	snap := makeSnapshot("inc-1", "test_type", now, map[string]int{"count": 5})

	if snap.IncidentID != "inc-1" {
		t.Errorf("expected incidentID inc-1, got %s", snap.IncidentID)
	}
	if snap.SnapshotType != "test_type" {
		t.Errorf("expected type test_type, got %s", snap.SnapshotType)
	}
	if snap.PayloadJSON == "" {
		t.Error("expected non-empty payload")
	}
}

func TestMakeErrorSnapshot(t *testing.T) {
	now := time.Now().UTC()
	snap := makeErrorSnapshot("inc-1", "failed_query", now, fmt.Errorf("connection refused"))

	if snap.SnapshotType != "failed_query" {
		t.Errorf("expected type failed_query, got %s", snap.SnapshotType)
	}
	if snap.PayloadJSON == "" {
		t.Error("expected non-empty error payload")
	}
}

// Test MySQLEvidenceCollector with mock repository (no real DB)
func TestMySQLEvidenceCollector_CaptureEvidenceNoDB(t *testing.T) {
	repo := &mockMySQLRepo{
		latestSample: MySQLSample{
			SampleID: "s1",
			CheckID:  "mysql-1",
		},
		recentDeltas: []MySQLDelta{
			{SampleID: "d1", CheckID: "mysql-1"},
		},
	}

	collector := NewMySQLEvidenceCollector(repo)
	snaps := collector.CaptureEvidence(nil, "inc-1", "mysql-1", nil)

	// Without DB, should get latest_sample + recent_deltas = 2 snapshots
	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots without DB, got %d", len(snaps))
	}
}

func TestMySQLEvidenceCollector_CaptureEvidenceWithErrors(t *testing.T) {
	repo := &mockMySQLRepo{
		latestErr: fmt.Errorf("no samples"),
		deltasErr: fmt.Errorf("no deltas"),
	}

	collector := NewMySQLEvidenceCollector(repo)
	snaps := collector.CaptureEvidence(nil, "inc-1", "mysql-1", nil)

	// Should get 2 error snapshots
	if len(snaps) != 2 {
		t.Errorf("expected 2 error snapshots, got %d", len(snaps))
	}
	for _, s := range snaps {
		if s.PayloadJSON == "" {
			t.Error("error snapshot should have payload")
		}
	}
}

// Mock MySQL repository for testing
type mockMySQLRepo struct {
	latestSample MySQLSample
	latestErr    error
	recentDeltas []MySQLDelta
	deltasErr    error
}

func (m *mockMySQLRepo) AppendSample(s MySQLSample) (string, error) { return s.SampleID, nil }
func (m *mockMySQLRepo) ComputeAndAppendDelta(id string) (MySQLDelta, error) {
	return MySQLDelta{}, nil
}
func (m *mockMySQLRepo) LatestSample(checkID string) (MySQLSample, error) {
	return m.latestSample, m.latestErr
}
func (m *mockMySQLRepo) RecentSamples(checkID string, limit int) ([]MySQLSample, error) {
	return nil, nil
}
func (m *mockMySQLRepo) RecentDeltas(checkID string, limit int) ([]MySQLDelta, error) {
	return m.recentDeltas, m.deltasErr
}
