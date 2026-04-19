package monitoring

import (
	"testing"
	"time"
)

func TestComputeDelta_BasicCounterDiffs(t *testing.T) {
	prev := MySQLSample{
		SampleID:             "s1",
		CheckID:              "mysql-1",
		Timestamp:            time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		AbortedConnects:      10,
		SlowQueries:          5,
		Questions:            1000,
		InnoDBRowLockWaits:   100,
		CreatedTmpDiskTables: 50,
		CreatedTmpTables:     200,
		ThreadsCreated:       30,
		ConnectionsRefused:   0,
	}
	cur := MySQLSample{
		SampleID:             "s2",
		CheckID:              "mysql-1",
		Timestamp:            time.Date(2025, 1, 1, 0, 0, 15, 0, time.UTC), // 15s later
		AbortedConnects:      15,
		SlowQueries:          8,
		Questions:            1150,
		InnoDBRowLockWaits:   110,
		CreatedTmpDiskTables: 55,
		CreatedTmpTables:     220,
		ThreadsCreated:       35,
		ConnectionsRefused:   2,
	}

	delta := ComputeDelta(cur, prev)

	if delta.SampleID != "s2" {
		t.Errorf("expected sampleID s2, got %s", delta.SampleID)
	}
	if delta.CheckID != "mysql-1" {
		t.Errorf("expected checkID mysql-1, got %s", delta.CheckID)
	}
	if delta.IntervalSec != 15.0 {
		t.Errorf("expected interval 15s, got %f", delta.IntervalSec)
	}
	if delta.AbortedConnectsDelta != 5 {
		t.Errorf("expected aborted connects delta 5, got %d", delta.AbortedConnectsDelta)
	}
	if delta.SlowQueriesDelta != 3 {
		t.Errorf("expected slow queries delta 3, got %d", delta.SlowQueriesDelta)
	}
	if delta.QuestionsDelta != 150 {
		t.Errorf("expected questions delta 150, got %d", delta.QuestionsDelta)
	}
	if delta.RowLockWaitsDelta != 10 {
		t.Errorf("expected row lock waits delta 10, got %d", delta.RowLockWaitsDelta)
	}
	if delta.TmpDiskTablesDelta != 5 {
		t.Errorf("expected tmp disk tables delta 5, got %d", delta.TmpDiskTablesDelta)
	}
	// TmpDiskTablesPct = 5/20*100 = 25%
	if delta.TmpDiskTablesPct != 25.0 {
		t.Errorf("expected tmp disk tables pct 25.0, got %f", delta.TmpDiskTablesPct)
	}
	if delta.ThreadsCreatedDelta != 5 {
		t.Errorf("expected threads created delta 5, got %d", delta.ThreadsCreatedDelta)
	}
	if delta.ConnectionsRefusedDelta != 2 {
		t.Errorf("expected connections refused delta 2, got %d", delta.ConnectionsRefusedDelta)
	}

	// Per-second rates
	expectedAbortedPerSec := 5.0 / 15.0
	if delta.AbortedConnectsPerSec != expectedAbortedPerSec {
		t.Errorf("expected aborted per sec %f, got %f", expectedAbortedPerSec, delta.AbortedConnectsPerSec)
	}
	expectedQPS := 150.0 / 15.0
	if delta.QuestionsPerSec != expectedQPS {
		t.Errorf("expected QPS %f, got %f", expectedQPS, delta.QuestionsPerSec)
	}
}

func TestComputeDelta_CounterReset(t *testing.T) {
	prev := MySQLSample{
		SampleID:  "s1",
		CheckID:   "mysql-1",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Questions: 1000000,
	}
	cur := MySQLSample{
		SampleID:  "s2",
		CheckID:   "mysql-1",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 15, 0, time.UTC),
		Questions: 500, // counter reset — value went down
	}

	delta := ComputeDelta(cur, prev)

	if delta.QuestionsDelta != 0 {
		t.Errorf("expected counter reset to yield 0, got %d", delta.QuestionsDelta)
	}
}

func TestComputeDelta_ZeroInterval(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	prev := MySQLSample{
		SampleID:  "s1",
		CheckID:   "mysql-1",
		Timestamp: ts,
		Questions: 100,
	}
	cur := MySQLSample{
		SampleID:  "s2",
		CheckID:   "mysql-1",
		Timestamp: ts, // same timestamp
		Questions: 200,
	}

	delta := ComputeDelta(cur, prev)

	// Should not panic — interval clamped to 1
	if delta.IntervalSec != 1.0 {
		t.Errorf("expected interval clamped to 1, got %f", delta.IntervalSec)
	}
	if delta.QuestionsDelta != 100 {
		t.Errorf("expected delta 100, got %d", delta.QuestionsDelta)
	}
}

func TestComputeDelta_ZeroTmpTables(t *testing.T) {
	prev := MySQLSample{
		SampleID:  "s1",
		CheckID:   "mysql-1",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	cur := MySQLSample{
		SampleID:  "s2",
		CheckID:   "mysql-1",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 15, 0, time.UTC),
	}

	delta := ComputeDelta(cur, prev)

	// Zero tmp tables should not divide by zero
	if delta.TmpDiskTablesPct != 0 {
		t.Errorf("expected 0 pct with zero tmp tables, got %f", delta.TmpDiskTablesPct)
	}
}

func TestCounterDiff(t *testing.T) {
	tests := []struct {
		name     string
		current  int64
		previous int64
		want     int64
	}{
		{"normal increment", 100, 50, 50},
		{"no change", 50, 50, 0},
		{"counter reset", 10, 100, 0},
		{"from zero", 10, 0, 10},
		{"both zero", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := counterDiff(tt.current, tt.previous)
			if got != tt.want {
				t.Errorf("counterDiff(%d, %d) = %d, want %d", tt.current, tt.previous, got, tt.want)
			}
		})
	}
}
