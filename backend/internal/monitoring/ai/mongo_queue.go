package ai

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

type MongoAIQueue struct {
	queue   *mongo.Collection
	results *mongo.Collection
	timeout time.Duration
}

var _ AIQueueRepository = (*MongoAIQueue)(nil)

func NewMongoAIQueue(client *mongo.Client, dbName, prefix string) (*MongoAIQueue, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo ai queue requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	q := &MongoAIQueue{
		queue:   client.Database(dbName).Collection(prefix + "_ai_queue"),
		results: client.Database(dbName).Collection(prefix + "_ai_results"),
		timeout: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	if err := q.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("create ai queue indexes: %w", err)
	}
	return q, nil
}

func (q *MongoAIQueue) Enqueue(incidentID string, promptVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	filter := bson.M{
		"incidentId": incidentID,
		"status":     bson.M{"$in": []string{"pending", "processing"}},
	}
	count, err := q.queue.CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("check ai queue dedupe: %w", err)
	}
	if count > 0 {
		return nil
	}

	item := monitoring.AIQueueItem{
		IncidentID:    incidentID,
		PromptVersion: promptVersion,
		Status:        "pending",
		CreatedAt:     time.Now().UTC(),
	}
	if _, err := q.queue.InsertOne(ctx, item); err != nil {
		return fmt.Errorf("insert ai queue item: %w", err)
	}
	return nil
}

func (q *MongoAIQueue) ClaimPending(limit int) ([]monitoring.AIQueueItem, error) {
	if limit <= 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(int64(limit))
	cur, err := q.queue.Find(ctx, bson.M{"status": "pending"}, opts)
	if err != nil {
		return nil, fmt.Errorf("find pending ai queue items: %w", err)
	}
	items, err := decodeAIQueueItems(ctx, cur)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	for i := range items {
		res, err := q.queue.UpdateOne(ctx, bson.M{
			"incidentId": items[i].IncidentID,
			"status":     "pending",
		}, bson.M{"$set": bson.M{"status": "processing", "claimedAt": now}})
		if err != nil {
			return nil, fmt.Errorf("claim ai queue item: %w", err)
		}
		if res.MatchedCount > 0 {
			items[i].Status = "processing"
			items[i].ClaimedAt = &now
		}
	}
	return items, nil
}

func (q *MongoAIQueue) Complete(incidentID string, result monitoring.AIAnalysisResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	now := time.Now().UTC()
	res, err := q.queue.UpdateOne(ctx, bson.M{
		"incidentId": incidentID,
		"status":     bson.M{"$in": []string{"pending", "processing"}},
	}, bson.M{"$set": bson.M{"status": "completed", "completedAt": now}})
	if err != nil {
		return fmt.Errorf("complete ai queue item: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
	}

	result.IncidentID = incidentID
	if result.CreatedAt.IsZero() {
		result.CreatedAt = now
	}
	if _, err := q.results.InsertOne(ctx, result); err != nil {
		return fmt.Errorf("insert ai result: %w", err)
	}
	return nil
}

func (q *MongoAIQueue) Fail(incidentID string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	res, err := q.queue.UpdateOne(ctx, bson.M{
		"incidentId": incidentID,
		"status":     bson.M{"$in": []string{"pending", "processing"}},
	}, bson.M{"$set": bson.M{"status": "failed", "lastError": reason}})
	if err != nil {
		return fmt.Errorf("fail ai queue item: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
	}
	return nil
}

func (q *MongoAIQueue) PruneBefore(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	if _, err := q.queue.DeleteMany(ctx, bson.M{"createdAt": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune ai queue: %w", err)
	}
	if _, err := q.results.DeleteMany(ctx, bson.M{"createdAt": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune ai results: %w", err)
	}
	return nil
}

func (q *MongoAIQueue) GetResults(incidentID string) []monitoring.AIAnalysisResult {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	cur, err := q.results.Find(ctx, bson.M{"incidentId": incidentID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	results, err := decodeAIResults(ctx, cur)
	if err != nil {
		return nil
	}
	return results
}

func (q *MongoAIQueue) AllResults(limit int) []monitoring.AIAnalysisResult {
	if limit <= 0 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	cur, err := q.results.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	results, err := decodeAIResults(ctx, cur)
	if err != nil {
		return nil
	}
	return results
}

func (q *MongoAIQueue) ListPendingItems(limit int) ([]monitoring.AIQueueItem, error) {
	if limit <= 0 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	cur, err := q.queue.Find(ctx, bson.M{"status": "pending"}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("find pending ai queue items: %w", err)
	}
	return decodeAIQueueItems(ctx, cur)
}

func (q *MongoAIQueue) AllItems() []monitoring.AIQueueItem {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()
	cur, err := q.queue.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil
	}
	items, err := decodeAIQueueItems(ctx, cur)
	if err != nil {
		return nil
	}
	return items
}

func (q *MongoAIQueue) ensureIndexes(ctx context.Context) error {
	if _, err := q.queue.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "incidentId", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}},
		{Keys: bson.D{{Key: "createdAt", Value: 1}}},
	}); err != nil && !mongoAIIndexAlreadyExists(err) {
		return err
	}
	if _, err := q.results.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "incidentId", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "createdAt", Value: 1}}},
	}); err != nil && !mongoAIIndexAlreadyExists(err) {
		return err
	}
	return nil
}

func decodeAIQueueItems(ctx context.Context, cur *mongo.Cursor) ([]monitoring.AIQueueItem, error) {
	defer cur.Close(ctx)
	items := make([]monitoring.AIQueueItem, 0)
	for cur.Next(ctx) {
		var item monitoring.AIQueueItem
		if err := cur.Decode(&item); err != nil {
			return nil, fmt.Errorf("decode ai queue item: %w", err)
		}
		items = append(items, item)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate ai queue items: %w", err)
	}
	return items, nil
}

func decodeAIResults(ctx context.Context, cur *mongo.Cursor) ([]monitoring.AIAnalysisResult, error) {
	results := make([]monitoring.AIAnalysisResult, 0)
	for cur.Next(ctx) {
		var result monitoring.AIAnalysisResult
		if err := cur.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode ai result: %w", err)
		}
		results = append(results, result)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate ai results: %w", err)
	}
	return results, nil
}

func mongoAIIndexAlreadyExists(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}
