package notify

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

type MongoNotificationOutbox struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ NotificationOutboxRepository = (*MongoNotificationOutbox)(nil)

func NewMongoNotificationOutbox(client *mongo.Client, dbName, prefix string) (*MongoNotificationOutbox, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo notification outbox requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	outbox := &MongoNotificationOutbox{
		collection: client.Database(dbName).Collection(prefix + "_notification_outbox"),
		timeout:    5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), outbox.timeout)
	defer cancel()
	if _, err := outbox.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}},
		{Keys: bson.D{{Key: "createdAt", Value: 1}}},
	}); err != nil && !mongoNotifyIndexAlreadyExists(err) {
		return nil, fmt.Errorf("create notification outbox indexes: %w", err)
	}
	return outbox, nil
}

func (o *MongoNotificationOutbox) Enqueue(evt monitoring.NotificationEvent) error {
	if evt.NotificationID == "" {
		evt.NotificationID = fmt.Sprintf("notif-%s-%d", evt.IncidentID, time.Now().UnixNano())
	}
	if evt.Status == "" {
		evt.Status = "pending"
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}

	doc := bson.M{
		"_id":            evt.NotificationID,
		"notificationId": evt.NotificationID,
		"incidentId":     evt.IncidentID,
		"channel":        evt.Channel,
		"payloadJson":    evt.PayloadJSON,
		"status":         evt.Status,
		"retryCount":     evt.RetryCount,
		"lastError":      evt.LastError,
		"createdAt":      evt.CreatedAt,
		"sentAt":         evt.SentAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	if _, err := o.collection.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func (o *MongoNotificationOutbox) ListPending(limit int) ([]monitoring.NotificationEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(int64(limit))
	cur, err := o.collection.Find(ctx, bson.M{"status": "pending"}, opts)
	if err != nil {
		return nil, fmt.Errorf("find pending notifications: %w", err)
	}
	defer cur.Close(ctx)
	return decodeNotificationEvents(ctx, cur)
}

func (o *MongoNotificationOutbox) MarkSent(id string) error {
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	res, err := o.collection.UpdateOne(ctx, bson.M{"_id": id, "status": "pending"}, bson.M{
		"$set": bson.M{"status": "sent", "sentAt": now},
	})
	if err != nil {
		return fmt.Errorf("mark notification sent: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("notification not found or not pending: %s", id)
	}
	return nil
}

func (o *MongoNotificationOutbox) MarkFailed(id string, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	res, err := o.collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"status": "failed", "lastError": reason},
		"$inc": bson.M{"retryCount": 1},
	})
	if err != nil {
		return fmt.Errorf("mark notification failed: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("notification not found: %s", id)
	}
	return nil
}

func (o *MongoNotificationOutbox) PruneBefore(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	if _, err := o.collection.DeleteMany(ctx, bson.M{"createdAt": bson.M{"$lt": cutoff}}); err != nil {
		return fmt.Errorf("prune notifications: %w", err)
	}
	return nil
}

func (o *MongoNotificationOutbox) AllNotifications() []monitoring.NotificationEvent {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	cur, err := o.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	events, err := decodeNotificationEvents(ctx, cur)
	if err != nil {
		return nil
	}
	return events
}

func decodeNotificationEvents(ctx context.Context, cur *mongo.Cursor) ([]monitoring.NotificationEvent, error) {
	events := make([]monitoring.NotificationEvent, 0)
	for cur.Next(ctx) {
		var event monitoring.NotificationEvent
		if err := cur.Decode(&event); err != nil {
			return nil, fmt.Errorf("decode notification: %w", err)
		}
		events = append(events, event)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return events, nil
}

func mongoNotifyIndexAlreadyExists(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}
