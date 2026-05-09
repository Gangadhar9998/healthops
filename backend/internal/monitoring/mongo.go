package monitoring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoMirror struct {
	client        *mongo.Client
	db            *mongo.Database
	checks        *mongo.Collection
	results       *mongo.Collection
	state         *mongo.Collection
	dashboard     *mongo.Collection
	retentionDays int
}

func NewMongoMirror(uri, dbName, prefix string, retentionDays int) (*MongoMirror, error) {
	if uri == "" {
		return nil, fmt.Errorf("mongo uri is required")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	// Force IPv4: replace localhost with 127.0.0.1 to avoid IPv6 socket issues on macOS
	uri = strings.ReplaceAll(uri, "localhost", "127.0.0.1")

	// Configure client options with longer timeouts for remote connections
	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetMaxPoolSize(100)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect failed: %w", err)
	}

	// Ping with timeout to verify connection
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping failed: %w", err)
	}

	return NewMongoMirrorFromClient(client, dbName, prefix, retentionDays)
}

func NewMongoMirrorFromClient(client *mongo.Client, dbName, prefix string, retentionDays int) (*MongoMirror, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo client is required")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	mirror := &MongoMirror{
		client:        client,
		db:            client.Database(dbName),
		checks:        client.Database(dbName).Collection(prefix + "_checks"),
		results:       client.Database(dbName).Collection(prefix + "_results"),
		state:         client.Database(dbName).Collection(prefix + "_state"),
		dashboard:     client.Database(dbName).Collection(prefix + "_dashboard"),
		retentionDays: retentionDays,
	}

	indexCtx, indexCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer indexCancel()

	if err := mirror.ensureIndexes(indexCtx); err != nil {
		return nil, fmt.Errorf("create mongodb mirror indexes: %w", err)
	}

	return mirror, nil
}

func (m *MongoMirror) ensureIndexes(ctx context.Context) error {
	if _, err := m.checks.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "type", Value: 1}}},
		{Keys: bson.D{{Key: "server", Value: 1}}},
		{Keys: bson.D{{Key: "enabled", Value: 1}}},
	}); err != nil && !indexAlreadyExists(err) {
		return err
	}

	_, err := m.results.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "checkId", Value: 1}, {Key: "finishedAt", Value: -1}}},
		{Keys: bson.D{{Key: "finishedAt", Value: -1}}},
	})
	if err != nil && !indexAlreadyExists(err) {
		return err
	}

	return nil
}

func indexAlreadyExists(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}

func (m *MongoMirror) SyncState(ctx context.Context, state State) error {
	if m == nil || m.client == nil {
		return nil
	}

	if err := m.upsertChecks(ctx, state.Checks); err != nil {
		return err
	}
	if err := m.upsertResults(ctx, state.Results); err != nil {
		return err
	}
	if err := m.upsertState(ctx, state); err != nil {
		return err
	}
	if err := m.upsertDashboardSnapshot(ctx, state); err != nil {
		return err
	}
	return nil
}

func (m *MongoMirror) ReadState(ctx context.Context) (State, error) {
	if m == nil || m.client == nil {
		return State{}, fmt.Errorf("mongo mirror is not configured")
	}

	checks, err := m.readChecks(ctx)
	if err != nil {
		return State{}, err
	}
	results, err := m.readResults(ctx)
	if err != nil {
		return State{}, err
	}
	meta, err := m.readStateMeta(ctx)
	if err != nil {
		return State{}, err
	}

	return State{
		Checks:    checks,
		Results:   results,
		LastRunAt: meta.LastRunAt,
		UpdatedAt: meta.UpdatedAt,
	}, nil
}

func (m *MongoMirror) ReadDashboardSnapshot(ctx context.Context) (DashboardSnapshot, error) {
	if m == nil || m.client == nil {
		return DashboardSnapshot{}, fmt.Errorf("mongo mirror is not configured")
	}

	var snapshot DashboardSnapshot
	if err := m.dashboard.FindOne(ctx, bson.M{"_id": "dashboard"}).Decode(&snapshot); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return DashboardSnapshot{}, nil
		}
		return DashboardSnapshot{}, err
	}
	return snapshot, nil
}

