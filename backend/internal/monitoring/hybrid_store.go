package monitoring

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type HybridStore struct {
	local       *FileStore
	mirror      Mirror
	logger      *log.Logger
	readTimeout time.Duration
	syncTimeout time.Duration
	syncMu      sync.Mutex
	mongoDown   atomic.Bool // tracks whether MongoDB is currently unreachable
}

var _ Store = (*HybridStore)(nil)

func NewHybridStore(statePath string, checks []CheckConfig, mongoURI, mongoDB, mongoPrefix string, retentionDays int, logger *log.Logger) (*HybridStore, error) {
	local, err := NewFileStore(statePath, checks)
	if err != nil {
		return nil, err
	}

	store := &HybridStore{
		local:       local,
		logger:      logger,
		readTimeout: 5 * time.Second,
		syncTimeout: 30 * time.Second,
	}

	if mongoURI != "" {
		mirror, err := NewMongoMirror(mongoURI, mongoDB, mongoPrefix, retentionDays)
		if err != nil {
			if logger != nil {
				logger.Printf("WARNING: MongoDB unavailable at startup, running with local file only: %v", err)
			}
			store.mongoDown.Store(true)
		} else {
			store.mirror = mirror
			// Seed local file from Mongo if local is empty (Mongo is primary)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			mongoState, readErr := mirror.ReadState(ctx)
			cancel()
			if readErr == nil && !isEmptyState(mongoState) && isEmptyState(local.Snapshot()) {
				_ = local.Update(func(s *State) error {
					s.Checks = mongoState.Checks
					s.Results = mongoState.Results
					s.LastRunAt = mongoState.LastRunAt
					s.UpdatedAt = mongoState.UpdatedAt
					return nil
				})
				if logger != nil {
					logger.Printf("Seeded local file from MongoDB (%d checks, %d results)", len(mongoState.Checks), len(mongoState.Results))
				}
			} else if readErr == nil {
				// Sync current local state to Mongo (non-blocking)
				go func() {
					if err := store.syncToMongo(); err != nil && logger != nil {
						logger.Printf("initial Mongo sync skipped: %v", err)
					}
				}()
			}
		}
	}

	return store, nil
}

// IsMongoDown returns true if MongoDB is currently unreachable.
func (s *HybridStore) IsMongoDown() bool {
	return s.mongoDown.Load()
}

// HasMongo returns true if MongoDB was configured (even if currently down).
func (s *HybridStore) HasMongo() bool {
	return s.mirror != nil
}

// Client returns the underlying MongoDB client, or nil if not configured.
func (s *HybridStore) Client() *mongo.Client {
	if s.mirror == nil {
		return nil
	}
	if mm, ok := s.mirror.(*MongoMirror); ok {
		return mm.Client()
	}
	return nil
}

// PingMongo checks MongoDB connectivity. Returns nil if reachable.
func (s *HybridStore) PingMongo(ctx context.Context) error {
	if s.mirror == nil {
		return nil
	}
	if mm, ok := s.mirror.(*MongoMirror); ok {
		return mm.Ping(ctx)
	}
	return nil
}

func (s *HybridStore) Snapshot() State {
	// Try Mongo first (primary), fall back to local
	if s.mirror != nil && !s.mongoDown.Load() {
		ctx, cancel := context.WithTimeout(context.Background(), s.readTimeout)
		state, err := s.mirror.ReadState(ctx)
		cancel()
		if err == nil && !isEmptyState(state) {
			return state
		}
	}
	return s.local.Snapshot()
}

func (s *HybridStore) DashboardSnapshot() DashboardSnapshot {
	// Try Mongo first (primary), fall back to local
	if s.mirror != nil && !s.mongoDown.Load() {
		ctx, cancel := context.WithTimeout(context.Background(), s.readTimeout)
		snapshot, err := s.mirror.ReadDashboardSnapshot(ctx)
		cancel()
		if err == nil && !isEmptyDashboardSnapshot(snapshot) {
			return snapshot
		}
	}
	return s.local.DashboardSnapshot()
}

func (s *HybridStore) Update(mutator func(*State) error) error {
	// Always update local (never lose data)
	if err := s.local.Update(mutator); err != nil {
		return err
	}
	// Sync to Mongo (primary) — best effort
	s.syncToMongo()
	return nil
}

func (s *HybridStore) ReplaceChecks(checks []CheckConfig) error {
	if err := s.local.ReplaceChecks(checks); err != nil {
		return err
	}
	s.syncToMongo()
	return nil
}

func (s *HybridStore) UpsertCheck(check CheckConfig) error {
	if err := s.local.UpsertCheck(check); err != nil {
		return err
	}
	s.syncToMongo()
	return nil
}

func (s *HybridStore) DeleteCheck(id string) error {
	if err := s.local.DeleteCheck(id); err != nil {
		return err
	}
	s.syncToMongo()
	return nil
}

func (s *HybridStore) AppendResults(results []CheckResult, retentionDays int) error {
	if err := s.local.AppendResults(results, retentionDays); err != nil {
		return err
	}
	s.syncToMongo()
	return nil
}

func (s *HybridStore) SetLastRun(at time.Time) error {
	if err := s.local.SetLastRun(at); err != nil {
		return err
	}
	s.syncToMongo()
	return nil
}

// syncToMongo syncs local state to MongoDB. Sets mongoDown flag on failure.
func (s *HybridStore) syncToMongo() error {
	if s.mirror == nil {
		return nil
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
	defer cancel()

	if err := s.mirror.SyncState(ctx, s.local.Snapshot()); err != nil {
		if !s.mongoDown.Load() {
			s.mongoDown.Store(true)
			if s.logger != nil {
				s.logger.Printf("ERROR: MongoDB sync failed — marked as down: %v", err)
			}
		}
		return err
	}

	// Mongo is reachable again
	if s.mongoDown.Load() {
		s.mongoDown.Store(false)
		if s.logger != nil {
			s.logger.Printf("MongoDB connectivity restored")
		}
	}
	return nil
}

func isEmptyState(state State) bool {
	return len(state.Checks) == 0 && len(state.Results) == 0 && state.LastRunAt.IsZero() && state.UpdatedAt.IsZero()
}

func isEmptyDashboardSnapshot(snapshot DashboardSnapshot) bool {
	return isEmptyState(snapshot.State) && snapshot.Summary.TotalChecks == 0 && snapshot.GeneratedAt.IsZero()
}
