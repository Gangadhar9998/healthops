package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"medics-health-check/backend/internal/monitoring"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrAlertRuleRepositoryNotConfigured is returned when the repository is not properly initialized.
	ErrAlertRuleRepositoryNotConfigured = errors.New("alert rule repository is not configured")
	// ErrAlertRuleRepoOffline is returned when MongoDB is unavailable.
	ErrAlertRuleRepoOffline = errors.New("alert rule repository is unavailable")
	// ErrAlertRuleNotFound is returned when a rule is not found.
	ErrAlertRuleNotFound = errors.New("alert rule not found")
	// ErrAlertRuleExists is returned when attempting to create a duplicate rule.
	ErrAlertRuleExists = errors.New("alert rule already exists")
	// ErrInvalidSeverity is returned for invalid severity values.
	ErrInvalidSeverity = errors.New("invalid severity (must be: critical, warning, info)")
)

// validSeverities defines the allowed severity levels.
var validSeverities = map[string]bool{
	"critical": true,
	"warning":  true,
	"info":     true,
}

// AlertRuleRepositoryError carries repository context while preserving typed error checks.
type AlertRuleRepositoryError struct {
	Op    string
	ID    string
	Err   error
	Cause error
}

func (e *AlertRuleRepositoryError) Error() string {
	if e == nil {
		return "<nil>"
	}

	switch {
	case e.ID != "" && e.Cause != nil:
		return fmt.Sprintf("alert rule repository %s %q: %v", e.Op, e.ID, e.Cause)
	case e.ID != "":
		return fmt.Sprintf("alert rule repository %s %q: %v", e.Op, e.ID, e.Err)
	case e.Cause != nil:
		return fmt.Sprintf("alert rule repository %s: %v", e.Op, e.Cause)
	default:
		return fmt.Sprintf("alert rule repository %s: %v", e.Op, e.Err)
	}
}

func (e *AlertRuleRepositoryError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Err != nil && e.Cause != nil {
		return errors.Join(e.Err, e.Cause)
	}
	if e.Err != nil {
		return e.Err
	}
	return e.Cause
}

// MongoAlertRuleRepository implements AlertRuleRepository using MongoDB.
type MongoAlertRuleRepository struct {
	collection alertRuleCollection
}

// NewMongoAlertRuleRepository creates a new MongoDB-backed alert rule repository.
func NewMongoAlertRuleRepository(uri, dbName, prefix string) (*MongoAlertRuleRepository, error) {
	if uri == "" {
		return nil, &AlertRuleRepositoryError{Op: "new", Err: ErrAlertRuleRepositoryNotConfigured}
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	// Force IPv4: replace localhost with 127.0.0.1 to avoid IPv6 socket issues on macOS
	uri = strings.ReplaceAll(uri, "localhost", "127.0.0.1")

	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetMaxPoolSize(25)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, &AlertRuleRepositoryError{Op: "new", Cause: err}
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, &AlertRuleRepositoryError{Op: "new", Cause: err}
	}

	collection := client.Database(dbName).Collection(prefix + "_alert_rules")
	repo := &MongoAlertRuleRepository{
		collection: mongoAlertRuleCollectionAdapter{collection: collection},
	}

	// Create indexes in background (don't fail if they take too long)
	indexCtx, indexCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer indexCancel()
	if err := repo.ensureIndexes(indexCtx); err != nil {
		// Log but don't fail - indexes will be created in background
		fmt.Printf("WARNING: Alert rule repository index creation deferred: %v\n", err)
	}

	return repo, nil
}

// ensureIndexes creates indexes for better query performance.
func (r *MongoAlertRuleRepository) ensureIndexes(ctx context.Context) error {
	adapter, ok := r.collection.(mongoAlertRuleCollectionAdapter)
	if !ok {
		return nil // adapter doesn't support direct index creation
	}

	_, err := adapter.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "name", Value: 1}}},
		{Keys: bson.D{{Key: "enabled", Value: 1}}},
	})
	if err != nil && !indexAlreadyExistsForAlertRules(err) {
		return err
	}

	return nil
}

func indexAlreadyExistsForAlertRules(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}

