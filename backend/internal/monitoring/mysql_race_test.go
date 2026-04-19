package monitoring

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// concurrentMockSampler implements MySQLSampler for concurrency testing.
type concurrentMockSampler struct {
	mu     sync.Mutex
	calls  int
	sample MySQLSample
}

func (m *concurrentMockSampler) Collect(ctx context.Context, check CheckConfig) (MySQLSample, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.sample, nil
}

func TestMySQLCollectorConcurrent(t *testing.T) {
	sampler := &concurrentMockSampler{
		sample: MySQLSample{
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
	check := CheckConfig{ID: "mysql-1", Name: "test-mysql", Type: "mysql"}

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

	// Test FileMySQLRepository concurrently: 10 writers, 10 readers
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	const repoWorkers = 10
	var wg2 sync.WaitGroup
	wg2.Add(repoWorkers * 2)

	// 10 writers appending samples
	for i := 0; i < repoWorkers; i++ {
		go func(idx int) {
			defer wg2.Done()
			s := MySQLSample{
				CheckID:        "mysql-1",
				Timestamp:      time.Now().UTC(),
				Connections:    int64(idx * 10),
				MaxConnections: 200,
			}
			_, err := repo.AppendSample(s)
			if err != nil {
				t.Errorf("AppendSample failed: %v", err)
			}
		}(i)
	}

	// 10 readers calling RecentSamples and LatestSample
	for i := 0; i < repoWorkers; i++ {
		go func() {
			defer wg2.Done()
			_, _ = repo.RecentSamples("mysql-1", 5)
			_, _ = repo.LatestSample("mysql-1")
		}()
	}

	wg2.Wait()
	t.Logf("FileMySQLRepository concurrent read/write completed without race")
}

func TestMySQLSchedulerConcurrent(t *testing.T) {
	dir := t.TempDir()
	rules := DefaultMySQLRules()
	engine, err := NewMySQLRuleEngine(rules, dir)
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
			sample := MySQLSample{
				SampleID:       fmt.Sprintf("s-%d", idx),
				CheckID:        checkID,
				Timestamp:      time.Now().UTC(),
				Connections:    int64(50 + idx),
				MaxConnections: 100,
				ThreadsRunning: int64(idx),
				SlowQueries:    int64(idx * 5),
			}
			delta := MySQLDelta{
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
	s1 := MySQLSample{SampleID: "iso-1", CheckID: "iso-a", Timestamp: time.Now().UTC(), Connections: 95, MaxConnections: 100}
	s2 := MySQLSample{SampleID: "iso-2", CheckID: "iso-b", Timestamp: time.Now().UTC(), Connections: 10, MaxConnections: 100}

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

func TestMySQLOutboxConcurrent(t *testing.T) {
	dir := t.TempDir()
	outboxPath := filepath.Join(dir, "outbox.jsonl")
	outbox, err := NewFileNotificationOutbox(outboxPath)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	const (
		enqueuers = 10
		listers   = 5
		markers   = 5
	)

	var wg sync.WaitGroup
	wg.Add(enqueuers + listers + markers)

	// 10 goroutines enqueueing events
	for i := 0; i < enqueuers; i++ {
		go func(idx int) {
			defer wg.Done()
			evt := NotificationEvent{
				IncidentID: fmt.Sprintf("inc-%d", idx),
				Channel:    "slack",
			}
			if err := outbox.Enqueue(evt); err != nil {
				t.Errorf("Enqueue failed: %v", err)
			}
		}(i)
	}

	// 5 goroutines listing pending
	for i := 0; i < listers; i++ {
		go func() {
			defer wg.Done()
			_, err := outbox.ListPending(10)
			if err != nil {
				t.Errorf("ListPending failed: %v", err)
			}
		}()
	}

	// 5 goroutines marking sent (may fail if ID doesn't exist yet — that's OK)
	for i := 0; i < markers; i++ {
		go func(idx int) {
			defer wg.Done()
			_ = outbox.MarkSent(fmt.Sprintf("notif-inc-%d-0", idx))
		}(i)
	}

	wg.Wait()
	t.Logf("FileNotificationOutbox concurrent test completed without race")

	// Test FileAIQueue concurrently
	aiDir := t.TempDir()
	aiQueue, err := NewFileAIQueue(aiDir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	const (
		aiEnqueuers  = 10
		aiClaimers   = 5
		aiCompleters = 5
	)

	var wg2 sync.WaitGroup
	wg2.Add(aiEnqueuers + aiClaimers + aiCompleters)

	// 10 goroutines enqueueing
	for i := 0; i < aiEnqueuers; i++ {
		go func(idx int) {
			defer wg2.Done()
			if err := aiQueue.Enqueue(fmt.Sprintf("ai-inc-%d", idx), "v1"); err != nil {
				t.Errorf("AIQueue Enqueue failed: %v", err)
			}
		}(i)
	}

	// 5 goroutines claiming
	for i := 0; i < aiClaimers; i++ {
		go func() {
			defer wg2.Done()
			_, err := aiQueue.ClaimPending(3)
			if err != nil {
				t.Errorf("AIQueue ClaimPending failed: %v", err)
			}
		}()
	}

	// 5 goroutines completing (may fail if incident not found — that's OK)
	for i := 0; i < aiCompleters; i++ {
		go func(idx int) {
			defer wg2.Done()
			result := AIAnalysisResult{
				IncidentID: fmt.Sprintf("ai-inc-%d", idx),
				Analysis:   "test analysis",
				CreatedAt:  time.Now().UTC(),
			}
			_ = aiQueue.Complete(fmt.Sprintf("ai-inc-%d", idx), result)
		}(i)
	}

	wg2.Wait()
	t.Logf("FileAIQueue concurrent test completed without race")
}

func TestMySQLRepositoryConcurrentReadWrite(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	const (
		writers         = 10
		samplesPerWrite = 10
		readers         = 10
	)

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	// 10 writer goroutines each appending 10 samples
	for w := 0; w < writers; w++ {
		go func(wIdx int) {
			defer wg.Done()
			for s := 0; s < samplesPerWrite; s++ {
				sample := MySQLSample{
					CheckID:        fmt.Sprintf("check-%d", wIdx),
					Timestamp:      time.Now().UTC(),
					Connections:    int64(wIdx*10 + s),
					MaxConnections: 200,
					ThreadsRunning: int64(s),
				}
				_, err := repo.AppendSample(sample)
				if err != nil {
					t.Errorf("writer %d sample %d: AppendSample failed: %v", wIdx, s, err)
				}
			}
		}(w)
	}

	// 10 reader goroutines reading concurrently
	for r := 0; r < readers; r++ {
		go func(rIdx int) {
			defer wg.Done()
			checkID := fmt.Sprintf("check-%d", rIdx)
			for i := 0; i < samplesPerWrite; i++ {
				_, _ = repo.RecentSamples(checkID, 5)
				_, _ = repo.LatestSample(checkID)
			}
		}(r)
	}

	wg.Wait()

	// Verify total sample count after all writers finish
	totalSamples := 0
	for w := 0; w < writers; w++ {
		checkID := fmt.Sprintf("check-%d", w)
		samples, err := repo.RecentSamples(checkID, 1000)
		if err != nil {
			t.Fatalf("RecentSamples for %s failed: %v", checkID, err)
		}
		totalSamples += len(samples)
	}

	expected := writers * samplesPerWrite
	if totalSamples != expected {
		t.Fatalf("expected %d total samples, got %d", expected, totalSamples)
	}
	t.Logf("verified %d total samples across %d checks", totalSamples, writers)
}

func TestMySQLSnapshotRepoConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.jsonl")
	repo, err := NewFileSnapshotRepository(path)
	if err != nil {
		t.Fatalf("create snapshot repo: %v", err)
	}

	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 10 goroutines saving snapshots
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			incID := fmt.Sprintf("inc-%d", idx)
			snaps := []IncidentSnapshot{
				{
					IncidentID:   incID,
					SnapshotType: "mysql_sample",
					Timestamp:    time.Now().UTC(),
					PayloadJSON:  fmt.Sprintf(`{"index":%d}`, idx),
				},
			}
			if err := repo.SaveSnapshots(incID, snaps); err != nil {
				t.Errorf("SaveSnapshots failed for %s: %v", incID, err)
			}
		}(i)
	}

	// 10 goroutines reading snapshots
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			incID := fmt.Sprintf("inc-%d", idx)
			_, err := repo.GetSnapshots(incID)
			if err != nil {
				t.Errorf("GetSnapshots failed for %s: %v", incID, err)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("FileSnapshotRepository concurrent test completed without race")
}
