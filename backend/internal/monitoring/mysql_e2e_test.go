package monitoring

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// MockMySQLSampler — configurable fake for MySQLSampler interface
// ---------------------------------------------------------------------------

// MockScenario selects the metric profile returned by MockMySQLSampler.
type MockScenario string

const (
	ScenarioHealthy  MockScenario = "healthy"
	ScenarioDegraded MockScenario = "degraded"
	ScenarioCritical MockScenario = "critical"
	ScenarioError    MockScenario = "error"
)

// MockMySQLSampler implements MySQLSampler with configurable scenarios.
type MockMySQLSampler struct {
	Scenario  MockScenario
	ErrorMsg  string
	callCount int
}

func (m *MockMySQLSampler) Collect(ctx context.Context, check CheckConfig) (MySQLSample, error) {
	m.callCount++

	if m.Scenario == ScenarioError {
		msg := m.ErrorMsg
		if msg == "" {
			msg = "mock error"
		}
		return MySQLSample{}, fmt.Errorf("%s", msg)
	}

	now := time.Now().UTC()
	sample := MySQLSample{
		SampleID:       fmt.Sprintf("%s-sample-%d", check.ID, now.UnixNano()),
		CheckID:        check.ID,
		Timestamp:      now,
		MaxConnections: 1000,
		UptimeSeconds:  86400,
		Questions:      int64(100000 + m.callCount*1000),
		SlowQueries:    0,
	}

	switch m.Scenario {
	case ScenarioHealthy:
		sample.Connections = 50
		sample.ThreadsRunning = 5
		sample.ThreadsConnected = 50
		sample.AbortedConnects = 0
		sample.AbortedClients = 0
	case ScenarioDegraded:
		sample.Connections = 750
		sample.ThreadsRunning = 30
		sample.ThreadsConnected = 750
		sample.AbortedConnects = 10
		sample.AbortedClients = 5
	case ScenarioCritical:
		sample.Connections = 950
		sample.ThreadsRunning = 80
		sample.ThreadsConnected = 950
		sample.AbortedConnects = 50
		sample.AbortedClients = 20
		sample.SlowQueries = int64(100 + m.callCount*10)
	}

	return sample, nil
}

// ---------------------------------------------------------------------------
// Test 1: Full MySQL Incident Lifecycle
// ---------------------------------------------------------------------------

