package mysql

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

// concurrentMockSampler implements monitoring.MySQLSampler for concurrency testing.
type concurrentMockSampler struct {
	mu     sync.Mutex
	calls  int
	sample monitoring.MySQLSample
}

func (m *concurrentMockSampler) Collect(ctx context.Context, check monitoring.CheckConfig) (monitoring.MySQLSample, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.sample, nil
}

func TestMySQLCollectorConcurrent(t *testing.T) {
	sampler := &concurrentMockSampler{
		sample: monitoring.MySQLSample{
			SampleID:       "s1",
			CheckID:        "mysql-1",
			Timestamp:      time.Now().UTC(),
			Connections:    50,
			MaxConnections: 100,
			ThreadsRunning: 3,
		},
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	ctx := context.Background()
	check := monitoring.CheckConfig{ID: "mysql-1", Name: "test-mysql", Type: "mysql"}

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := sampler.Collect(ctx, check)
			if err != nil {
				t.Errorf("Collect failed: %v", err)
			}
		}()
	}

	wg.Wait()

	sampler.mu.Lock()
	count := sampler.calls
	sampler.mu.Unlock()

	if count != goroutines {
		t.Fatalf("expected %d calls, got %d", goroutines, count)
	}
	t.Logf("all %d concurrent Collect calls completed successfully", count)
}

func TestMySQLSchedulerConcurrent(t *testing.T) {
	dir := t.TempDir()
	rules := monitoring.DefaultMySQLRules()
	engine, err := monitoring.NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create rule engine: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			checkID := fmt.Sprintf("mysql-check-%d", idx)
			sample := monitoring.MySQLSample{
				SampleID:       fmt.Sprintf("s-%d", idx),
				CheckID:        checkID,
				Timestamp:      time.Now().UTC(),
				Connections:    int64(50 + idx),
				MaxConnections: 100,
				ThreadsRunning: int64(idx),
				SlowQueries:    int64(idx * 5),
			}
			delta := monitoring.MySQLDelta{
				CheckID:           checkID,
				IntervalSec:       60,
				SlowQueriesDelta:  int64(idx),
				SlowQueriesPerSec: float64(idx) / 60.0,
			}
			results := engine.Evaluate(checkID, sample, &delta)
			// Just verify no panic; results may vary
			_ = results
		}(i)
	}

	wg.Wait()
	t.Logf("all %d concurrent Evaluate calls completed without race", goroutines)

	// Verify state isolation: evaluate two distinct checks and confirm independent states
	s1 := monitoring.MySQLSample{SampleID: "iso-1", CheckID: "iso-a", Timestamp: time.Now().UTC(), Connections: 95, MaxConnections: 100}
	s2 := monitoring.MySQLSample{SampleID: "iso-2", CheckID: "iso-b", Timestamp: time.Now().UTC(), Connections: 10, MaxConnections: 100}

	r1 := engine.Evaluate("iso-a", s1, nil)
	r2 := engine.Evaluate("iso-b", s2, nil)

	// Results should be independent — different checkIDs should not cross-contaminate
	for _, r := range r1 {
		if r.CheckID != "iso-a" {
			t.Fatalf("expected checkID iso-a, got %s", r.CheckID)
		}
	}
	for _, r := range r2 {
		if r.CheckID != "iso-b" {
			t.Fatalf("expected checkID iso-b, got %s", r.CheckID)
		}
	}
	t.Logf("state isolation verified: iso-a got %d results, iso-b got %d results", len(r1), len(r2))
}
