package monitoring

import (
	"testing"
	"time"
)

func TestCloneStateCopiesMutableFields(t *testing.T) {
	enabled := true
	original := State{
		Checks: []CheckConfig{{
			ID:       "check-1",
			Enabled:  &enabled,
			Tags:     []string{"api"},
			Metadata: map[string]string{"owner": "ops"},
			SSH:      &SSHCheckConfig{Metrics: []string{"cpu"}},
		}},
		Results: []CheckResult{{
			ID:      "result-1",
			CheckID: "check-1",
			Metrics: map[string]float64{
				"latency": 12,
			},
			Tags: []string{"prod"},
		}},
	}

	cloned := cloneState(original)
	*cloned.Checks[0].Enabled = false
	cloned.Checks[0].Tags[0] = "mutated"
	cloned.Checks[0].Metadata["owner"] = "mutated"
	cloned.Checks[0].SSH.Metrics[0] = "mutated"
	cloned.Results[0].Metrics["latency"] = 99
	cloned.Results[0].Tags[0] = "mutated"

	if !*original.Checks[0].Enabled {
		t.Fatal("clone mutated original enabled pointer")
	}
	if original.Checks[0].Tags[0] != "api" {
		t.Fatalf("clone mutated original tags: %v", original.Checks[0].Tags)
	}
	if original.Checks[0].Metadata["owner"] != "ops" {
		t.Fatalf("clone mutated original metadata: %v", original.Checks[0].Metadata)
	}
	if original.Checks[0].SSH.Metrics[0] != "cpu" {
		t.Fatalf("clone mutated original ssh metrics: %v", original.Checks[0].SSH.Metrics)
	}
	if original.Results[0].Metrics["latency"] != 12 {
		t.Fatalf("clone mutated original metrics: %v", original.Results[0].Metrics)
	}
	if original.Results[0].Tags[0] != "prod" {
		t.Fatalf("clone mutated original result tags: %v", original.Results[0].Tags)
	}
}

func TestPruneResultsDropsExpiredResults(t *testing.T) {
	now := time.Now().UTC()
	results := []CheckResult{
		{ID: "old", FinishedAt: now.Add(-48 * time.Hour)},
		{ID: "new", FinishedAt: now},
	}

	pruneResults(&results, 1)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ID != "new" {
		t.Fatalf("remaining result = %q, want new", results[0].ID)
	}
}