// List retrieves all alert rules.
func (r *MongoAlertRuleRepository) List(ctx context.Context) ([]monitoring.AlertRule, error) {
	if r == nil || r.collection == nil {
		return nil, &AlertRuleRepositoryError{Op: "list", Err: ErrAlertRuleRepositoryNotConfigured}
	}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, &AlertRuleRepositoryError{Op: "list", Err: ErrAlertRuleRepoOffline, Cause: err}
	}
	defer cur.Close(ctx)

	rules := make([]monitoring.AlertRule, 0)
	for cur.Next(ctx) {
		var doc mongoAlertRuleDocument
		if err := cur.Decode(&doc); err != nil {
			return nil, &AlertRuleRepositoryError{Op: "list", Err: ErrAlertRuleRepoOffline, Cause: err}
		}
		rule := doc.toAlertRule()
		rules = append(rules, rule)
	}
	if err := cur.Err(); err != nil {
		return nil, &AlertRuleRepositoryError{Op: "list", Err: ErrAlertRuleRepoOffline, Cause: err}
	}

	return rules, nil
}

// Get retrieves a rule by ID.
func (r *MongoAlertRuleRepository) Get(ctx context.Context, id string) (*monitoring.AlertRule, error) {
	if r == nil || r.collection == nil {
		return nil, &AlertRuleRepositoryError{Op: "get", ID: id, Err: ErrAlertRuleRepositoryNotConfigured}
	}
	if id == "" {
		return nil, &AlertRuleRepositoryError{Op: "get", Err: fmt.Errorf("id is required")}
	}

	var doc mongoAlertRuleDocument
	if err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, &AlertRuleRepositoryError{Op: "get", ID: id, Err: ErrAlertRuleNotFound}
		}
		return nil, &AlertRuleRepositoryError{Op: "get", ID: id, Err: ErrAlertRuleRepoOffline, Cause: err}
	}

	rule := doc.toAlertRule()
	return &rule, nil
}

// Create inserts a new alert rule.
func (r *MongoAlertRuleRepository) Create(ctx context.Context, rule *monitoring.AlertRule) error {
	if r == nil || r.collection == nil {
		return &AlertRuleRepositoryError{Op: "create", ID: rule.ID, Err: ErrAlertRuleRepositoryNotConfigured}
	}

	if err := validateAlertRule(rule); err != nil {
		return &AlertRuleRepositoryError{Op: "create", ID: rule.ID, Cause: err}
	}

	// Apply defaults
	if rule.Severity == "" {
		rule.Severity = "warning"
	}

	doc := mongoAlertRuleDocumentFromAlertRule(rule)
	if _, err := r.collection.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return &AlertRuleRepositoryError{Op: "create", ID: rule.ID, Err: ErrAlertRuleExists, Cause: err}
		}
		return &AlertRuleRepositoryError{Op: "create", ID: rule.ID, Err: ErrAlertRuleRepoOffline, Cause: err}
	}

	return nil
}

// Update modifies an existing rule.
func (r *MongoAlertRuleRepository) Update(ctx context.Context, id string, rule *monitoring.AlertRule) error {
	if r == nil || r.collection == nil {
		return &AlertRuleRepositoryError{Op: "update", ID: id, Err: ErrAlertRuleRepositoryNotConfigured}
	}
	if id == "" {
		return &AlertRuleRepositoryError{Op: "update", Err: fmt.Errorf("id is required")}
	}

	if err := validateAlertRule(rule); err != nil {
		return &AlertRuleRepositoryError{Op: "update", ID: id, Cause: err}
	}

	// Ensure ID matches
	rule.ID = id

	doc := mongoAlertRuleDocumentFromAlertRule(rule)
	result, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id}, doc)
	if err != nil {
		return &AlertRuleRepositoryError{Op: "update", ID: id, Err: ErrAlertRuleRepoOffline, Cause: err}
	}
	if result.MatchedCount == 0 {
		return &AlertRuleRepositoryError{Op: "update", ID: id, Err: ErrAlertRuleNotFound}
	}

	return nil
}

// Delete removes a rule.
func (r *MongoAlertRuleRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.collection == nil {
		return &AlertRuleRepositoryError{Op: "delete", ID: id, Err: ErrAlertRuleRepositoryNotConfigured}
	}
	if id == "" {
		return &AlertRuleRepositoryError{Op: "delete", Err: fmt.Errorf("id is required")}
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return &AlertRuleRepositoryError{Op: "delete", ID: id, Err: ErrAlertRuleRepoOffline, Cause: err}
	}
	if result.DeletedCount == 0 {
		return &AlertRuleRepositoryError{Op: "delete", ID: id, Err: ErrAlertRuleNotFound}
	}

	return nil
}

