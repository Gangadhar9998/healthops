package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoServerMetricsRepository struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ ServerMetricsStore = (*MongoServerMetricsRepository)(nil)

func NewMongoServerMetricsRepository(client *mongo.Client, dbName, prefix string) (*MongoServerMetricsRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo server metrics repository requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	repo := &MongoServerMetricsRepository{
		collection: client.Database(dbName).Collection(prefix + "_server_metrics"),
		timeout:    5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), repo.timeout)
	defer cancel()
	if _, err := repo.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "serverId", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}},
	}); err != nil && !indexAlreadyExists(err) {
		return nil, fmt.Errorf("create server metric indexes: %w", err)
	}
	return repo, nil
}

func (r *MongoServerMetricsRepository) Save(snap ServerSnapshot) error {
	if snap.Timestamp.IsZero() {
		snap.Timestamp = time.Now().UTC()
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.InsertOne(ctx, snap); err != nil {
		return fmt.Errorf("insert server snapshot: %w", err)
	}
	return nil
}

func (r *MongoServerMetricsRepository) GetSnapshots(serverID string, since, until time.Time) ([]ServerSnapshot, error) {
	filter := bson.M{"serverId": serverID}
	timeFilter := bson.M{}
	if !since.IsZero() {
		timeFilter["$gte"] = since
	}
	if !until.IsZero() {
		timeFilter["$lte"] = until
	}
	if len(timeFilter) > 0 {
		filter["timestamp"] = timeFilter
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})
	cur, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find server snapshots: %w", err)
	}
	defer cur.Close(ctx)

	var snaps []ServerSnapshot
	if err := cur.All(ctx, &snaps); err != nil {
		return nil, fmt.Errorf("decode server snapshots: %w", err)
	}
	if snaps == nil {
		return []ServerSnapshot{}, nil
	}
	return snaps, nil
}

func (r *MongoServerMetricsRepository) GetLatest(serverID string) (*ServerSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	var snap ServerSnapshot
	if err := r.collection.FindOne(ctx, bson.M{"serverId": serverID}, opts).Decode(&snap); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("find latest server snapshot: %w", err)
	}
	return &snap, nil
}

func (r *MongoServerMetricsRepository) PruneBefore(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.DeleteMany(ctx, bson.M{"timestamp": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune server snapshots: %w", err)
	}
	return nil
}
