package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requiredSecret(key string, minBytes int) (string, error) {
	value, err := requiredEnv(key)
	if err != nil {
		return "", err
	}
	if len([]byte(value)) < minBytes {
		return "", fmt.Errorf("%s must be at least %d bytes", key, minBytes)
	}
	return value, nil
}

func connectMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	uri = strings.ReplaceAll(uri, "localhost", "127.0.0.1")
	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetMaxPoolSize(100)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect failed: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping failed: %w", err)
	}
	return client, nil
}
