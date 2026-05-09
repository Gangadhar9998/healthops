package monitoring

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoSnapshotRepository struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ IncidentSnapshotRepository = (*MongoSnapshotRepository)(nil)

func NewMongoSnapshotRepository(client *mongo.Client, dbName, prefix string) (*MongoSnapshotRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo snapshot repository requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	repo := &MongoSnapshotRepository{
		collection: client.Database(dbName).Collection(prefix + "_incident_snapshots"),
		timeout:    5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), repo.timeout)
	defer cancel()
	if _, err := repo.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "incidentId", Value: 1}, {Key: "timestamp", Value: 1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}},
	}); err != nil && !indexAlreadyExists(err) {
		return nil, fmt.Errorf("create snapshot indexes: %w", err)
	}
	return repo, nil
}

func (r *MongoSnapshotRepository) SaveSnapshots(incidentID string, snaps []IncidentSnapshot) error {
	if len(snaps) == 0 {
		return nil
	}

	docs := make([]interface{}, 0, len(snaps))
	for _, snap := range snaps {
		snap.IncidentID = incidentID
		if snap.Timestamp.IsZero() {
			snap.Timestamp = time.Now().UTC()
		}
		docs = append(docs, snap)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("insert snapshots: %w", err)
	}
	return nil
}

func (r *MongoSnapshotRepository) GetSnapshots(incidentID string) ([]IncidentSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})
	cur, err := r.collection.Find(ctx, bson.M{"incidentId": incidentID}, opts)
	if err != nil {
		return nil, fmt.Errorf("find snapshots: %w", err)
	}
	defer cur.Close(ctx)

	var snaps []IncidentSnapshot
	if err := cur.All(ctx, &snaps); err != nil {
		return nil, fmt.Errorf("decode snapshots: %w", err)
	}
	if snaps == nil {
		return []IncidentSnapshot{}, nil
	}
	return snaps, nil
}

func (r *MongoSnapshotRepository) PruneBefore(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.DeleteMany(ctx, bson.M{"timestamp": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune snapshots: %w", err)
	}
	return nil
}
