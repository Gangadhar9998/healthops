package repositories

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoChannelRepository implements NotificationChannelRepository with MongoDB.
type MongoChannelRepository struct {
	client  *mongo.Client
	db      *mongo.Database
	coll    *mongo.Collection
	timeout time.Duration
}

// NewMongoChannelRepository creates a new MongoDB-backed channel repository.
func NewMongoChannelRepository(uri, dbName, collectionPrefix string, timeoutSeconds int) (*MongoChannelRepository, error) {
	if uri == "" {
		return nil, fmt.Errorf("mongo uri is required")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if collectionPrefix == "" {
		collectionPrefix = "healthops"
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 5
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping failed: %w", err)
	}

	repo := &MongoChannelRepository{
		client:  client,
		db:      client.Database(dbName),
		coll:    client.Database(dbName).Collection(collectionPrefix + "_notification_channels"),
		timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	if err := repo.ensureIndexes(ctx); err != nil {
		fmt.Printf("WARNING: MongoDB channel index creation deferred: %v\n", err)
	}

	return repo, nil
}

// ensureIndexes creates indexes for performance and querying.
func (r *MongoChannelRepository) ensureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "name", Value: 1}}},
		{Keys: bson.D{{Key: "enabled", Value: 1}}},
	})
	return err
}

// channelModel represents the MongoDB document structure.
type channelModel struct {
	ID                     string                 `bson:"_id"`
	Name                   string                 `bson:"name"`
	Type                   string                 `bson:"type"`
	Enabled                bool                   `bson:"enabled"`
	WebhookURL             string                 `bson:"webhookUrl,omitempty"`
	Email                  string                 `bson:"email,omitempty"`
	SMTPHost               string                 `bson:"smtpHost,omitempty"`
	SMTPPort               int                    `bson:"smtpPort,omitempty"`
	SMTPUser               string                 `bson:"smtpUser,omitempty"`
	SMTPPass               string                 `bson:"smtpPass,omitempty"`
	FromEmail              string                 `bson:"fromEmail,omitempty"`
	BotToken               string                 `bson:"botToken,omitempty"`
	ChatID                 string                 `bson:"chatId,omitempty"`
	RoutingKey             string                 `bson:"routingKey,omitempty"`
	Severities             []string               `bson:"severities,omitempty"`
	CheckIDs               []string               `bson:"checkIds,omitempty"`
	CheckTypes             []string               `bson:"checkTypes,omitempty"`
	Servers                []string               `bson:"servers,omitempty"`
	Tags                   []string               `bson:"tags,omitempty"`
	CooldownMinutes        int                    `bson:"cooldownMinutes,omitempty"`
	MinConsecutiveFailures int                    `bson:"minConsecutiveFailures,omitempty"`
	NotifyOnResolve        bool                   `bson:"notifyOnResolve,omitempty"`
	Headers                map[string]string      `bson:"headers,omitempty"`
	Config                 map[string]interface{} `bson:"config,omitempty"`
	SmartFilters           smartFiltersModel      `bson:"smartFilters,omitempty"`
	CreatedAt              time.Time              `bson:"createdAt"`
	UpdatedAt              time.Time              `bson:"updatedAt"`
}

// smartFiltersModel represents legacy smart filters in MongoDB.
type smartFiltersModel struct {
	Severities []string `bson:"severities,omitempty"`
	CheckIDs   []string `bson:"checkIds,omitempty"`
	CheckTypes []string `bson:"checkTypes,omitempty"`
	Servers    []string `bson:"servers,omitempty"`
	Tags       []string `bson:"tags,omitempty"`
}

// List retrieves all notification channels.
func (r *MongoChannelRepository) List(ctx context.Context) ([]NotificationChannel, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cur, err := r.coll.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("find channels: %w", err)
	}
	defer cur.Close(ctx)

	var models []channelModel
	if err := cur.All(ctx, &models); err != nil {
		return nil, fmt.Errorf("decode channels: %w", err)
	}

	return r.modelsToChannels(models), nil
}

// Get retrieves a channel by ID.
func (r *MongoChannelRepository) Get(ctx context.Context, id string) (*NotificationChannel, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var model channelModel
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&model)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("channel not found: %s", id)
		}
		return nil, fmt.Errorf("find channel: %w", err)
	}

	channel := r.modelToChannel(model)
	return &channel, nil
}

// Create inserts a new notification channel.
func (r *MongoChannelRepository) Create(ctx context.Context, channel *NotificationChannel) error {
	if err := r.validateChannelType(channel.Type); err != nil {
		return err
	}
	if channel.ID == "" {
		channel.ID = fmt.Sprintf("ch-%d", time.Now().UnixNano())
	}

	now := time.Now().UTC()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	model := r.channelToModel(channel)

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, model)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("channel with id %q already exists", channel.ID)
		}
		return fmt.Errorf("insert channel: %w", err)
	}

	return nil
}