func (m *MongoMirror) readChecks(ctx context.Context) ([]CheckConfig, error) {
	cur, err := m.checks.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var checks []CheckConfig
	for cur.Next(ctx) {
		var check CheckConfig
		if err := cur.Decode(&check); err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return checks, nil
}

func (m *MongoMirror) readResults(ctx context.Context) ([]CheckResult, error) {
	filter := bson.D{}
	if m.retentionDays > 0 {
		filter = bson.D{{Key: "finishedAt", Value: bson.M{"$gte": time.Now().UTC().Add(-time.Duration(m.retentionDays) * 24 * time.Hour)}}}
	}
	findOpts := options.Find().SetSort(bson.D{{Key: "finishedAt", Value: 1}})
	cur, err := m.results.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []CheckResult
	for cur.Next(ctx) {
		var result CheckResult
		if err := cur.Decode(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (m *MongoMirror) readStateMeta(ctx context.Context) (State, error) {
	var doc State
	err := m.state.FindOne(ctx, bson.M{"_id": "state"}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return State{}, nil
		}
		if errors.Is(err, io.EOF) {
			return State{}, nil
		}
		return State{}, err
	}
	return doc, nil
}

func (m *MongoMirror) upsertChecks(ctx context.Context, checks []CheckConfig) error {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.ID == "" {
			continue
		}
		ids = append(ids, check.ID)
		if _, err := m.checks.ReplaceOne(ctx, bson.M{"_id": check.ID}, check, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}
	filter := bson.M{}
	if len(ids) > 0 {
		filter = bson.M{"_id": bson.M{"$nin": ids}}
	}
	if _, err := m.checks.DeleteMany(ctx, filter); err != nil {
		return err
	}
	return nil
}

func (m *MongoMirror) upsertResults(ctx context.Context, results []CheckResult) error {
	cutoff := time.Time{}
	if m.retentionDays > 0 {
		cutoff = time.Now().UTC().Add(-time.Duration(m.retentionDays) * 24 * time.Hour)
	}

	for _, result := range results {
		if result.ID == "" {
			continue
		}
		if _, err := m.results.ReplaceOne(ctx, bson.M{"_id": result.ID}, result, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}

	if !cutoff.IsZero() {
		if _, err := m.results.DeleteMany(ctx, bson.M{"finishedAt": bson.M{"$lt": cutoff}}); err != nil {
			return err
		}
	}
	return nil
}

func (m *MongoMirror) upsertState(ctx context.Context, state State) error {
	doc := bson.M{
		"_id":    "state",
		"checks": state.Checks,
		// Don't store results here - they're already in the results collection
		// Storing results in state document causes 16MB document size limit errors
		"lastRunAt": state.LastRunAt,
		"updatedAt": state.UpdatedAt,
	}
	_, err := m.state.ReplaceOne(ctx, bson.M{"_id": "state"}, doc, options.Replace().SetUpsert(true))
	return err
}

func (m *MongoMirror) upsertDashboardSnapshot(ctx context.Context, state State) error {
	snapshot := buildDashboardSnapshot(state)
	doc := bson.M{
		"_id":         "dashboard",
		"state":       snapshot.State,
		"summary":     snapshot.Summary,
		"generatedAt": snapshot.GeneratedAt,
	}
	_, err := m.dashboard.ReplaceOne(ctx, bson.M{"_id": "dashboard"}, doc, options.Replace().SetUpsert(true))
	return err
}

// Ping checks whether MongoDB is reachable.
func (m *MongoMirror) Ping(ctx context.Context) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("mongo client is nil")
	}
	return m.client.Ping(ctx, nil)
}

// Client returns the underlying MongoDB client
func (m *MongoMirror) Client() *mongo.Client {
	if m == nil {
		return nil
	}
	return m.client
}
