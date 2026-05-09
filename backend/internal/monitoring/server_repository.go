package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrServerRepositoryNotConfigured = errors.New("server repository is not configured")
	ErrServerRepoOffline             = errors.New("server repository is unavailable")
	ErrServerNotFound                = errors.New("server not found")
	ErrServerExists                  = errors.New("server already exists")
	ErrServerAlreadyExists           = ErrServerExists
)

// ServerRepository persists RemoteServer definitions in an authoritative store.
type ServerRepository interface {
	List(context.Context) ([]RemoteServer, error)
	Get(context.Context, string) (RemoteServer, error)
	Create(context.Context, RemoteServer) (RemoteServer, error)
	Update(context.Context, RemoteServer) (RemoteServer, error)
	Delete(context.Context, string) error
	SeedIfEmpty(context.Context, []RemoteServer) error
}

// ServerRepositoryError carries repository context while preserving typed error checks.
type ServerRepositoryError struct {
	Op    string
	ID    string
	Err   error
	Cause error
}

func (e *ServerRepositoryError) Error() string {
	if e == nil {
		return "<nil>"
	}

	switch {
	case e.ID != "" && e.Cause != nil:
		return fmt.Sprintf("server repository %s %q: %v", e.Op, e.ID, e.Cause)
	case e.ID != "":
		return fmt.Sprintf("server repository %s %q: %v", e.Op, e.ID, e.Err)
	case e.Cause != nil:
		return fmt.Sprintf("server repository %s: %v", e.Op, e.Cause)
	default:
		return fmt.Sprintf("server repository %s: %v", e.Op, e.Err)
	}
}

func (e *ServerRepositoryError) Unwrap() error {
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

type MongoServerRepository struct {
	collection serverCollection
}

func NewMongoServerRepository(uri, dbName, prefix string) (*MongoServerRepository, error) {
	if uri == "" {
		return nil, &ServerRepositoryError{Op: "new", Err: ErrServerRepositoryNotConfigured}
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	uri = strings.ReplaceAll(uri, "localhost", "127.0.0.1")
	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetMaxPoolSize(25)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, &ServerRepositoryError{Op: "new", Cause: err}
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, &ServerRepositoryError{Op: "new", Cause: err}
	}

	return NewMongoServerRepositoryFromClient(client, dbName, prefix)
}

func NewMongoServerRepositoryFromClient(client *mongo.Client, dbName, prefix string) (*MongoServerRepository, error) {
	if client == nil {
		return nil, &ServerRepositoryError{Op: "new", Err: ErrServerRepositoryNotConfigured}
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}
	collection := client.Database(dbName).Collection(prefix + "_servers")

	indexCtx, indexCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer indexCancel()

	if err := ensureServerIndexes(indexCtx, collection); err != nil {
		return nil, &ServerRepositoryError{Op: "index", Cause: err}
	}

	return newMongoServerRepositoryWithCollection(mongoCollectionAdapter{collection: collection}), nil
}

func ensureServerIndexes(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "name", Value: 1}}},
	})
	if err != nil && !indexAlreadyExists(err) {
		return err
	}
	return nil
}

func newMongoServerRepositoryWithCollection(collection serverCollection) *MongoServerRepository {
	return &MongoServerRepository{collection: collection}
}

func (r *MongoServerRepository) List(ctx context.Context) ([]RemoteServer, error) {
	if r == nil || r.collection == nil {
		return nil, &ServerRepositoryError{Op: "list", Err: ErrServerRepositoryNotConfigured}
	}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, &ServerRepositoryError{Op: "list", Err: ErrServerRepoOffline, Cause: err}
	}
	defer cur.Close(ctx)

	servers := make([]RemoteServer, 0)
	for cur.Next(ctx) {
		var doc mongoServerDocument
		if err := cur.Decode(&doc); err != nil {
			return nil, &ServerRepositoryError{Op: "list", Err: ErrServerRepoOffline, Cause: err}
		}
		servers = append(servers, doc.toRemoteServer())
	}
	if err := cur.Err(); err != nil {
		return nil, &ServerRepositoryError{Op: "list", Err: ErrServerRepoOffline, Cause: err}
	}

	return servers, nil
}