// Update modifies an existing channel.
func (r *MongoChannelRepository) Update(ctx context.Context, id string, channel *NotificationChannel) error {
	if err := r.validateChannelType(channel.Type); err != nil {
		return err
	}

	channel.ID = id
	channel.UpdatedAt = time.Now().UTC()
	model := r.channelToModel(channel)

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": id}, model)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("channel not found: %s", id)
	}

	return nil
}

// Delete removes a channel.
func (r *MongoChannelRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	result, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("channel not found: %s", id)
	}

	return nil
}

// GetEnabled retrieves only active channels for alert dispatch.
func (r *MongoChannelRepository) GetEnabled(ctx context.Context) ([]NotificationChannel, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cur, err := r.coll.Find(ctx, bson.M{"enabled": true})
	if err != nil {
		return nil, fmt.Errorf("find enabled channels: %w", err)
	}
	defer cur.Close(ctx)

	var models []channelModel
	if err := cur.All(ctx, &models); err != nil {
		return nil, fmt.Errorf("decode enabled channels: %w", err)
	}

	return r.modelsToChannels(models), nil
}

// SeedIfEmpty populates default channels if none exist.
func (r *MongoChannelRepository) SeedIfEmpty(ctx context.Context, channels []NotificationChannel) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	count, err := r.coll.CountDocuments(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("count channels: %w", err)
	}
	if count > 0 || len(channels) == 0 {
		return nil
	}

	now := time.Now().UTC()
	var docs []interface{}
	for i := range channels {
		if channels[i].ID == "" {
			channels[i].ID = fmt.Sprintf("ch-%d", now.UnixNano()+int64(i))
		}
		channels[i].CreatedAt = now
		channels[i].UpdatedAt = now
		docs = append(docs, r.channelToModel(&channels[i]))
	}

	_, err = r.coll.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("seed channels: %w", err)
	}

	return nil
}

// validateChannelType checks if the channel type is supported.
func (r *MongoChannelRepository) validateChannelType(channelType string) error {
	validTypes := map[string]bool{
		"email":     true,
		"slack":     true,
		"discord":   true,
		"telegram":  true,
		"webhook":   true,
		"pagerduty": true,
	}
	if !validTypes[channelType] {
		return fmt.Errorf("unsupported channel type: %s", channelType)
	}
	return nil
}

// modelToChannel converts MongoDB model to repository domain model.
func (r *MongoChannelRepository) modelToChannel(model channelModel) NotificationChannel {
	channel := NotificationChannel{
		ID:                     model.ID,
		Name:                   model.Name,
		Type:                   model.Type,
		Enabled:                model.Enabled,
		WebhookURL:             firstNonEmpty(model.WebhookURL, getString(model.Config, "webhookUrl")),
		Email:                  firstNonEmpty(model.Email, getString(model.Config, "email")),
		SMTPHost:               firstNonEmpty(model.SMTPHost, getString(model.Config, "smtpHost")),
		SMTPPort:               firstNonZero(model.SMTPPort, getInt(model.Config, "smtpPort")),
		SMTPUser:               firstNonEmpty(model.SMTPUser, getString(model.Config, "smtpUser")),
		SMTPPass:               firstNonEmpty(model.SMTPPass, getString(model.Config, "smtpPass")),
		FromEmail:              firstNonEmpty(model.FromEmail, getString(model.Config, "fromEmail")),
		BotToken:               firstNonEmpty(model.BotToken, getString(model.Config, "botToken")),
		ChatID:                 firstNonEmpty(model.ChatID, getString(model.Config, "chatId")),
		RoutingKey:             firstNonEmpty(model.RoutingKey, getString(model.Config, "routingKey")),
		Severities:             firstNonEmptySlice(model.Severities, model.SmartFilters.Severities),
		CheckIDs:               firstNonEmptySlice(model.CheckIDs, model.SmartFilters.CheckIDs),
		CheckTypes:             firstNonEmptySlice(model.CheckTypes, model.SmartFilters.CheckTypes),
		Servers:                firstNonEmptySlice(model.Servers, model.SmartFilters.Servers),
		Tags:                   firstNonEmptySlice(model.Tags, model.SmartFilters.Tags),
		CooldownMinutes:        model.CooldownMinutes,
		MinConsecutiveFailures: model.MinConsecutiveFailures,
		NotifyOnResolve:        model.NotifyOnResolve,
		Headers:                cloneMap(model.Headers),
		CreatedAt:              model.CreatedAt,
		UpdatedAt:              model.UpdatedAt,
	}

	channel.CooldownMinutes = firstNonZero(channel.CooldownMinutes, getInt(model.Config, "cooldownMinutes"))
	channel.MinConsecutiveFailures = firstNonZero(channel.MinConsecutiveFailures, getInt(model.Config, "minConsecutiveFailures"))
	if !channel.NotifyOnResolve {
		channel.NotifyOnResolve = getBool(model.Config, "notifyOnResolve")
	}
	if len(channel.Headers) == 0 {
		channel.Headers = getStringMap(model.Config, "headers")
	}

	return channel
}