// GetEnabled retrieves only active rules for evaluation.
func (r *MongoAlertRuleRepository) GetEnabled(ctx context.Context) ([]monitoring.AlertRule, error) {
	if r == nil || r.collection == nil {
		return nil, &AlertRuleRepositoryError{Op: "get_enabled", Err: ErrAlertRuleRepositoryNotConfigured}
	}

	filter := bson.M{"enabled": true}
	cur, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, &AlertRuleRepositoryError{Op: "get_enabled", Err: ErrAlertRuleRepoOffline, Cause: err}
	}
	defer cur.Close(ctx)

	rules := make([]monitoring.AlertRule, 0)
	for cur.Next(ctx) {
		var doc mongoAlertRuleDocument
		if err := cur.Decode(&doc); err != nil {
			return nil, &AlertRuleRepositoryError{Op: "get_enabled", Err: ErrAlertRuleRepoOffline, Cause: err}
		}
		rule := doc.toAlertRule()
		rules = append(rules, rule)
	}
	if err := cur.Err(); err != nil {
		return nil, &AlertRuleRepositoryError{Op: "get_enabled", Err: ErrAlertRuleRepoOffline, Cause: err}
	}

	return rules, nil
}

// validateAlertRule checks if an alert rule is valid.
func validateAlertRule(rule *monitoring.AlertRule) error {
	if rule.ID == "" {
		return fmt.Errorf("id is required")
	}
	if rule.Name == "" {
		return fmt.Errorf("name is required")
	}
	if rule.Severity != "" && !validSeverities[rule.Severity] {
		return ErrInvalidSeverity
	}
	return nil
}

// alertRuleCollection defines the collection interface for dependency injection.
type alertRuleCollection interface {
	CountDocuments(context.Context, any, ...options.Lister[options.CountOptions]) (int64, error)
	DeleteOne(context.Context, any, ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error)
	Find(context.Context, any, ...options.Lister[options.FindOptions]) (alertRuleCursor, error)
	FindOne(context.Context, any, ...options.Lister[options.FindOneOptions]) alertRuleSingleResult
	InsertOne(context.Context, any, ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error)
	ReplaceOne(context.Context, any, any, ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error)
}

// alertRuleCursor defines the cursor interface.
type alertRuleCursor interface {
	Close(context.Context) error
	Decode(any) error
	Err() error
	Next(context.Context) bool
}

// alertRuleSingleResult defines the single result interface.
type alertRuleSingleResult interface {
	Decode(any) error
	Err() error
}

// mongoAlertRuleCollectionAdapter adapts mongo.Collection to alertRuleCollection.
type mongoAlertRuleCollectionAdapter struct {
	collection *mongo.Collection
}

func (a mongoAlertRuleCollectionAdapter) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	return a.collection.CountDocuments(ctx, filter, opts...)
}

func (a mongoAlertRuleCollectionAdapter) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	return a.collection.DeleteOne(ctx, filter, opts...)
}

func (a mongoAlertRuleCollectionAdapter) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (alertRuleCursor, error) {
	cur, err := a.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	return mongoAlertRuleCursorAdapter{cursor: cur}, nil
}

func (a mongoAlertRuleCollectionAdapter) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) alertRuleSingleResult {
	return mongoAlertRuleSingleResultAdapter{single: a.collection.FindOne(ctx, filter, opts...)}
}

func (a mongoAlertRuleCollectionAdapter) InsertOne(ctx context.Context, document any, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error) {
	return a.collection.InsertOne(ctx, document, opts...)
}

func (a mongoAlertRuleCollectionAdapter) ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error) {
	return a.collection.ReplaceOne(ctx, filter, replacement, opts...)
}

// mongoAlertRuleCursorAdapter adapts mongo.Cursor to alertRuleCursor.
type mongoAlertRuleCursorAdapter struct {
	cursor *mongo.Cursor
}

func (a mongoAlertRuleCursorAdapter) Close(ctx context.Context) error {
	return a.cursor.Close(ctx)
}

func (a mongoAlertRuleCursorAdapter) Decode(v any) error {
	return a.cursor.Decode(v)
}

func (a mongoAlertRuleCursorAdapter) Err() error {
	return a.cursor.Err()
}

func (a mongoAlertRuleCursorAdapter) Next(ctx context.Context) bool {
	return a.cursor.Next(ctx)
}

// mongoAlertRuleSingleResultAdapter adapts mongo.SingleResult to alertRuleSingleResult.
type mongoAlertRuleSingleResultAdapter struct {
	single *mongo.SingleResult
}

func (a mongoAlertRuleSingleResultAdapter) Decode(v any) error {
	return a.single.Decode(v)
}