func (r *MongoServerRepository) Get(ctx context.Context, id string) (RemoteServer, error) {
	if r == nil || r.collection == nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "get", ID: id, Err: ErrServerRepositoryNotConfigured}
	}
	if id == "" {
		return RemoteServer{}, &ServerRepositoryError{Op: "get", Err: fmt.Errorf("id is required")}
	}

	var doc mongoServerDocument
	if err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return RemoteServer{}, &ServerRepositoryError{Op: "get", ID: id, Err: ErrServerNotFound}
		}
		return RemoteServer{}, &ServerRepositoryError{Op: "get", ID: id, Err: ErrServerRepoOffline, Cause: err}
	}

	return doc.toRemoteServer(), nil
}

func (r *MongoServerRepository) Create(ctx context.Context, server RemoteServer) (RemoteServer, error) {
	if r == nil || r.collection == nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "create", ID: server.ID, Err: ErrServerRepositoryNotConfigured}
	}

	server, err := normalizeRemoteServer(server)
	if err != nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "create", ID: server.ID, Cause: err}
	}

	if _, err := r.collection.InsertOne(ctx, mongoServerDocumentFromRemoteServer(server)); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return RemoteServer{}, &ServerRepositoryError{Op: "create", ID: server.ID, Err: ErrServerExists, Cause: err}
		}
		return RemoteServer{}, &ServerRepositoryError{Op: "create", ID: server.ID, Err: ErrServerRepoOffline, Cause: err}
	}

	return cloneRemoteServer(server), nil
}

func (r *MongoServerRepository) Update(ctx context.Context, server RemoteServer) (RemoteServer, error) {
	if r == nil || r.collection == nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "update", ID: server.ID, Err: ErrServerRepositoryNotConfigured}
	}

	server, err := normalizeRemoteServer(server)
	if err != nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "update", ID: server.ID, Cause: err}
	}

	result, err := r.collection.ReplaceOne(ctx, bson.M{"_id": server.ID}, mongoServerDocumentFromRemoteServer(server))
	if err != nil {
		return RemoteServer{}, &ServerRepositoryError{Op: "update", ID: server.ID, Err: ErrServerRepoOffline, Cause: err}
	}
	if result.MatchedCount == 0 {
		return RemoteServer{}, &ServerRepositoryError{Op: "update", ID: server.ID, Err: ErrServerNotFound}
	}

	return cloneRemoteServer(server), nil
}

func (r *MongoServerRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.collection == nil {
		return &ServerRepositoryError{Op: "delete", ID: id, Err: ErrServerRepositoryNotConfigured}
	}
	if id == "" {
		return &ServerRepositoryError{Op: "delete", Err: fmt.Errorf("id is required")}
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return &ServerRepositoryError{Op: "delete", ID: id, Err: ErrServerRepoOffline, Cause: err}
	}
	if result.DeletedCount == 0 {
		return &ServerRepositoryError{Op: "delete", ID: id, Err: ErrServerNotFound}
	}

	return nil
}

func (r *MongoServerRepository) SeedIfEmpty(ctx context.Context, servers []RemoteServer) error {
	if r == nil || r.collection == nil {
		return &ServerRepositoryError{Op: "seed", Err: ErrServerRepositoryNotConfigured}
	}
	if len(servers) == 0 {
		return nil
	}

	count, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return &ServerRepositoryError{Op: "seed", Err: ErrServerRepoOffline, Cause: err}
	}
	if count > 0 {
		return nil
	}

	normalized := make([]RemoteServer, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		server, err = normalizeRemoteServer(server)
		if err != nil {
			return &ServerRepositoryError{Op: "seed", ID: server.ID, Cause: err}
		}
		if _, exists := seen[server.ID]; exists {
			return &ServerRepositoryError{Op: "seed", ID: server.ID, Cause: fmt.Errorf("duplicate seed server id %q", server.ID)}
		}
		seen[server.ID] = struct{}{}
		normalized = append(normalized, server)
	}

	for _, server := range normalized {
		if _, err := r.collection.InsertOne(ctx, mongoServerDocumentFromRemoteServer(server)); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return &ServerRepositoryError{Op: "seed", ID: server.ID, Err: ErrServerExists, Cause: err}
			}
			return &ServerRepositoryError{Op: "seed", ID: server.ID, Err: ErrServerRepoOffline, Cause: err}
		}
	}

	return nil
}

type serverCollection interface {
	CountDocuments(context.Context, any, ...options.Lister[options.CountOptions]) (int64, error)
	DeleteOne(context.Context, any, ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error)
	Find(context.Context, any, ...options.Lister[options.FindOptions]) (serverCursor, error)
	FindOne(context.Context, any, ...options.Lister[options.FindOneOptions]) serverSingleResult
	InsertOne(context.Context, any, ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error)
	ReplaceOne(context.Context, any, any, ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error)
}