// modelsToChannels converts multiple MongoDB models to domain models.
func (r *MongoChannelRepository) modelsToChannels(models []channelModel) []NotificationChannel {
	channels := make([]NotificationChannel, len(models))
	for i, model := range models {
		channels[i] = r.modelToChannel(model)
	}
	return channels
}

// channelToModel converts repository domain model to MongoDB model.
func (r *MongoChannelRepository) channelToModel(channel *NotificationChannel) channelModel {
	config := map[string]interface{}{
		"webhookUrl":             channel.WebhookURL,
		"email":                  channel.Email,
		"smtpHost":               channel.SMTPHost,
		"smtpPort":               channel.SMTPPort,
		"smtpUser":               channel.SMTPUser,
		"smtpPass":               channel.SMTPPass,
		"fromEmail":              channel.FromEmail,
		"botToken":               channel.BotToken,
		"chatId":                 channel.ChatID,
		"routingKey":             channel.RoutingKey,
		"cooldownMinutes":        channel.CooldownMinutes,
		"minConsecutiveFailures": channel.MinConsecutiveFailures,
		"notifyOnResolve":        channel.NotifyOnResolve,
		"headers":                cloneMap(channel.Headers),
	}

	return channelModel{
		ID:                     channel.ID,
		Name:                   channel.Name,
		Type:                   channel.Type,
		Enabled:                channel.Enabled,
		WebhookURL:             channel.WebhookURL,
		Email:                  channel.Email,
		SMTPHost:               channel.SMTPHost,
		SMTPPort:               channel.SMTPPort,
		SMTPUser:               channel.SMTPUser,
		SMTPPass:               channel.SMTPPass,
		FromEmail:              channel.FromEmail,
		BotToken:               channel.BotToken,
		ChatID:                 channel.ChatID,
		RoutingKey:             channel.RoutingKey,
		Severities:             append([]string(nil), channel.Severities...),
		CheckIDs:               append([]string(nil), channel.CheckIDs...),
		CheckTypes:             append([]string(nil), channel.CheckTypes...),
		Servers:                append([]string(nil), channel.Servers...),
		Tags:                   append([]string(nil), channel.Tags...),
		CooldownMinutes:        channel.CooldownMinutes,
		MinConsecutiveFailures: channel.MinConsecutiveFailures,
		NotifyOnResolve:        channel.NotifyOnResolve,
		Headers:                cloneMap(channel.Headers),
		Config:                 config,
		SmartFilters: smartFiltersModel{
			Severities: append([]string(nil), channel.Severities...),
			CheckIDs:   append([]string(nil), channel.CheckIDs...),
			CheckTypes: append([]string(nil), channel.CheckTypes...),
			Servers:    append([]string(nil), channel.Servers...),
			Tags:       append([]string(nil), channel.Tags...),
		},
		CreatedAt: channel.CreatedAt,
		UpdatedAt: channel.UpdatedAt,
	}
}

// Close disconnects the MongoDB client.
func (r *MongoChannelRepository) Close() error {
	if r.client != nil {
		return r.client.Disconnect(context.Background())
	}
	return nil
}

// Ping checks if MongoDB is reachable.
func (r *MongoChannelRepository) Ping(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("mongo client is nil")
	}
	return r.client.Ping(ctx, nil)
}

func firstNonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func firstNonZero(primary, fallback int) int {
	if primary != 0 {
		return primary
	}
	return fallback
}

func firstNonEmptySlice(primary, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	if len(fallback) > 0 {
		return append([]string(nil), fallback...)
	}
	return nil
}

func getString(config map[string]interface{}, key string) string {
	if config == nil {
		return ""
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func getInt(config map[string]interface{}, key string) int {
	if config == nil {
		return 0
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func getBool(config map[string]interface{}, key string) bool {
	if config == nil {
		return false
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return value == "true"
	}
	return false
}

func getStringMap(config map[string]interface{}, key string) map[string]string {
	if config == nil {
		return nil
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case map[string]string:
		return cloneMap(value)
	case map[string]interface{}:
		out := make(map[string]string, len(value))
		for k, v := range value {
			out[k] = fmt.Sprint(v)
		}
		return out
	default:
		return nil
	}
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
