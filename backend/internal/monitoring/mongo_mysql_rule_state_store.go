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

type MongoMySQLRuleStateStore struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ MySQLRuleStateStore = (*MongoMySQLRuleStateStore)(nil)

func NewMongoMySQLRuleStateStore(client *mongo.Client, dbName, prefix string) (*MongoMySQLRuleStateStore, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo client is required")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	store := &MongoMySQLRuleStateStore{
		collection: client.Database(dbName).Collection(prefix + "_mysql_rule_states"),
		timeout:    5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), store.timeout)
	defer cancel()
	if _, err := store.collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "updatedAt", Value: -1}}}); err != nil && !indexAlreadyExists(err) {
		return nil, fmt.Errorf("create mysql rule state indexes: %w", err)
	}
	return store, nil
}

func (s *MongoMySQLRuleStateStore) LoadStates() (map[string]*AlertState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	cur, err := s.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("find mysql rule states: %w", err)
	}
	defer cur.Close(ctx)

	states := make(map[string]*AlertState)
	for cur.Next(ctx) {
		var doc struct {
			ID    string     `bson:"_id"`
			State AlertState `bson:"state"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode mysql rule state: %w", err)
		}
		state := doc.State
		states[doc.ID] = &state
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate mysql rule states: %w", err)
	}
	return states, nil
}

func (s *MongoMySQLRuleStateStore) SaveStates(states map[string]*AlertState) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	keys := make([]string, 0, len(states))
	now := time.Now().UTC()
	for key, state := range states {
		if state == nil {
			continue
		}
		keys = append(keys, key)
		doc := bson.M{
			"_id":       key,
			"state":     *state,
			"updatedAt": now,
		}
		if _, err := s.collection.ReplaceOne(ctx, bson.M{"_id": key}, doc, options.Replace().SetUpsert(true)); err != nil {
			return fmt.Errorf("save mysql rule state %q: %w", key, err)
		}
	}

	filter := bson.M{}
	if len(keys) > 0 {
		filter = bson.M{"_id": bson.M{"$nin": keys}}
	}
	if _, err := s.collection.DeleteMany(ctx, filter); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("delete stale mysql rule states: %w", err)
	}
	return nil
}
