package monitoring

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"medics-health-check/backend/internal/util/mongotest"
)

func TestMongoMirrorSyncStateDeletesRemovedChecks(t *testing.T) {
	client := mongotest.Connect(t, 0)
	dbName := "healthops_test_mirror_delete"
	prefix := "mirror_delete"
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
	})

	mirror := &MongoMirror{
		client:  client,
		db:      db,
		checks:  db.Collection(prefix + "_checks"),
		results: db.Collection(prefix + "_results"),
		state:   db.Collection(prefix + "_state"),
	}

	ctx := context.Background()
	if err := mirror.SyncState(ctx, State{Checks: []CheckConfig{
		{ID: "keep", Name: "Keep", Type: "api"},
		{ID: "delete", Name: "Delete", Type: "api"},
	}}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	if err := mirror.SyncState(ctx, State{Checks: []CheckConfig{
		{ID: "keep", Name: "Keep", Type: "api"},
	}}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	count, err := mirror.checks.CountDocuments(ctx, bson.M{"_id": "delete"})
	if err != nil {
		t.Fatalf("count deleted check: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted check count = %d, want 0", count)
	}

	var doc CheckConfig
	err = mirror.checks.FindOne(ctx, bson.M{"_id": "keep"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			t.Fatal("kept check was removed")
		}
		t.Fatalf("find kept check: %v", err)
	}
}
