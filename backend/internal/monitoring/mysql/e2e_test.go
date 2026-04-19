package mysql

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
	"medics-health-check/backend/internal/monitoring/ai"
	"medics-health-check/backend/internal/monitoring/notify"
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

// MockMySQLSampler implements monitoring.MySQLSampler with configurable scenarios.
type MockMySQLSampler struct {
	Scenario  MockScenario
	ErrorMsg  string
	callCount int
}

func (m *MockMySQLSampler) Collect(ctx context.Context, check monitoring.CheckConfig) (monitoring.MySQLSample, error) {
	m.callCount++

	if m.Scenario == ScenarioError {
		msg := m.ErrorMsg
		if msg == "" {
			msg = "mock error"
		}
		return monitoring.MySQLSample{}, fmt.Errorf("%s", msg)
	}

	now := time.Now().UTC()
	sample := monitoring.MySQLSample{
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
	check := monitoring.CheckConfig{ID: checkID, Name: "MySQL Prod", Type: "mysql"}

	// --- Set up repositories ---
	t.Log("Step 1: Create all repositories and engines")

	mysqlRepo, err := NewFileMySQLRepository(filepath.Join(tmpDir, "mysql"))
	if err != nil {
		t.Fatalf("NewFileMySQLRepository: %v", err)
	}

	snapRepo, err := monitoring.NewFileSnapshotRepository(filepath.Join(tmpDir, "snapshots", "snapshots.jsonl"))
	if err != nil {
		t.Fatalf("NewFileSnapshotRepository: %v", err)
	}

	outbox, err := notify.NewFileNotificationOutbox(filepath.Join(tmpDir, "outbox", "outbox.jsonl"))
	if err != nil {
		t.Fatalf("NewFileNotificationOutbox: %v", err)
	}

	aiQueue, err := ai.NewFileAIQueue(filepath.Join(tmpDir, "ai"))
	if err != nil {
		t.Fatalf("NewFileAIQueue: %v", err)
	}

	rules := monitoring.DefaultMySQLRules()
	ruleEngine, err := monitoring.NewMySQLRuleEngine(rules, filepath.Join(tmpDir, "rules"))
	if err != nil {
		t.Fatalf("NewMySQLRuleEngine: %v", err)
	}

	incidentRepo := monitoring.NewMemoryIncidentRepository()
	incidentMgr := monitoring.NewIncidentManager(incidentRepo, log.Default())

	evidenceCollector := monitoring.NewMySQLEvidenceCollector(mysqlRepo)

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
	var openResult *monitoring.EvaluateResult
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
	if err := notify.EnqueueIncidentNotification(outbox, incident, "email"); err != nil {
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
		makeSample          func(breached bool) monitoring.MySQLSample
		makeDelta           func(breached bool) *monitoring.MySQLDelta
		consecutiveBreaches int
		recoverySamples     int
	}

	checkID := "mysql-streak-test"

	scenarios := []ruleScenario{
		{
			ruleCode:            "CONN_UTIL_WARN",
			consecutiveBreaches: 3, recoverySamples: 3,
			makeSample: func(breached bool) monitoring.MySQLSample {
				s := monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.Connections = 750 // 75% > 70%
				} else {
					s.Connections = 100 // 10%
				}
				return s
			},
			makeDelta: func(_ bool) *monitoring.MySQLDelta { return &monitoring.MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "CONN_UTIL_CRIT",
			consecutiveBreaches: 2, recoverySamples: 3,
			makeSample: func(breached bool) monitoring.MySQLSample {
				s := monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.Connections = 950 // 95% > 90%
				} else {
					s.Connections = 50
				}
				return s
			},
			makeDelta: func(_ bool) *monitoring.MySQLDelta { return &monitoring.MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "MAX_CONN_REFUSED",
			consecutiveBreaches: 1, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
				if breached {
					d.ConnectionsRefusedDelta = 5
				}
				return d
			},
		},
		{
			ruleCode:            "ABORTED_CONNECT_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
				if breached {
					d.AbortedConnectsPerSec = 10 // >= 5
				}
				return d
			},
		},
		{
			ruleCode:            "THREADS_RUNNING_HIGH",
			consecutiveBreaches: 3, recoverySamples: 3,
			makeSample: func(breached bool) monitoring.MySQLSample {
				s := monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
				if breached {
					s.ThreadsRunning = 60 // >= 50
				} else {
					s.ThreadsRunning = 5
				}
				return s
			},
			makeDelta: func(_ bool) *monitoring.MySQLDelta { return &monitoring.MySQLDelta{CheckID: checkID} },
		},
		{
			ruleCode:            "ROW_LOCK_WAITS_HIGH",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
				if breached {
					d.RowLockWaitsPerSec = 15 // >= 10
				}
				return d
			},
		},
		{
			ruleCode:            "SLOW_QUERY_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
				if breached {
					d.SlowQueriesPerSec = 5 // >= 2
				}
				return d
			},
		},
		{
			ruleCode:            "TMP_DISK_PCT_HIGH",
			consecutiveBreaches: 5, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
				if breached {
					d.TmpDiskTablesPct = 30 // >= 25
				}
				return d
			},
		},
		{
			ruleCode:            "THREAD_CREATE_SPIKE",
			consecutiveBreaches: 3, recoverySamples: 5,
			makeSample: func(_ bool) monitoring.MySQLSample {
				return monitoring.MySQLSample{CheckID: checkID, MaxConnections: 1000, Timestamp: time.Now().UTC()}
			},
			makeDelta: func(breached bool) *monitoring.MySQLDelta {
				d := &monitoring.MySQLDelta{CheckID: checkID}
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
			var targetRule monitoring.AlertRule
			for _, r := range monitoring.DefaultMySQLRules() {
				if r.RuleCode == sc.ruleCode {
					targetRule = r
					break
				}
			}
			if targetRule.ID == "" {
				t.Fatalf("rule %s not found in DefaultMySQLRules()", sc.ruleCode)
			}
			singleRuleSet := []monitoring.AlertRule{targetRule}

			engine, err := monitoring.NewMySQLRuleEngine(singleRuleSet, filepath.Join(tmpDir, "rules"))
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
			t.Logf("Verifying OK resets breach streak for %s", sc.ruleCode)
			okSample := sc.makeSample(false)
			okDelta := sc.makeDelta(false)
			engine.Evaluate(checkID, okSample, okDelta)

			// Now breach again — should need full consecutive count again
			for i := 1; i <= sc.consecutiveBreaches; i++ {
				sample := sc.makeSample(true)
				delta := sc.makeDelta(true)
				results := engine.Evaluate(checkID, sample, delta)

				if i < sc.consecutiveBreaches {
					for _, r := range results {
						if r.RuleCode == sc.ruleCode && r.Action == "open" {
							t.Fatalf("after reset, should not open after %d/%d breaches", i, sc.consecutiveBreaches)
						}
					}
				}
			}

			// --- Recovery phase ---
			t.Logf("Verifying recovery for %s — need %d consecutive OK", sc.ruleCode, sc.recoverySamples)

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
