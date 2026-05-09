package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"medics-health-check/backend/internal/monitoring"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const mongoRepositoryDefaultTimeout = 5 * time.Second

// MongoMySQLRepository implements monitoring.MySQLMetricsRepository with MongoDB persistence.
type MongoMySQLRepository struct {
	client  *mongo.Client
	samples *mongo.Collection
	deltas  *mongo.Collection
	timeout time.Duration
}

var _ monitoring.MySQLMetricsRepository = (*MongoMySQLRepository)(nil)

// NewMongoMySQLRepository creates a MongoDB-backed MySQL metrics repository.
func NewMongoMySQLRepository(client *mongo.Client, dbName, prefix string) (*MongoMySQLRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo mysql repository requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	repo := &MongoMySQLRepository{
		client:  client,
		samples: client.Database(dbName).Collection(prefix + "_mysql_samples"),
		deltas:  client.Database(dbName).Collection(prefix + "_mysql_deltas"),
		timeout: mongoRepositoryDefaultTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("create mysql metric indexes: %w", err)
	}

	return repo, nil
}

// SaveSample persists a sample and returns its sample ID.
func (r *MongoMySQLRepository) SaveSample(sample monitoring.MySQLSample) (string, error) {
	return r.AppendSample(sample)
}

// AppendSample persists a sample and returns its sample ID.
func (r *MongoMySQLRepository) AppendSample(sample monitoring.MySQLSample) (string, error) {
	if r == nil || r.client == nil {
		return "", fmt.Errorf("mongo mysql repository is not configured")
	}
	if sample.SampleID == "" {
		sample.SampleID = fmt.Sprintf("%s-%d", sample.CheckID, time.Now().UnixNano())
	}

	ctx, cancel := r.operationContext()
	defer cancel()
	if _, err := r.samples.InsertOne(ctx, sample); err != nil {
		return "", fmt.Errorf("insert mysql sample: %w", err)
	}
	return sample.SampleID, nil
}

// SaveDelta persists a precomputed delta.
func (r *MongoMySQLRepository) SaveDelta(delta monitoring.MySQLDelta) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("mongo mysql repository is not configured")
	}

	ctx, cancel := r.operationContext()
	defer cancel()
	if _, err := r.deltas.InsertOne(ctx, delta); err != nil {
		return fmt.Errorf("insert mysql delta: %w", err)
	}
	return nil
}

// ComputeAndAppendDelta computes a delta for sampleID against the prior sample for that check.
func (r *MongoMySQLRepository) ComputeAndAppendDelta(sampleID string) (monitoring.MySQLDelta, error) {
	if r == nil || r.client == nil {
		return monitoring.MySQLDelta{}, fmt.Errorf("mongo mysql repository is not configured")
	}

	ctx, cancel := r.operationContext()
	defer cancel()

	var current monitoring.MySQLSample
	if err := r.samples.FindOne(ctx, bson.M{"sampleId": sampleID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return monitoring.MySQLDelta{}, fmt.Errorf("sample not found: %s", sampleID)
		}
		return monitoring.MySQLDelta{}, fmt.Errorf("find mysql sample: %w", err)
	}

	filter := bson.M{
		"checkId":   current.CheckID,
		"sampleId":  bson.M{"$ne": sampleID},
		"timestamp": bson.M{"$lte": current.Timestamp},
	}
	opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}})

	var previous monitoring.MySQLSample
	if err := r.samples.FindOne(ctx, filter, opts).Decode(&previous); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return monitoring.MySQLDelta{}, fmt.Errorf("no previous sample for check %s", current.CheckID)
		}
		return monitoring.MySQLDelta{}, fmt.Errorf("find previous mysql sample: %w", err)
	}

	delta := monitoring.ComputeDelta(current, previous)
	if _, err := r.deltas.InsertOne(ctx, delta); err != nil {
		return monitoring.MySQLDelta{}, fmt.Errorf("insert mysql delta: %w", err)
	}
	return delta, nil
}

// LatestSample returns the newest sample for a check.
func (r *MongoMySQLRepository) LatestSample(checkID string) (monitoring.MySQLSample, error) {
	if r == nil || r.client == nil {
		return monitoring.MySQLSample{}, fmt.Errorf("mongo mysql repository is not configured")
	}

	ctx, cancel := r.operationContext()
	defer cancel()

	var sample monitoring.MySQLSample
	opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}})
	if err := r.samples.FindOne(ctx, bson.M{"checkId": checkID}, opts).Decode(&sample); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return monitoring.MySQLSample{}, fmt.Errorf("no samples found for check %s", checkID)
		}
		return monitoring.MySQLSample{}, fmt.Errorf("find latest mysql sample: %w", err)
	}
	return sample, nil
}

// RecentSamples returns newest samples first for a check.
func (r *MongoMySQLRepository) RecentSamples(checkID string, limit int) ([]monitoring.MySQLSample, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("mongo mysql repository is not configured")
	}
	if limit <= 0 {
		limit = 20
	}

	ctx, cancel := r.operationContext()
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit))
	cursor, err := r.samples.Find(ctx, bson.M{"checkId": checkID}, opts)
	if err != nil {
		return nil, fmt.Errorf("find recent mysql samples: %w", err)
	}
	defer cursor.Close(ctx)

	var samples []monitoring.MySQLSample
	if err := cursor.All(ctx, &samples); err != nil {
		return nil, fmt.Errorf("decode recent mysql samples: %w", err)
	}
	if samples == nil {
		return []monitoring.MySQLSample{}, nil
	}
	return samples, nil
}

// RecentDeltas returns newest deltas first for a check.
func (r *MongoMySQLRepository) RecentDeltas(checkID string, limit int) ([]monitoring.MySQLDelta, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("mongo mysql repository is not configured")
	}
	if limit <= 0 {
		limit = 20
	}

	ctx, cancel := r.operationContext()
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit))
	cursor, err := r.deltas.Find(ctx, bson.M{"checkId": checkID}, opts)
	if err != nil {
		return nil, fmt.Errorf("find recent mysql deltas: %w", err)
	}
	defer cursor.Close(ctx)

	var deltas []monitoring.MySQLDelta
	if err := cursor.All(ctx, &deltas); err != nil {
		return nil, fmt.Errorf("decode recent mysql deltas: %w", err)
	}
	if deltas == nil {
		return []monitoring.MySQLDelta{}, nil
	}
	return deltas, nil
}

// PruneBefore removes samples and deltas older than cutoff.
func (r *MongoMySQLRepository) PruneBefore(cutoff time.Time) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("mongo mysql repository is not configured")
	}

	ctx, cancel := r.operationContext()
	defer cancel()
	if _, err := r.samples.DeleteMany(ctx, bson.M{"timestamp": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune mysql samples: %w", err)
	}
	if _, err := r.deltas.DeleteMany(ctx, bson.M{"timestamp": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune mysql deltas: %w", err)
	}
	return nil
}

func (r *MongoMySQLRepository) ensureIndexes(ctx context.Context) error {
	if _, err := r.samples.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "checkId", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}},
		{Keys: bson.D{{Key: "sampleId", Value: 1}}},
	}); err != nil && !mongoMySQLIndexAlreadyExists(err) {
		return err
	}

	if _, err := r.deltas.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "checkId", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}},
		{Keys: bson.D{{Key: "sampleId", Value: 1}}},
	}); err != nil && !mongoMySQLIndexAlreadyExists(err) {
		return err
	}

	return nil
}

func (r *MongoMySQLRepository) operationContext() (context.Context, context.CancelFunc) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = mongoRepositoryDefaultTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

func mongoMySQLIndexAlreadyExists(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}
