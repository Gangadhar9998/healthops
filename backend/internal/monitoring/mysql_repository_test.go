package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeSample(checkID, sampleID string, ts time.Time) MySQLSample {
	return MySQLSample{
		SampleID:       sampleID,
		CheckID:        checkID,
		Timestamp:      ts,
		Connections:    50,
		MaxConnections: 100,
		Questions:      1000,
		SlowQueries:    5,
	}
}

func TestFileMySQLRepository_AppendAndLatest(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	s1 := makeSample("mysql-1", "s1", now.Add(-30*time.Second))
	s2 := makeSample("mysql-1", "s2", now)

	id1, err := repo.AppendSample(s1)
	if err != nil {
		t.Fatalf("append s1: %v", err)
	}
	if id1 != "s1" {
		t.Errorf("expected id s1, got %s", id1)
	}

	_, err = repo.AppendSample(s2)
	if err != nil {
		t.Fatalf("append s2: %v", err)
	}

	latest, err := repo.LatestSample("mysql-1")
	if err != nil {
		t.Fatalf("latest sample: %v", err)
	}
	if latest.SampleID != "s2" {
		t.Errorf("expected latest s2, got %s", latest.SampleID)
	}
}

func TestFileMySQLRepository_LatestSampleNotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	_, err = repo.LatestSample("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent check")
	}
}

func TestFileMySQLRepository_AutoGeneratesSampleID(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	s := MySQLSample{
		CheckID:   "mysql-1",
		Timestamp: time.Now().UTC(),
	}
	id, err := repo.AppendSample(s)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if id == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestFileMySQLRepository_RecentSamples(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		s := makeSample("mysql-1", "", now.Add(time.Duration(i)*time.Second))
		s.SampleID = "" // auto-generate
		_, _ = repo.AppendSample(s)
	}

	// Add samples for different check
	for i := 0; i < 3; i++ {
		s := makeSample("mysql-2", "", now.Add(time.Duration(i)*time.Second))
		s.SampleID = ""
		_, _ = repo.AppendSample(s)
	}

	samples, err := repo.RecentSamples("mysql-1", 3)
	if err != nil {
		t.Fatalf("recent samples: %v", err)
	}
	if len(samples) != 3 {
		t.Errorf("expected 3 samples, got %d", len(samples))
	}
	for _, s := range samples {
		if s.CheckID != "mysql-1" {
			t.Errorf("expected checkID mysql-1, got %s", s.CheckID)
		}
	}
}

func TestFileMySQLRepository_ComputeAndAppendDelta(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	s1 := MySQLSample{
		SampleID:  "s1",
		CheckID:   "mysql-1",
		Timestamp: now.Add(-15 * time.Second),
		Questions: 1000,
	}
	s2 := MySQLSample{
		SampleID:  "s2",
		CheckID:   "mysql-1",
		Timestamp: now,
		Questions: 1150,
	}

	_, _ = repo.AppendSample(s1)
	_, _ = repo.AppendSample(s2)

	delta, err := repo.ComputeAndAppendDelta("s2")
	if err != nil {
		t.Fatalf("compute delta: %v", err)
	}

	if delta.QuestionsDelta != 150 {
		t.Errorf("expected questions delta 150, got %d", delta.QuestionsDelta)
	}

	deltas, err := repo.RecentDeltas("mysql-1", 10)
	if err != nil {
		t.Fatalf("recent deltas: %v", err)
	}
	if len(deltas) != 1 {
		t.Errorf("expected 1 delta, got %d", len(deltas))
	}
}

func TestFileMySQLRepository_ComputeDeltaNoSample(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	_, err = repo.ComputeAndAppendDelta("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sample")
	}
}

func TestFileMySQLRepository_ComputeDeltaNoPrevious(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	s := MySQLSample{SampleID: "s1", CheckID: "mysql-1", Timestamp: time.Now().UTC()}
	_, _ = repo.AppendSample(s)

	_, err = repo.ComputeAndAppendDelta("s1")
	if err == nil {
		t.Fatal("expected error with no previous sample")
	}
}

func TestFileMySQLRepository_PruneBefore(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	old := MySQLSample{SampleID: "old", CheckID: "mysql-1", Timestamp: now.Add(-48 * time.Hour)}
	recent := MySQLSample{SampleID: "recent", CheckID: "mysql-1", Timestamp: now}

	_, _ = repo.AppendSample(old)
	_, _ = repo.AppendSample(recent)

	cutoff := now.Add(-24 * time.Hour)
	if err := repo.PruneBefore(cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	samples, err := repo.RecentSamples("mysql-1", 10)
	if err != nil {
		t.Fatalf("recent samples: %v", err)
	}
	if len(samples) != 1 {
		t.Errorf("expected 1 sample after prune, got %d", len(samples))
	}
	if samples[0].SampleID != "recent" {
		t.Errorf("expected 'recent' sample to survive, got %s", samples[0].SampleID)
	}
}

func TestFileMySQLRepository_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create and populate
	repo1, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	s := makeSample("mysql-1", "persist-test", time.Now().UTC())
	_, _ = repo1.AppendSample(s)

	// Verify JSONL file exists
	samplesPath := filepath.Join(dir, "mysql_samples.jsonl")
	if _, err := os.Stat(samplesPath); os.IsNotExist(err) {
		t.Fatal("samples JSONL file not created")
	}

	// Reload from disk
	repo2, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("reload repo: %v", err)
	}

	latest, err := repo2.LatestSample("mysql-1")
	if err != nil {
		t.Fatalf("latest after reload: %v", err)
	}
	if latest.SampleID != "persist-test" {
		t.Errorf("expected persist-test, got %s", latest.SampleID)
	}
}

func TestFileMySQLRepository_RecentSamplesDefaultLimit(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 25; i++ {
		s := MySQLSample{CheckID: "mysql-1", Timestamp: now.Add(time.Duration(i) * time.Second)}
		_, _ = repo.AppendSample(s)
	}

	// limit=0 should use default of 20
	samples, err := repo.RecentSamples("mysql-1", 0)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(samples) != 20 {
		t.Errorf("expected default limit 20, got %d", len(samples))
	}
}
