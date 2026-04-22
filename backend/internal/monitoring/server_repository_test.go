package monitoring

import (
	"context"
	"errors"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoServerRepositoryCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMongoServerRepositoryWithCollection(newFakeServerCollection())

	created, err := repo.Create(ctx, RemoteServer{
		ID:       "validation",
		Name:     "Validation",
		Host:     "13.127.106.100",
		User:     "sai",
		Password: "sai@123",
		Tags:     []string{"validation"},
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if created.Port != 22 {
		t.Fatalf("expected default port 22, got %d", created.Port)
	}
	if created.Enabled == nil || !*created.Enabled {
		t.Fatalf("expected server to default enabled")
	}

	got, err := repo.Get(ctx, "validation")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if got.Host != "13.127.106.100" || got.User != "sai" {
		t.Fatalf("unexpected server payload: %+v", got)
	}
	if got.Port != 22 {
		t.Fatalf("expected default port 22, got %d", got.Port)
	}
	if got.Enabled == nil || !*got.Enabled {
		t.Fatalf("expected enabled server from get")
	}

	updated, err := repo.Update(ctx, RemoteServer{
		ID:          "validation",
		Name:        "Validation",
		Host:        "13.127.106.101",
		User:        "sai",
		PasswordEnv: "VALIDATION_SSH_PASSWORD",
		Tags:        []string{"validation", "ssh"},
	})
	if err != nil {
		t.Fatalf("update server: %v", err)
	}
	if updated.PasswordEnv != "VALIDATION_SSH_PASSWORD" {
		t.Fatalf("expected password env update, got %+v", updated)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 server, got %d", len(listed))
	}
	if listed[0].Host != "13.127.106.101" {
		t.Fatalf("expected updated host, got %+v", listed[0])
	}
	if listed[0].PasswordEnv != "VALIDATION_SSH_PASSWORD" {
		t.Fatalf("expected updated password env, got %+v", listed[0])
	}

	if err := repo.Delete(ctx, "validation"); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	if _, err := repo.Get(ctx, "validation"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestMongoServerRepositoryTypedErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMongoServerRepositoryWithCollection(newFakeServerCollection())

	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected get missing to wrap ErrServerNotFound, got %v", err)
	}

	if _, err := repo.Update(ctx, RemoteServer{
		ID:       "missing",
		Name:     "Missing",
		Host:     "127.0.0.1",
		User:     "root",
		Password: "secret",
	}); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected update missing to wrap ErrServerNotFound, got %v", err)
	}

	if err := repo.Delete(ctx, "missing"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected delete missing to wrap ErrServerNotFound, got %v", err)
	}

	if _, err := repo.Create(ctx, RemoteServer{
		ID:       "production",
		Name:     "Production",
		Host:     "13.233.171.43",
		User:     "sai",
		Password: "Ubq@1234",
	}); err != nil {
		t.Fatalf("create production server: %v", err)
	}

	if _, err := repo.Create(ctx, RemoteServer{
		ID:       "production",
		Name:     "Production Duplicate",
		Host:     "13.233.171.44",
		User:     "sai",
		Password: "Ubq@1234",
	}); !errors.Is(err, ErrServerExists) {
		t.Fatalf("expected duplicate create to wrap ErrServerExists, got %v", err)
	}
}

func TestMongoServerRepositorySeedIfEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMongoServerRepositoryWithCollection(newFakeServerCollection())

	seed := []RemoteServer{
		{
			ID:       "validation",
			Name:     "Validation",
			Host:     "13.127.106.100",
			User:     "sai",
			Password: "sai@123",
		},
		{
			ID:       "production",
			Name:     "Production",
			Host:     "13.233.171.43",
			User:     "sai",
			Password: "Ubq@1234",
		},
	}

	if err := repo.SeedIfEmpty(ctx, seed); err != nil {
		t.Fatalf("seed empty repository: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list after seed: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 servers after seed, got %d", len(listed))
	}

	if err := repo.SeedIfEmpty(ctx, []RemoteServer{{
		ID:       "ignored",
		Name:     "Ignored",
		Host:     "10.0.0.10",
		User:     "sai",
		Password: "ignored",
	}}); err != nil {
		t.Fatalf("seed non-empty repository should be no-op: %v", err)
	}

	listed, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("list after no-op seed: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected repository to remain at 2 servers, got %d", len(listed))
	}
}

func TestMongoServerRepositorySeedRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMongoServerRepositoryWithCollection(newFakeServerCollection())

	err := repo.SeedIfEmpty(ctx, []RemoteServer{
		{
			ID:       "dup",
			Name:     "Duplicate One",
			Host:     "10.0.0.1",
			User:     "sai",
			Password: "secret",
		},
		{
			ID:       "dup",
			Name:     "Duplicate Two",
			Host:     "10.0.0.2",
			User:     "sai",
			Password: "secret",
		},
	})
	if err == nil {
		t.Fatal("expected duplicate seed IDs to fail")
	}
}

func TestMongoServerRepositoryOfflineErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMongoServerRepositoryWithCollection(&fakeServerCollection{
		docs:    make(map[string]mongoServerDocument),
		findErr: errors.New("mongo unavailable"),
	})

	if _, err := repo.List(ctx); !errors.Is(err, ErrServerRepoOffline) {
		t.Fatalf("expected list to wrap ErrServerRepoOffline, got %v", err)
	}
}

