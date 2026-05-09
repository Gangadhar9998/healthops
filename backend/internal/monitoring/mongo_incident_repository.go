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

type MongoIncidentRepository struct {
	collection *mongo.Collection
	timeout    time.Duration
}

var _ IncidentRepository = (*MongoIncidentRepository)(nil)

func NewMongoIncidentRepository(client *mongo.Client, dbName, prefix string) (*MongoIncidentRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo incident repository requires client")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	repo := &MongoIncidentRepository{
		collection: client.Database(dbName).Collection(prefix + "_incidents"),
		timeout:    5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), repo.timeout)
	defer cancel()
	if _, err := repo.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "checkId", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "updatedAt", Value: -1}}},
		{Keys: bson.D{{Key: "startedAt", Value: -1}}},
	}); err != nil && !indexAlreadyExists(err) {
		return nil, fmt.Errorf("create incident indexes: %w", err)
	}

	return repo, nil
}

func (r *MongoIncidentRepository) CreateIncident(incident Incident) error {
	if incident.ID == "" {
		incident.ID = fmt.Sprintf("incident-%d", time.Now().UnixNano())
	}
	incident = cloneIncident(incident)

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.InsertOne(ctx, incident); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("incident already exists: %s", incident.ID)
		}
		return fmt.Errorf("insert incident: %w", err)
	}
	return nil
}

func (r *MongoIncidentRepository) UpdateIncident(id string, mutator func(*Incident) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var incident Incident
	if err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&incident); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("incident not found: %s", id)
		}
		return fmt.Errorf("find incident: %w", err)
	}

	incident = cloneIncident(incident)
	if err := mutator(&incident); err != nil {
		return err
	}

	result, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id}, incident)
	if err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("incident not found: %s", id)
	}
	return nil
}

func (r *MongoIncidentRepository) GetIncident(id string) (Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var incident Incident
	if err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&incident); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Incident{}, nil
		}
		return Incident{}, fmt.Errorf("find incident: %w", err)
	}
	return cloneIncident(incident), nil
}

func (r *MongoIncidentRepository) ListIncidents() ([]Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "startedAt", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("find incidents: %w", err)
	}
	defer cur.Close(ctx)

	incidents := make([]Incident, 0)
	for cur.Next(ctx) {
		var incident Incident
		if err := cur.Decode(&incident); err != nil {
			return nil, fmt.Errorf("decode incident: %w", err)
		}
		incidents = append(incidents, cloneIncident(incident))
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return incidents, nil
}

func (r *MongoIncidentRepository) FindOpenIncident(checkID string) (Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	opts := options.FindOne().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
	filter := bson.M{"checkId": checkID, "status": bson.M{"$ne": "resolved"}}
	var incident Incident
	if err := r.collection.FindOne(ctx, filter, opts).Decode(&incident); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Incident{}, nil
		}
		return Incident{}, fmt.Errorf("find open incident: %w", err)
	}
	return cloneIncident(incident), nil
}

func (r *MongoIncidentRepository) PruneBefore(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	if _, err := r.collection.DeleteMany(ctx, bson.M{
		"status":    "resolved",
		"updatedAt": bson.M{"$lt": cutoff},
	}); err != nil {
		return fmt.Errorf("prune incidents: %w", err)
	}
	return nil
}

func cloneIncident(in Incident) Incident {
	out := in
	if in.ResolvedAt != nil {
		resolved := *in.ResolvedAt
		out.ResolvedAt = &resolved
	}
	if in.AcknowledgedAt != nil {
		ack := *in.AcknowledgedAt
		out.AcknowledgedAt = &ack
	}
	if in.Metadata != nil {
		out.Metadata = copyMap(in.Metadata)
	}
	return out
}
