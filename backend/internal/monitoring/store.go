package monitoring

import (
	"sort"
	"time"
)

func cloneState(state State) State {
	return cloneStateDeep(state)
}

func cloneChecks(checks []CheckConfig) []CheckConfig {
	return cloneChecksDeep(checks)
}

func cloneResults(results []CheckResult) []CheckResult {
	return cloneResultsDeep(results)
}

func pruneResults(results *[]CheckResult, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	items := (*results)[:0]
	for _, result := range *results {
		finishedAt := result.FinishedAt
		if finishedAt.IsZero() {
			finishedAt = result.StartedAt
		}
		if finishedAt.IsZero() || finishedAt.After(cutoff) {
			items = append(items, result)
		}
	}
	*results = items
	sort.SliceStable(*results, func(i, j int) bool {
		return (*results)[i].FinishedAt.Before((*results)[j].FinishedAt)
	})
}

func isEmptyState(state State) bool {
	return len(state.Checks) == 0 && len(state.Results) == 0 && state.LastRunAt.IsZero() && state.UpdatedAt.IsZero()
}

func isEmptyDashboardSnapshot(snapshot DashboardSnapshot) bool {
	return isEmptyState(snapshot.State) && snapshot.Summary.TotalChecks == 0 && snapshot.GeneratedAt.IsZero()
}