func TestE2E_MySQLIncidentLifecycle(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	checkID := "mysql-prod-1"
	check := CheckConfig{ID: checkID, Name: "MySQL Prod", Type: "mysql"}

	// --- Set up repositories ---
	t.Log("Step 1: Create all repositories and engines")

	mysqlRepo, err := NewFileMySQLRepository(filepath.Join(tmpDir, "mysql"))
	if err != nil {
		t.Fatalf("NewFileMySQLRepository: %v", err)
	}

	snapRepo, err := NewFileSnapshotRepository(filepath.Join(tmpDir, "snapshots", "snapshots.jsonl"))
	if err != nil {
		t.Fatalf("NewFileSnapshotRepository: %v", err)
	}

	outbox, err := NewFileNotificationOutbox(filepath.Join(tmpDir, "outbox", "outbox.jsonl"))
	if err != nil {
		t.Fatalf("NewFileNotificationOutbox: %v", err)
	}

	aiQueue, err := NewFileAIQueue(filepath.Join(tmpDir, "ai"))
	if err != nil {
		t.Fatalf("NewFileAIQueue: %v", err)
	}

	rules := DefaultMySQLRules()
	ruleEngine, err := NewMySQLRuleEngine(rules, filepath.Join(tmpDir, "rules"))
	if err != nil {
		t.Fatalf("NewMySQLRuleEngine: %v", err)
	}

	incidentRepo := NewMemoryIncidentRepository()
	incidentMgr := NewIncidentManager(incidentRepo, log.Default())

	evidenceCollector := NewMySQLEvidenceCollector(mysqlRepo)

	// --- Phase A: Breach with critical connection utilization ---
	t.Log("Step 2: Mock critical scenario — 950/1000 connections (95%)")

	mock := &MockMySQLSampler{Scenario: ScenarioCritical}

	// We need a previous sample so we can compute deltas.
	// Collect first baseline sample (healthy).
	baselineMock := &MockMySQLSampler{Scenario: ScenarioHealthy}
	baselineSample, err := baselineMock.Collect(ctx, check)
	if err != nil {
		t.Fatalf("baseline collect: %v", err)
	}
	if _, err := mysqlRepo.AppendSample(baselineSample); err != nil {
		t.Fatalf("append baseline: %v", err)
	}

	// Small sleep to ensure timestamp difference.
	time.Sleep(10 * time.Millisecond)

	// Breach 1
	t.Log("Step 3: First breach sample")
	s1, err := mock.Collect(ctx, check)
	if err != nil {
		t.Fatalf("collect breach 1: %v", err)
	}
	sid1, err := mysqlRepo.AppendSample(s1)
	if err != nil {
		t.Fatalf("append breach 1: %v", err)
	}
	delta1, err := mysqlRepo.ComputeAndAppendDelta(sid1)
	if err != nil {
		t.Fatalf("delta breach 1: %v", err)
	}

	results1 := ruleEngine.Evaluate(checkID, s1, &delta1)
	// CONN_UTIL_CRIT needs 2 consecutive breaches; first breach should NOT open.
	for _, r := range results1 {
		if r.RuleCode == "CONN_UTIL_CRIT" && r.Action == "open" {
			t.Fatalf("CONN_UTIL_CRIT should not open after 1 breach")
		}
	}

	time.Sleep(10 * time.Millisecond)

	// Breach 2
	t.Log("Step 4: Second breach sample — should trigger open")
	s2, err := mock.Collect(ctx, check)
	if err != nil {
		t.Fatalf("collect breach 2: %v", err)
	}
	sid2, err := mysqlRepo.AppendSample(s2)
	if err != nil {
		t.Fatalf("append breach 2: %v", err)
	}
	delta2, err := mysqlRepo.ComputeAndAppendDelta(sid2)
	if err != nil {
		t.Fatalf("delta breach 2: %v", err)
	}

	results2 := ruleEngine.Evaluate(checkID, s2, &delta2)
	var openResult *EvaluateResult
	for i := range results2 {
		if results2[i].RuleCode == "CONN_UTIL_CRIT" && results2[i].Action == "open" {
			openResult = &results2[i]
			break
		}
	}
	if openResult == nil {
		t.Fatalf("CONN_UTIL_CRIT should open after 2 consecutive breaches")
	}

	// --- Phase B: Create incident, record evidence, queue notifications ---
	t.Log("Step 5: Create incident via IncidentManager")

	err = incidentMgr.ProcessAlert(checkID, check.Name, check.Type, openResult.Severity, openResult.Message, nil)
	if err != nil {
		t.Fatalf("ProcessAlert: %v", err)
	}

	incident, err := incidentRepo.FindOpenIncident(checkID)
	if err != nil {
		t.Fatalf("FindOpenIncident: %v", err)
	}
	if incident.ID == "" {
		t.Fatalf("expected open incident, got none")
	}
	t.Logf("Incident created: %s", incident.ID)

	ruleEngine.SetOpenIncidentID("CONN_UTIL_CRIT", checkID, incident.ID)

	t.Log("Step 6: Capture evidence (nil db)")
	snaps := evidenceCollector.CaptureEvidence(ctx, incident.ID, checkID, nil)
	if len(snaps) < 2 {
		t.Fatalf("expected at least 2 evidence snapshots, got %d", len(snaps))
	}

	if err := snapRepo.SaveSnapshots(incident.ID, snaps); err != nil {
		t.Fatalf("SaveSnapshots: %v", err)
	}

	savedSnaps, err := snapRepo.GetSnapshots(incident.ID)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(savedSnaps) < 2 {
		t.Fatalf("expected at least 2 saved snapshots, got %d", len(savedSnaps))
	}

	t.Log("Step 7: Enqueue notification")
	if err := EnqueueIncidentNotification(outbox, incident, "email"); err != nil {
		t.Fatalf("EnqueueIncidentNotification: %v", err)
	}

	pending, err := outbox.ListPending(10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending notification, got %d", len(pending))
	}

	t.Log("Step 8: Enqueue AI analysis")
	if err := aiQueue.Enqueue(incident.ID, "v1"); err != nil {
		t.Fatalf("Enqueue AI: %v", err)
	}

	aiPending, err := aiQueue.ListPendingItems(10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(aiPending) != 1 {
		t.Fatalf("expected 1 pending AI item, got %d", len(aiPending))
	}

	// --- Phase C: Recovery ---
	t.Log("Step 9: Switch to healthy — begin recovery")

	mock.Scenario = ScenarioHealthy

	// CONN_UTIL_CRIT needs RecoverySamples=3.
	for i := 1; i <= 3; i++ {
		time.Sleep(10 * time.Millisecond)
		sr, err := mock.Collect(ctx, check)
		if err != nil {
			t.Fatalf("recovery collect %d: %v", i, err)
		}
		sid, err := mysqlRepo.AppendSample(sr)
		if err != nil {
			t.Fatalf("recovery append %d: %v", i, err)
		}
		dr, err := mysqlRepo.ComputeAndAppendDelta(sid)
		if err != nil {
			t.Fatalf("recovery delta %d: %v", i, err)
		}

		evalResults := ruleEngine.Evaluate(checkID, sr, &dr)

		if i < 3 {
			for _, r := range evalResults {
				if r.RuleCode == "CONN_UTIL_CRIT" && r.Action == "close" {
					t.Fatalf("CONN_UTIL_CRIT should not close after %d recovery samples", i)
				}
			}
		} else {
			var closeFound bool
			for _, r := range evalResults {
				if r.RuleCode == "CONN_UTIL_CRIT" && r.Action == "close" {
					closeFound = true
					if r.IncidentID != incident.ID {
						t.Fatalf("close incident ID mismatch: got %s want %s", r.IncidentID, incident.ID)
					}
				}
			}
			if !closeFound {
				t.Fatalf("CONN_UTIL_CRIT should close after 3 recovery samples")
			}
		}
	}

	t.Log("Step 10: Full lifecycle completed successfully")
}

// ---------------------------------------------------------------------------
// Test 2: Rule Streak Logic for All 9 Rules
// ---------------------------------------------------------------------------

func TestE2E_MySQLRuleStreaks(t *testing.T) {
	type ruleScenario struct {
		ruleCode            string
		makeSample          func(breached bool) MySQLSample
		makeDelta           func(breached bool) *MySQLDelta
		consecutiveBreaches int
		recoverySamples     int
	}

	checkID := "mysql-streak-test"

	scenarios := []ruleScenario{
		{
			ruleCode:            "CONN_UTIL_WARN",
			consecutiveBreaches: 3, recoverySamples: 3,
			makeSample: func(breached bool) MySQLSample {
				s := MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.Connections = 750 // 75% > 70%
				} else {
					s.Connections = 100 // 10%
				}
				return s
			},
			makeDelta: func(_ bool) *MySQLDelta { return &MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "CONN_UTIL_CRIT",
			consecutiveBreaches: 2, recoverySamples: 3,
			makeSample: func(breached bool) MySQLSample {
				s := MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.Connections = 950 // 95% > 90%
				} else {
					s.Connections = 50
				}
				return s
			},
			makeDelta: func(_ bool) *MySQLDelta { return &MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "MAX_CONN_REFUSED",
			consecutiveBreaches: 1, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.ConnectionsRefusedDelta = 5
				}
				return d
			},
		},
		{
			ruleCode:            "ABORTED_CONNECT_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.AbortedConnectsPerSec = 10 // >= 5
				}
				return d
			},
		},
		{
			ruleCode:            "THREADS_RUNNING_HIGH",
			consecutiveBreaches: 3, recoverySamples: 3,
			makeSample: func(breached bool) MySQLSample {
				s := MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.ThreadsRunning = 60 // >= 50
				} else {
					s.ThreadsRunning = 5
				}
				return s
			},
			makeDelta: func(_ bool) *MySQLDelta { return &MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "ROW_LOCK_WAITS_HIGH",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.RowLockWaitsPerSec = 15 // >= 10
				}
				return d
			},
		},
		{
			ruleCode:            "SLOW_QUERY_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.SlowQueriesPerSec = 5 // >= 2
				}
				return d
			},
		},
		{
			ruleCode:            "TMP_DISK_PCT_HIGH",
			consecutiveBreaches: 5, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.TmpDiskTablesPct = 30 // >= 25
				}
				return d
			},
		},
		{
			ruleCode:            "THREAD_CREATE_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) MySQLSample {
				return MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *MySQLDelta {
				d := &MySQLDelta{CheckID: checkID}
				if breached {
					d.ThreadsCreatedPerSec = 15 // >= 10
				}
				return d
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.ruleCode, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create engine with ONLY this rule enabled
			var targetRule AlertRule
			for _, r := range DefaultMySQLRules() {
				if r.RuleCode == sc.ruleCode {
					targetRule = r
					break
				}
			}
			if targetRule.ID == "" {
				t.Fatalf("rule %s not found in DefaultMySQLRules()", sc.ruleCode)
			}
			singleRuleSet := []AlertRule{targetRule}

			engine, err := NewMySQLRuleEngine(singleRuleSet, filepath.Join(tmpDir, "rules"))
			if err != nil {
				t.Fatalf("NewMySQLRuleEngine: %v", err)
			}

			// --- Breach phase: verify opening requires consecutive breaches ---
			t.Logf("Breaching %s — need %d consecutive", sc.ruleCode, sc.consecutiveBreaches)

			for i := 1; i <= sc.consecutiveBreaches; i++ {
				sample := sc.makeSample(true)
				delta := sc.makeDelta(true)
				results := engine.Evaluate(checkID, sample, delta)

				if i < sc.consecutiveBreaches {
					for _, r := range results {
						if r.RuleCode == sc.ruleCode && r.Action == "open" {
							t.Fatalf("should not open after %d/%d breaches", i, sc.consecutiveBreaches)
						}
					}
				} else {
					var opened bool
					for _, r := range results {
						if r.RuleCode == sc.ruleCode && r.Action == "open" {
							opened = true
						}
					}
					if !opened {
						t.Fatalf("should open after %d consecutive breaches", sc.consecutiveBreaches)
					}
				}
			}

			// --- Verify OK resets breach streak ---
			// Only meaningful when ConsecutiveBreaches > 1.
			if sc.consecutiveBreaches > 1 {
				t.Log("Sending OK to reset breach streak, then partial breach")

				engine2, err := NewMySQLRuleEngine(singleRuleSet, filepath.Join(tmpDir, "rules2"))
				if err != nil {
					t.Fatalf("NewMySQLRuleEngine reset test: %v", err)
				}

				// Breach N-1 times
				for i := 1; i < sc.consecutiveBreaches; i++ {
					engine2.Evaluate(checkID, sc.makeSample(true), sc.makeDelta(true))
				}

				// One OK resets
				engine2.Evaluate(checkID, sc.makeSample(false), sc.makeDelta(false))

				// One more breach should not open (streak was reset)
				results := engine2.Evaluate(checkID, sc.makeSample(true), sc.makeDelta(true))
				for _, r := range results {
					if r.RuleCode == sc.ruleCode && r.Action == "open" {
						t.Fatalf("OK sample should have reset breach streak — should not open")
					}
				}
			} else {
				t.Log("ConsecutiveBreaches=1 — streak reset test not applicable")
			}

			// --- Recovery phase: verify closing requires recovery samples ---
			// Use first engine which already has an OPEN state.
			engine.SetOpenIncidentID(sc.ruleCode, checkID, "inc-test-"+sc.ruleCode)

			t.Logf("Recovering %s — need %d consecutive OK", sc.ruleCode, sc.recoverySamples)

			for i := 1; i <= sc.recoverySamples; i++ {
				sample := sc.makeSample(false)
				delta := sc.makeDelta(false)
				results := engine.Evaluate(checkID, sample, delta)

				if i < sc.recoverySamples {
					for _, r := range results {
						if r.RuleCode == sc.ruleCode && r.Action == "close" {
							t.Fatalf("should not close after %d/%d recovery samples", i, sc.recoverySamples)
						}
					}
				} else {
					var closed bool
					for _, r := range results {
						if r.RuleCode == sc.ruleCode && r.Action == "close" {
							closed = true
						}
					}
					if !closed {
						t.Fatalf("should close after %d recovery samples", sc.recoverySamples)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 3: Evidence Capture
// ---------------------------------------------------------------------------

func TestE2E_MySQLEvidenceCapture(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	checkID := "mysql-evidence-check"
	incidentID := "inc-evidence-001"

	t.Log("Step 1: Set up repository with sample data")
	mysqlRepo, err := NewFileMySQLRepository(filepath.Join(tmpDir, "mysql"))
	if err != nil {
		t.Fatalf("NewFileMySQLRepository: %v", err)
	}

	// Insert a baseline sample + a current sample so LatestSample and RecentDeltas work.
	baseline := MySQLSample{
		SampleID: "baseline-1", CheckID: checkID, Timestamp: time.Now().UTC().Add(-60 * time.Second),
		Connections: 100, MaxConnections: 1000, Questions: 50000, UptimeSeconds: 86000,
	}
	if _, err := mysqlRepo.AppendSample(baseline); err != nil {
		t.Fatalf("append baseline: %v", err)
	}

	current := MySQLSample{
		SampleID: "current-1", CheckID: checkID, Timestamp: time.Now().UTC(),
		Connections: 900, MaxConnections: 1000, Questions: 55000, UptimeSeconds: 86060,
		SlowQueries: 10, ThreadsRunning: 40,
	}
	sid, err := mysqlRepo.AppendSample(current)
	if err != nil {
		t.Fatalf("append current: %v", err)
	}
	if _, err := mysqlRepo.ComputeAndAppendDelta(sid); err != nil {
		t.Fatalf("compute delta: %v", err)
	}

	t.Log("Step 2: Create evidence collector and capture with nil db")
	collector := NewMySQLEvidenceCollector(mysqlRepo)
	snaps := collector.CaptureEvidence(ctx, incidentID, checkID, nil)

	// With nil db: latest_sample + recent_deltas = 2 snapshots.
	// DB queries are skipped when db is nil.
	if len(snaps) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(snaps))
	}

	// Verify snapshot types present
	typeSet := make(map[string]bool)
	for _, s := range snaps {
		typeSet[s.SnapshotType] = true
	}
	if !typeSet["latest_sample"] {
		t.Fatalf("missing latest_sample snapshot")
	}
	if !typeSet["recent_deltas"] {
		t.Fatalf("missing recent_deltas snapshot")
	}

	t.Log("Step 3: Save to FileSnapshotRepository and verify round-trip")
	snapRepo, err := NewFileSnapshotRepository(filepath.Join(tmpDir, "snaps", "snaps.jsonl"))
	if err != nil {
		t.Fatalf("NewFileSnapshotRepository: %v", err)
	}

	if err := snapRepo.SaveSnapshots(incidentID, snaps); err != nil {
		t.Fatalf("SaveSnapshots: %v", err)
	}

	loaded, err := snapRepo.GetSnapshots(incidentID)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(loaded) != len(snaps) {
		t.Fatalf("expected %d snapshots from repo, got %d", len(snaps), len(loaded))
	}

	// Verify each snapshot has non-empty payload
	for _, s := range loaded {
		if s.PayloadJSON == "" {
			t.Fatalf("snapshot %s has empty payload", s.SnapshotType)
		}
		if s.IncidentID != incidentID {
			t.Fatalf("snapshot incident ID mismatch: got %s want %s", s.IncidentID, incidentID)
		}
	}

	t.Log("Step 4: Verify snapshot payloads contain valid JSON")
	for _, s := range loaded {
		if s.PayloadJSON[0] != '{' && s.PayloadJSON[0] != '[' {
			t.Fatalf("snapshot %s payload is not valid JSON: %s", s.SnapshotType, s.PayloadJSON[:50])
		}
	}

	t.Log("Evidence capture test completed")
}

// ---------------------------------------------------------------------------
// Test 4: Notification Outbox Flow
// ---------------------------------------------------------------------------

func TestE2E_NotificationOutboxFlow(t *testing.T) {
	tmpDir := t.TempDir()

	t.Log("Step 1: Create FileNotificationOutbox")
	outbox, err := NewFileNotificationOutbox(filepath.Join(tmpDir, "outbox.jsonl"))
	if err != nil {
		t.Fatalf("NewFileNotificationOutbox: %v", err)
	}

	// Create 3 incidents for notification
	incidents := []Incident{
		{ID: "inc-001", CheckID: "chk-1", CheckName: "DB Primary", Severity: "critical", Message: "High connections", Status: "open"},
		{ID: "inc-002", CheckID: "chk-2", CheckName: "DB Replica", Severity: "warning", Message: "Slow queries", Status: "open"},
		{ID: "inc-003", CheckID: "chk-3", CheckName: "DB Analytics", Severity: "warning", Message: "Lock waits", Status: "open"},
	}

	t.Log("Step 2: Enqueue 3 notifications")
	for _, inc := range incidents {
		if err := EnqueueIncidentNotification(outbox, inc, "email"); err != nil {
			t.Fatalf("Enqueue for %s: %v", inc.ID, err)
		}
	}

	t.Log("Step 3: ListPending should return 3")
	pending, err := outbox.ListPending(10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(pending))
	}

	// Record notification IDs for later use
	notifIDs := make([]string, len(pending))
	for i, p := range pending {
		notifIDs[i] = p.NotificationID
	}

	t.Log("Step 4: MarkSent on first notification")
	if err := outbox.MarkSent(notifIDs[0]); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	pending, err = outbox.ListPending(10)
	if err != nil {
		t.Fatalf("ListPending after MarkSent: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending after MarkSent, got %d", len(pending))
	}
	for _, p := range pending {
		if p.NotificationID == notifIDs[0] {
			t.Fatalf("sent notification should not appear in pending")
		}
	}

	t.Log("Step 5: MarkFailed on second notification")
	if err := outbox.MarkFailed(notifIDs[1], "SMTP timeout"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	pending, err = outbox.ListPending(10)
	if err != nil {
		t.Fatalf("ListPending after MarkFailed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending after MarkFailed, got %d", len(pending))
	}
	if pending[0].NotificationID != notifIDs[2] {
		t.Fatalf("expected third notification in pending, got %s", pending[0].NotificationID)
	}

	t.Log("Step 6: Idempotency — MarkSent on already-sent should error")
	err = outbox.MarkSent(notifIDs[0])
	if err == nil {
		t.Fatalf("MarkSent on already-sent notification should return error")
	}

	t.Log("Notification outbox flow completed")
}

// ---------------------------------------------------------------------------
// Test 5: AI Queue Flow
// ---------------------------------------------------------------------------

func TestE2E_AIQueueFlow(t *testing.T) {
	tmpDir := t.TempDir()

	t.Log("Step 1: Create FileAIQueue")
	aiQueue, err := NewFileAIQueue(filepath.Join(tmpDir, "ai"))
	if err != nil {
		t.Fatalf("NewFileAIQueue: %v", err)
	}

	t.Log("Step 2: Enqueue 2 items with different incident IDs")
	if err := aiQueue.Enqueue("inc-a", "v1"); err != nil {
		t.Fatalf("Enqueue inc-a: %v", err)
	}
	if err := aiQueue.Enqueue("inc-b", "v1"); err != nil {
		t.Fatalf("Enqueue inc-b: %v", err)
	}

	t.Log("Step 3: Verify dedup — same incident should not duplicate")
	if err := aiQueue.Enqueue("inc-a", "v1"); err != nil {
		t.Fatalf("Dedup enqueue should not error: %v", err)
	}

	pendingBefore, err := aiQueue.ListPendingItems(10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pendingBefore) != 2 {
		t.Fatalf("expected 2 pending items (dedup), got %d", len(pendingBefore))
	}

	t.Log("Step 4: ClaimPending — returns 2 items with status=processing")
	claimed, err := aiQueue.ClaimPending(10)
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed items, got %d", len(claimed))
	}
	for _, item := range claimed {
		if item.Status != "processing" {
			t.Fatalf("claimed item %s should be processing, got %s", item.IncidentID, item.Status)
		}
	}

	t.Log("Step 5: Complete first with AIAnalysisResult")
	result := AIAnalysisResult{
		IncidentID: "inc-a",
		Analysis:   "High connection utilization due to connection pool exhaustion",
		Suggestions: []string{
			"Increase max_connections",
			"Implement connection pooling",
			"Review slow queries",
		},
		Severity:  "critical",
		CreatedAt: time.Now().UTC(),
	}
	if err := aiQueue.Complete("inc-a", result); err != nil {
		t.Fatalf("Complete inc-a: %v", err)
	}

	t.Log("Step 6: Fail second with reason")
	if err := aiQueue.Fail("inc-b", "LLM rate limit exceeded"); err != nil {
		t.Fatalf("Fail inc-b: %v", err)
	}

	t.Log("Step 7: ListPendingItems should return 0")
	pendingAfter, err := aiQueue.ListPendingItems(10)
	if err != nil {
		t.Fatalf("ListPendingItems after complete/fail: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Fatalf("expected 0 pending items, got %d", len(pendingAfter))
	}

	t.Log("Step 8: Verify completed item has result persisted")
	// Re-load queue from disk to verify persistence
	aiQueue2, err := NewFileAIQueue(filepath.Join(tmpDir, "ai"))
	if err != nil {
		t.Fatalf("reload NewFileAIQueue: %v", err)
	}

	// Verify no pending items in reloaded queue
	reloadPending, err := aiQueue2.ListPendingItems(10)
	if err != nil {
		t.Fatalf("reload ListPendingItems: %v", err)
	}
	if len(reloadPending) != 0 {
		t.Fatalf("reloaded queue should have 0 pending, got %d", len(reloadPending))
	}

	t.Log("AI queue flow completed")
}
