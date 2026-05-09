package monitoring

import (
	"context"
	"fmt"
	"time"
)

const mongoStoreTimeout = 10 * time.Second

// MongoStore is the authoritative MongoDB-backed Store implementation.
// It intentionally has no local file fallback.
type MongoStore struct {
	mirror Mirror
}

var _ Store = (*MongoStore)(nil)

func NewMongoStore(mirror Mirror, seedChecks []CheckConfig) (*MongoStore, error) {
	if mirror == nil {
		return nil, fmt.Errorf("mongo store requires a mirror")
	}

	store := &MongoStore{mirror: mirror}
	state, err := store.readState()
	if err != nil {
		return nil, fmt.Errorf("read initial mongo state: %w", err)
	}

	if isEmptyState(state) && len(seedChecks) > 0 {
		seeded := State{
			Checks:    cloneChecksDeep(seedChecks),
			UpdatedAt: time.Now().UTC(),
		}
		if err := store.syncState(seeded); err != nil {
			return nil, fmt.Errorf("seed mongo state: %w", err)
		}
	}

	return store, nil
}

func (s *MongoStore) Snapshot() State {
	state, err := s.readState()
	if err != nil {
		return State{}
	}
	return cloneStateDeep(state)
}

func (s *MongoStore) DashboardSnapshot() DashboardSnapshot {
	ctx, cancel := context.WithTimeout(context.Background(), mongoStoreTimeout)
	snapshot, err := s.mirror.ReadDashboardSnapshot(ctx)
	cancel()
	if err == nil && !isEmptyDashboardSnapshot(snapshot) {
		return cloneDashboardSnapshot(snapshot)
	}

	state := s.Snapshot()
	snapshot = buildDashboardSnapshot(state)
	snapshot.GeneratedAt = time.Now().UTC()
	return cloneDashboardSnapshot(snapshot)
}

func (s *MongoStore) Update(mutator func(*State) error) error {
	current, err := s.readState()
	if err != nil {
		return err
	}

	next := cloneStateDeep(current)
	if err := mutator(&next); err != nil {
		return err
	}
	next.UpdatedAt = time.Now().UTC()
	return s.syncState(next)
}

func (s *MongoStore) ReplaceChecks(checks []CheckConfig) error {
	return s.Update(func(state *State) error {
		state.Checks = cloneChecksDeep(checks)
		return nil
	})
}

func (s *MongoStore) UpsertCheck(check CheckConfig) error {
	return s.Update(func(state *State) error {
		check = cloneCheckDeep(check)
		for i := range state.Checks {
			if state.Checks[i].ID == check.ID {
				state.Checks[i] = check
				return nil
			}
		}
		state.Checks = append(state.Checks, check)
		return nil
	})
}

func (s *MongoStore) DeleteCheck(id string) error {
	return s.Update(func(state *State) error {
		out := state.Checks[:0]
		for _, check := range state.Checks {
			if check.ID != id {
				out = append(out, check)
			}
		}
		state.Checks = out
		return nil
	})
}

func (s *MongoStore) AppendResults(results []CheckResult, retentionDays int) error {
	return s.Update(func(state *State) error {
		state.Results = append(state.Results, cloneResultsDeep(results)...)
		pruneResults(&state.Results, retentionDays)
		return nil
	})
}

func (s *MongoStore) SetLastRun(at time.Time) error {
	return s.Update(func(state *State) error {
		state.LastRunAt = at.UTC()
		return nil
	})
}

func (s *MongoStore) Ping(ctx context.Context) error {
	type pinger interface {
		Ping(context.Context) error
	}
	if mirror, ok := s.mirror.(pinger); ok {
		return mirror.Ping(ctx)
	}
	_, err := s.mirror.ReadState(ctx)
	return err
}

func (s *MongoStore) readState() (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mongoStoreTimeout)
	defer cancel()
	state, err := s.mirror.ReadState(ctx)
	if err != nil {
		return State{}, err
	}
	return cloneStateDeep(state), nil
}

func (s *MongoStore) syncState(state State) error {
	ctx, cancel := context.WithTimeout(context.Background(), mongoStoreTimeout)
	defer cancel()
	return s.mirror.SyncState(ctx, cloneStateDeep(state))
}

func cloneDashboardSnapshot(snapshot DashboardSnapshot) DashboardSnapshot {
	return DashboardSnapshot{
		State:       cloneStateDeep(snapshot.State),
		Summary:     cloneSummaryDeep(snapshot.Summary),
		GeneratedAt: snapshot.GeneratedAt,
	}
}

func cloneStateDeep(state State) State {
	return State{
		Checks:    cloneChecksDeep(state.Checks),
		Results:   cloneResultsDeep(state.Results),
		LastRunAt: state.LastRunAt,
		UpdatedAt: state.UpdatedAt,
	}
}

func cloneChecksDeep(checks []CheckConfig) []CheckConfig {
	if len(checks) == 0 {
		return nil
	}
	out := make([]CheckConfig, len(checks))
	for i := range checks {
		out[i] = cloneCheckDeep(checks[i])
	}
	return out
}

func cloneCheckDeep(check CheckConfig) CheckConfig {
	out := check
	if check.Enabled != nil {
		enabled := *check.Enabled
		out.Enabled = &enabled
	}
	out.Tags = cloneStringSlice(check.Tags)
	out.NotificationChannelIDs = cloneStringSlice(check.NotificationChannelIDs)
	out.Metadata = cloneStringMapMonitoring(check.Metadata)
	if check.MySQL != nil {
		mysql := *check.MySQL
		out.MySQL = &mysql
	}
	if check.SSH != nil {
		ssh := *check.SSH
		ssh.Metrics = cloneStringSlice(check.SSH.Metrics)
		out.SSH = &ssh
	}
	return out
}

func cloneResultsDeep(results []CheckResult) []CheckResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]CheckResult, len(results))
	for i := range results {
		out[i] = results[i]
		out[i].Metrics = cloneFloatMap(results[i].Metrics)
		out[i].Tags = cloneStringSlice(results[i].Tags)
	}
	return out
}

func cloneSummaryDeep(summary Summary) Summary {
	out := summary
	out.ByServer = cloneStatusCountMap(summary.ByServer)
	out.ByApplication = cloneStatusCountMap(summary.ByApplication)
	out.Latest = cloneResultsDeep(summary.Latest)
	return out
}

func cloneStringMapMonitoring(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStatusCountMap(in map[string]StatusCount) map[string]StatusCount {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]StatusCount, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