type serverCursor interface {
	Close(context.Context) error
	Decode(any) error
	Err() error
	Next(context.Context) bool
}

type serverSingleResult interface {
	Decode(any) error
	Err() error
}

type mongoCollectionAdapter struct {
	collection *mongo.Collection
}

func (a mongoCollectionAdapter) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	return a.collection.CountDocuments(ctx, filter, opts...)
}

func (a mongoCollectionAdapter) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	return a.collection.DeleteOne(ctx, filter, opts...)
}

func (a mongoCollectionAdapter) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (serverCursor, error) {
	cur, err := a.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	return mongoCursorAdapter{cursor: cur}, nil
}

func (a mongoCollectionAdapter) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) serverSingleResult {
	return mongoSingleResultAdapter{single: a.collection.FindOne(ctx, filter, opts...)}
}

func (a mongoCollectionAdapter) InsertOne(ctx context.Context, document any, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error) {
	return a.collection.InsertOne(ctx, document, opts...)
}

func (a mongoCollectionAdapter) ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error) {
	return a.collection.ReplaceOne(ctx, filter, replacement, opts...)
}

type mongoCursorAdapter struct {
	cursor *mongo.Cursor
}

func (a mongoCursorAdapter) Close(ctx context.Context) error {
	return a.cursor.Close(ctx)
}

func (a mongoCursorAdapter) Decode(v any) error {
	return a.cursor.Decode(v)
}

func (a mongoCursorAdapter) Err() error {
	return a.cursor.Err()
}

func (a mongoCursorAdapter) Next(ctx context.Context) bool {
	return a.cursor.Next(ctx)
}

type mongoSingleResultAdapter struct {
	single *mongo.SingleResult
}

func (a mongoSingleResultAdapter) Decode(v any) error {
	return a.single.Decode(v)
}

func (a mongoSingleResultAdapter) Err() error {
	return a.single.Err()
}

type mongoServerDocument struct {
	ID          string   `bson:"_id"`
	Name        string   `bson:"name"`
	Host        string   `bson:"host"`
	Port        int      `bson:"port,omitempty"`
	User        string   `bson:"user"`
	KeyPath     string   `bson:"keyPath,omitempty"`
	KeyEnv      string   `bson:"keyEnv,omitempty"`
	Password    string   `bson:"password,omitempty"`
	PasswordEnv string   `bson:"passwordEnv,omitempty"`
	Tags        []string `bson:"tags,omitempty"`
	Enabled     *bool    `bson:"enabled,omitempty"`
}

func mongoServerDocumentFromRemoteServer(server RemoteServer) mongoServerDocument {
	server = cloneRemoteServer(server)
	return mongoServerDocument{
		ID:          server.ID,
		Name:        server.Name,
		Host:        server.Host,
		Port:        server.Port,
		User:        server.User,
		KeyPath:     server.KeyPath,
		KeyEnv:      server.KeyEnv,
		Password:    server.Password,
		PasswordEnv: server.PasswordEnv,
		Tags:        cloneStringSlice(server.Tags),
		Enabled:     cloneBoolPtr(server.Enabled),
	}
}

func (d mongoServerDocument) toRemoteServer() RemoteServer {
	server := RemoteServer{
		ID:          d.ID,
		Name:        d.Name,
		Host:        d.Host,
		Port:        d.Port,
		User:        d.User,
		KeyPath:     d.KeyPath,
		KeyEnv:      d.KeyEnv,
		Password:    d.Password,
		PasswordEnv: d.PasswordEnv,
		Tags:        cloneStringSlice(d.Tags),
		Enabled:     cloneBoolPtr(d.Enabled),
	}
	server.applyDefaults()
	return server
}

func normalizeRemoteServer(server RemoteServer) (RemoteServer, error) {
	server = cloneRemoteServer(server)
	if server.ID == "" {
		return RemoteServer{}, fmt.Errorf("id is required")
	}
	server.applyDefaults()
	if err := server.validate(); err != nil {
		return RemoteServer{}, err
	}
	return server, nil
}

func cloneRemoteServer(server RemoteServer) RemoteServer {
	out := server
	out.Tags = cloneStringSlice(server.Tags)
	out.Enabled = cloneBoolPtr(server.Enabled)
	return out
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
