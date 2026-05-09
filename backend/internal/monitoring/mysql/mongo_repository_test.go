package mysql

import (
	"context"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
	"medics-health-check/backend/internal/util/mongotest"
)

func makeSample(checkID, sampleID string, ts time.Time) monitoring.MySQLSample {
	return monitoring.MySQLSample{
		SampleID:       sampleID,
		CheckID:        checkID,
		Timestamp:      ts,
		Connections:    50,
		MaxConnections: 100,
		Questions:      1000,
		SlowQueries:    5,
		ThreadsRunning: 3,
	}
}

func TestMongoMySQLRepositoryPersistsSamplesAndDeltasNewestFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mongotest.Connect(t, 2*time.Second)
	repo, err := NewMongoMySQLRepository(client, "healthops_test", "mysql_metrics_test")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.samples.Drop(ctx)
		_ = repo.deltas.Drop(ctx)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	s1 := makeSample("mysql-1", "s1", now.Add(-30*time.Second))
	s1.Questions = 1000
	s1.SlowQueries = 5
	s2 := makeSample("mysql-1", "s2", now.Add(-10*time.Second))
	s2.Questions = 1120
	s2.SlowQueries = 7
	other := makeSample("mysql-2", "other", now)

	if id, err := repo.SaveSample(s1); err != nil {
		t.Fatalf("save s1: %v", err)
	} else if id != "s1" {
		t.Fatalf("expected id s1, got %s", id)
	}
	if _, err := repo.AppendSample(s2); err != nil {
		t.Fatalf("append s2: %v", err)
	}
	if _, err := repo.AppendSample(other); err != nil {
		t.Fatalf("append other: %v", err)
	}

	latest, err := repo.LatestSample("mysql-1")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.SampleID != "s2" {
		t.Fatalf("expected latest s2, got %s", latest.SampleID)
	}

	samples, err := repo.RecentSamples("mysql-1", 2)
	if err != nil {
		t.Fatalf("recent samples: %v", err)
	}
	if got := sampleIDs(samples); len(got) != 2 || got[0] != "s2" || got[1] != "s1" {
		t.Fatalf("expected newest-first samples [s2 s1], got %v", got)
	}

	delta, err := repo.ComputeAndAppendDelta("s2")
	if err != nil {
		t.Fatalf("compute delta: %v", err)
	}
	if delta.QuestionsDelta != 120 {
		t.Fatalf("expected questions delta 120, got %d", delta.QuestionsDelta)
	}
	if err := repo.SaveDelta(monitoring.MySQLDelta{
		SampleID:       "manual",
		CheckID:        "mysql-1",
		Timestamp:      now.Add(5 * time.Second),
		QuestionsDelta: 1,
	}); err != nil {
		t.Fatalf("save manual delta: %v", err)
	}

	deltas, err := repo.RecentDeltas("mysql-1", 2)
	if err != nil {
		t.Fatalf("recent deltas: %v", err)
	}
	if got := deltaIDs(deltas); len(got) != 2 || got[0] != "manual" || got[1] != "s2" {
		t.Fatalf("expected newest-first deltas [manual s2], got %v", got)
	}
}

func TestMongoMySQLRepositoryDefaultsIDsLimitsAndMissingErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mongotest.Connect(t, 2*time.Second)
	repo, err := NewMongoMySQLRepository(client, "healthops_test", "mysql_metrics_defaults_test")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.samples.Drop(ctx)
		_ = repo.deltas.Drop(ctx)
	})

	if _, err := repo.LatestSample("missing"); err == nil {
		t.Fatal("expected missing latest sample error")
	}
	if _, err := repo.ComputeAndAppendDelta("missing"); err == nil {
		t.Fatal("expected missing delta source error")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < 25; i++ {
		id, err := repo.AppendSample(monitoring.MySQLSample{
			CheckID:   "mysql-1",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("append sample %d: %v", i, err)
		}
		if id == "" {
			t.Fatalf("sample %d did not get an id", i)
		}
	}

	samples, err := repo.RecentSamples("mysql-1", 0)
	if err != nil {
		t.Fatalf("recent samples: %v", err)
	}
	if len(samples) != 20 {
		t.Fatalf("expected default limit 20, got %d", len(samples))
	}
	if !samples[0].Timestamp.After(samples[len(samples)-1].Timestamp) {
		t.Fatalf("expected newest-first samples, got first %s last %s", samples[0].Timestamp, samples[len(samples)-1].Timestamp)
	}
}

func TestMongoMySQLRepositoryComputeDeltaNoPrevious(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mongotest.Connect(t, 2*time.Second)
	repo, err := NewMongoMySQLRepository(client, "healthops_test", "mysql_metrics_no_previous_test")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.samples.Drop(ctx)
		_ = repo.deltas.Drop(ctx)
	})

	if _, err := repo.AppendSample(monitoring.MySQLSample{
		SampleID:  "only",
		CheckID:   "mysql-1",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append sample: %v", err)
	}
	if _, err := repo.ComputeAndAppendDelta("only"); err == nil {
		t.Fatal("expected no previous sample error")
	}
}

func TestMongoMySQLRepositoryPruneBefore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mongotest.Connect(t, 2*time.Second)
	repo, err := NewMongoMySQLRepository(client, "healthops_test", "mysql_metrics_prune_test")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.samples.Drop(ctx)
		_ = repo.deltas.Drop(ctx)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	old := monitoring.MySQLSample{SampleID: "old", CheckID: "mysql-1", Timestamp: now.Add(-48 * time.Hour)}
	recent := monitoring.MySQLSample{SampleID: "recent", CheckID: "mysql-1", Timestamp: now}
	if _, err := repo.AppendSample(old); err != nil {
		t.Fatalf("append old sample: %v", err)
	}
	if _, err := repo.AppendSample(recent); err != nil {
		t.Fatalf("append recent sample: %v", err)
	}
	if err := repo.SaveDelta(monitoring.MySQLDelta{SampleID: "old-delta", CheckID: "mysql-1", Timestamp: now.Add(-48 * time.Hour)}); err != nil {
		t.Fatalf("append old delta: %v", err)
	}
	if err := repo.SaveDelta(monitoring.MySQLDelta{SampleID: "recent-delta", CheckID: "mysql-1", Timestamp: now}); err != nil {
		t.Fatalf("append recent delta: %v", err)
	}

	if err := repo.PruneBefore(now.Add(-24 * time.Hour)); err != nil {
		t.Fatalf("prune: %v", err)
	}

	samples, err := repo.RecentSamples("mysql-1", 10)
	if err != nil {
		t.Fatalf("recent samples: %v", err)
	}
	if got := sampleIDs(samples); len(got) != 1 || got[0] != "recent" {
		t.Fatalf("expected only recent sample, got %v", got)
	}
	deltas, err := repo.RecentDeltas("mysql-1", 10)
	if err != nil {
		t.Fatalf("recent deltas: %v", err)
	}
	if got := deltaIDs(deltas); len(got) != 1 || got[0] != "recent-delta" {
		t.Fatalf("expected only recent delta, got %v", got)
	}
}

func sampleIDs(samples []monitoring.MySQLSample) []string {
	ids := make([]string, len(samples))
	for i, sample := range samples {
		ids[i] = sample.SampleID
	}
	return ids
}

func deltaIDs(deltas []monitoring.MySQLDelta) []string {
	ids := make([]string, len(deltas))
	for i, delta := range deltas {
		ids[i] = delta.SampleID
	}
	return ids
}