func (a mongoAlertRuleSingleResultAdapter) Err() error {
	return a.single.Err()
}

// mongoAlertRuleDocument represents the MongoDB document structure.
type mongoAlertRuleDocument struct {
	ID                  string                            `bson:"_id"`
	Name                string                            `bson:"name"`
	Description         string                            `bson:"description,omitempty"`
	Enabled             bool                              `bson:"enabled"`
	CheckIDs            []string                          `bson:"checkIds,omitempty"`
	Conditions          []mongoAlertConditionDocument     `bson:"conditions,omitempty"`
	Severity            string                            `bson:"severity"`
	Channels            []mongoAlertChannelDocument       `bson:"channels,omitempty"`
	CooldownMinutes     int                               `bson:"cooldownMinutes"`
	ConsecutiveBreaches int                               `bson:"consecutiveBreaches,omitempty"`
	RecoverySamples     int                               `bson:"recoverySamples,omitempty"`
	ThresholdNum        float64                           `bson:"thresholdNum,omitempty"`
	RuleCode            string                            `bson:"ruleCode,omitempty"`
}

// mongoAlertConditionDocument represents an alert condition in MongoDB.
type mongoAlertConditionDocument struct {
	Field    string      `bson:"field"`
	Operator string      `bson:"operator"`
	Value    interface{} `bson:"value"`
}

// mongoAlertChannelDocument represents an alert channel in MongoDB.
type mongoAlertChannelDocument struct {
	Type   string                 `bson:"type"`
	Config map[string]interface{} `bson:"config"`
}

// mongoAlertRuleDocumentFromAlertRule converts a monitoring.AlertRule to its MongoDB document representation.
func mongoAlertRuleDocumentFromAlertRule(rule *monitoring.AlertRule) mongoAlertRuleDocument {
	doc := mongoAlertRuleDocument{
		ID:                  rule.ID,
		Name:                rule.Name,
		Description:         rule.Description,
		Enabled:             rule.Enabled,
		CheckIDs:            cloneStringSlice(rule.CheckIDs),
		Severity:            rule.Severity,
		Channels:            make([]mongoAlertChannelDocument, len(rule.Channels)),
		CooldownMinutes:     rule.CooldownMinutes,
		ConsecutiveBreaches: rule.ConsecutiveBreaches,
		RecoverySamples:     rule.RecoverySamples,
		ThresholdNum:        rule.ThresholdNum,
		RuleCode:            rule.RuleCode,
	}

	// Convert conditions
	if len(rule.Conditions) > 0 {
		doc.Conditions = make([]mongoAlertConditionDocument, len(rule.Conditions))
		for i, cond := range rule.Conditions {
			doc.Conditions[i] = mongoAlertConditionDocument{
				Field:    cond.Field,
				Operator: string(cond.Operator),
				Value:    cond.Value,
			}
		}
	}

	// Convert channels
	for i, ch := range rule.Channels {
		doc.Channels[i] = mongoAlertChannelDocument{
			Type:   ch.Type,
			Config: ch.Config,
		}
	}

	return doc
}

// toAlertRule converts a MongoDB document to a monitoring.AlertRule.
func (d mongoAlertRuleDocument) toAlertRule() monitoring.AlertRule {
	rule := monitoring.AlertRule{
		ID:                  d.ID,
		Name:                d.Name,
		Description:         d.Description,
		Enabled:             d.Enabled,
		CheckIDs:            cloneStringSlice(d.CheckIDs),
		Severity:            d.Severity,
		Channels:            make([]monitoring.AlertChannel, len(d.Channels)),
		CooldownMinutes:     d.CooldownMinutes,
		ConsecutiveBreaches: d.ConsecutiveBreaches,
		RecoverySamples:     d.RecoverySamples,
		ThresholdNum:        d.ThresholdNum,
		RuleCode:            d.RuleCode,
	}

	// Convert conditions
	if len(d.Conditions) > 0 {
		rule.Conditions = make([]monitoring.AlertCondition, len(d.Conditions))
		for i, cond := range d.Conditions {
			rule.Conditions[i] = monitoring.AlertCondition{
				Field:    cond.Field,
				Operator: monitoring.AlertOperator(cond.Operator),
				Value:    cond.Value,
			}
		}
	}

	// Convert channels
	for i, ch := range d.Channels {
		rule.Channels[i] = monitoring.AlertChannel{
			Type:   ch.Type,
			Config: ch.Config,
		}
	}

	return rule
}

// cloneStringSlice creates a deep copy of a string slice.
func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
