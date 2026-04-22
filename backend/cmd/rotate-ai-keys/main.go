package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"medics-health-check/backend/internal/monitoring/ai/repositories"
)

func main() {
	// Command-line flags
	mongoURI := flag.String("mongo-uri", getEnv("MONGODB_URI", "mongodb://localhost:27017"), "MongoDB connection URI")
	database := flag.String("database", getEnv("MONGODB_DATABASE", "healthops"), "Database name")
	collection := flag.String("collection", "healthops_ai_config", "Collection name")
	dataDir := flag.String("data-dir", "data", "Data directory for encryption keys")
	keyPath := flag.String("key-path", "", "Path for new encryption key (auto-generated if empty)")
	listVersions := flag.Bool("list", false, "List current key versions")
	flag.Parse()

	// Create repository configuration
	cfg := repositories.MongoAIConfigRepositoryConfig{
		MongoURI:       *mongoURI,
		DatabaseName:   *database,
		CollectionName: *collection,
		DataDir:        *dataDir,
		RetentionDays:  7,
		KeyPath:        os.Getenv("AI_ENCRYPTION_KEY_PATH"),
	}

	// Initialize repository
	repo, err := repositories.NewMongoAIConfigRepository(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Check if MongoDB is reachable
	if err := repo.Ping(ctx); err != nil {
		log.Fatalf("MongoDB ping failed: %v", err)
	}

	// Handle list command
	if *listVersions {
		versions := repo.GetKeyVersions()
		fmt.Println("Current encryption key versions:")
		for version, info := range versions {
			infoMap, ok := info.(map[string]string)
			if !ok {
				continue
			}
			fmt.Printf("  Version %d: %s (status: %s)\n", version, infoMap["path"], infoMap["status"])
		}
		return
	}

	// Perform key rotation
	newKeyPath := *keyPath
	if newKeyPath == "" {
		// Auto-generate path with version number
		versions := repo.GetKeyVersions()
		maxVersion := 1
		for v := range versions {
			if v > maxVersion {
				maxVersion = v
			}
		}
		newKeyPath = fmt.Sprintf("%s/.ai_enc_key_v%d", *dataDir, maxVersion+1)
	}

	fmt.Printf("Starting key rotation...\n")
	fmt.Printf("New key will be saved to: %s\n", newKeyPath)

	err = repo.RotateKey(ctx, newKeyPath)
	if err != nil {
		log.Fatalf("Key rotation failed: %v", err)
	}

	fmt.Println("Key rotation completed successfully!")

	// List updated versions
	versions := repo.GetKeyVersions()
	fmt.Println("\nUpdated encryption key versions:")
	for version, info := range versions {
		infoMap, ok := info.(map[string]string)
		if !ok {
			continue
		}
		fmt.Printf("  Version %d: %s (status: %s)\n", version, infoMap["path"], infoMap["status"])
	}

	fmt.Println("\nNote: Old encryption keys are archived for recovery purposes.")
	fmt.Println("You can safely delete old key files after verifying all providers work correctly.")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