type fakeServerCollection struct {
	docs       map[string]mongoServerDocument
	countErr   error
	deleteErr  error
	findErr    error
	findOneErr error
	insertErr  error
	replaceErr error
}

func newFakeServerCollection() *fakeServerCollection {
	return &fakeServerCollection{docs: make(map[string]mongoServerDocument)}
}

func (c *fakeServerCollection) CountDocuments(context.Context, any, ...options.Lister[options.CountOptions]) (int64, error) {
	if c.countErr != nil {
		return 0, c.countErr
	}
	return int64(len(c.docs)), nil
}

func (c *fakeServerCollection) DeleteOne(_ context.Context, filter any, _ ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	if c.deleteErr != nil {
		return nil, c.deleteErr
	}
	id, ok := fakeServerIDFromFilter(filter)
	if !ok {
		return nil, errors.New("unsupported delete filter")
	}
	if _, exists := c.docs[id]; !exists {
		return &mongo.DeleteResult{}, nil
	}
	delete(c.docs, id)
	return &mongo.DeleteResult{DeletedCount: 1}, nil
}

func (c *fakeServerCollection) Find(context.Context, any, ...options.Lister[options.FindOptions]) (serverCursor, error) {
	if c.findErr != nil {
		return nil, c.findErr
	}
	ids := make([]string, 0, len(c.docs))
	for id := range c.docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	docs := make([]mongoServerDocument, 0, len(ids))
	for _, id := range ids {
		docs = append(docs, c.docs[id])
	}
	return &fakeServerCursor{docs: docs, index: -1}, nil
}

func (c *fakeServerCollection) FindOne(_ context.Context, filter any, _ ...options.Lister[options.FindOneOptions]) serverSingleResult {
	if c.findOneErr != nil {
		return fakeServerSingleResult{err: c.findOneErr}
	}
	id, ok := fakeServerIDFromFilter(filter)
	if !ok {
		return fakeServerSingleResult{err: errors.New("unsupported find filter")}
	}
	doc, exists := c.docs[id]
	if !exists {
		return fakeServerSingleResult{err: mongo.ErrNoDocuments}
	}
	return fakeServerSingleResult{doc: doc}
}

func (c *fakeServerCollection) InsertOne(_ context.Context, document any, _ ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error) {
	if c.insertErr != nil {
		return nil, c.insertErr
	}
	doc, err := fakeServerDocument(document)
	if err != nil {
		return nil, err
	}
	if _, exists := c.docs[doc.ID]; exists {
		return nil, mongo.WriteException{
			WriteErrors: mongo.WriteErrors{
				{Code: 11000, Message: "duplicate key"},
			},
		}
	}
	c.docs[doc.ID] = doc
	return &mongo.InsertOneResult{InsertedID: doc.ID}, nil
}

func (c *fakeServerCollection) ReplaceOne(_ context.Context, filter any, replacement any, _ ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error) {
	if c.replaceErr != nil {
		return nil, c.replaceErr
	}
	id, ok := fakeServerIDFromFilter(filter)
	if !ok {
		return nil, errors.New("unsupported replace filter")
	}
	if _, exists := c.docs[id]; !exists {
		return &mongo.UpdateResult{}, nil
	}

	doc, err := fakeServerDocument(replacement)
	if err != nil {
		return nil, err
	}
	c.docs[id] = doc
	return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
}

type fakeServerCursor struct {
	docs  []mongoServerDocument
	index int
}

func (c *fakeServerCursor) Close(context.Context) error {
	return nil
}

func (c *fakeServerCursor) Decode(v any) error {
	if c.index < 0 || c.index >= len(c.docs) {
		return errors.New("cursor out of range")
	}
	return fakeDecode(c.docs[c.index], v)
}

func (c *fakeServerCursor) Err() error {
	return nil
}

func (c *fakeServerCursor) Next(context.Context) bool {
	next := c.index + 1
	if next >= len(c.docs) {
		return false
	}
	c.index = next
	return true
}

type fakeServerSingleResult struct {
	doc mongoServerDocument
	err error
}

func (r fakeServerSingleResult) Decode(v any) error {
	if r.err != nil {
		return r.err
	}
	return fakeDecode(r.doc, v)
}

func (r fakeServerSingleResult) Err() error {
	return r.err
}

func fakeServerDocument(v any) (mongoServerDocument, error) {
	var doc mongoServerDocument
	if err := fakeDecode(v, &doc); err != nil {
		return mongoServerDocument{}, err
	}
	return doc, nil
}

func fakeDecode(from any, to any) error {
	raw, err := bson.Marshal(from)
	if err != nil {
		return err
	}
	return bson.Unmarshal(raw, to)
}

func fakeServerIDFromFilter(filter any) (string, bool) {
	switch f := filter.(type) {
	case bson.M:
		id, ok := f["_id"].(string)
		return id, ok
	case bson.D:
		for _, item := range f {
			if item.Key == "_id" {
				id, ok := item.Value.(string)
				return id, ok
			}
		}
	}
	return "", false
}
