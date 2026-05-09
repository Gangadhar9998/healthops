package monitoring

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoAuditRepository struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ AuditRepository = (*MongoAuditRepository)(nil)

func NewMongoAuditRepository(client *mongo.Client, dbName, prefix string) (*MongoAuditRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo audit repository requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	repo := &MongoAuditRepository{
		collection: client.Database(dbName).Collection(prefix + "_audit_events"),
		timeout:    5 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), repo.timeout)
	defer cancel()
	_, err := repo.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "action", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "actor", Value: 1}, {Key: "timestamp", Value: -1}}},
	})
	if err != nil && !indexAlreadyExists(err) {
		return nil, fmt.Errorf("create audit indexes: %w", err)
	}
	return repo, nil
}

func (r *MongoAuditRepository) InsertEvent(event AuditEvent) error {
	if event.ID == "" {
		event.ID = generateAuditID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, err := r.collection.InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (r *MongoAuditRepository) ListEvents(filter AuditFilter) ([]AuditEvent, error) {
	query := bson.M{}
	if filter.Action != "" {
		query["action"] = filter.Action
	}
	if filter.Actor != "" {
		query["actor"] = filter.Actor
	}
	if filter.Target != "" {
		query["target"] = filter.Target
	}
	if filter.TargetID != "" {
		query["targetId"] = filter.TargetID
	}
	if !filter.StartTime.IsZero() || !filter.EndTime.IsZero() {
		timeFilter := bson.M{}
		if !filter.StartTime.IsZero() {
			timeFilter["$gte"] = filter.StartTime
		}
		if !filter.EndTime.IsZero() {
			timeFilter["$lte"] = filter.EndTime
		}
		query["timestamp"] = timeFilter
	}

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cursor, err := r.collection.Find(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("find audit events: %w", err)
	}
	defer cursor.Close(ctx)

	var events []AuditEvent
	for cursor.Next(ctx) {
		var event AuditEvent
		if err := cursor.Decode(&event); err != nil {
			return nil, fmt.Errorf("decode audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	if events == nil {
		return []AuditEvent{}, nil
	}
	return events, nil
}
