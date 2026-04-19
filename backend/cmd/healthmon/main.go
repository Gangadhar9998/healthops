package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"medics-health-check/backend/internal/monitoring"
)

func main() {
	logger := log.New(os.Stdout, "healthmon ", log.LstdFlags|log.Lmicroseconds)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configPath := resolvePath("CONFIG_PATH", filepath.Join("backend", "config", "default.json"), filepath.Join("config", "default.json"))
	mongoURI := os.Getenv("MONGODB_URI")
	mongoDB := envOrDefault("MONGODB_DATABASE", "healthmon")
	mongoPrefix := envOrDefault("MONGODB_COLLECTION_PREFIX", "healthmon")

	cfg, err := monitoring.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	var store monitoring.Store
	statePath := resolvePath("STATE_PATH", filepath.Join("backend", "data", "state.json"), filepath.Join("data", "state.json"))

	if mongoURI != "" {
		// Use HybridStore with MongoDB mirror
		store, err = monitoring.NewHybridStore(statePath, cfg.Checks, mongoURI, mongoDB, mongoPrefix, cfg.RetentionDays, logger)
		if err != nil {
			logger.Fatalf("init hybrid store: %v", err)
		}
		logger.Printf("running with hybrid storage (local file + MongoDB mirror)")
	} else {
		// Use only local file store
		store, err = monitoring.NewFileStore(statePath, cfg.Checks)
		if err != nil {
			logger.Fatalf("init file store: %v", err)
		}
		logger.Printf("running with local file persistence")
	}

	service := monitoring.NewService(cfg, store, logger)

	// Initialize incident manager
	incidentRepo := monitoring.NewMemoryIncidentRepository()
	incidentManager := monitoring.NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	// Initialize generic repositories (notification outbox, AI queue, snapshots)
	dataDir := resolvePath("DATA_DIR", filepath.Join("backend", "data"), "data")

	outbox, err := monitoring.NewFileNotificationOutbox(filepath.Join(dataDir, "notification_outbox.jsonl"))
	if err != nil {
		logger.Printf("Warning: Failed to init notification outbox: %v", err)
	}

	aiQueue, err := monitoring.NewFileAIQueue(dataDir)
	if err != nil {
		logger.Printf("Warning: Failed to init AI queue: %v", err)
	}

	snapshotRepo, err := monitoring.NewFileSnapshotRepository(filepath.Join(dataDir, "incident_snapshots.jsonl"))
	if err != nil {
		logger.Printf("Warning: Failed to init snapshot repository: %v", err)
	}

	// Initialize MySQL-specific repositories if mysql checks exist
	hasMySQLChecks := false
	for _, check := range cfg.Checks {
		if check.Type == "mysql" {
			hasMySQLChecks = true
			break
		}
	}

	if hasMySQLChecks {
		mysqlRepo, err := monitoring.NewFileMySQLRepository(dataDir)
		if err != nil {
			logger.Fatalf("init mysql repository: %v", err)
		}

		sampler := monitoring.NewLiveMySQLSampler()
		service.Runner().SetMySQLSampler(sampler)

		// Initialize MySQL rule engine
		mysqlRules := monitoring.DefaultMySQLRules()
		ruleEngine, err := monitoring.NewMySQLRuleEngine(mysqlRules, dataDir)
		if err != nil {
			logger.Fatalf("init mysql rule engine: %v", err)
		}
		_ = ruleEngine // will be used in scheduler callbacks

		// Set up MySQL API handler
		var auditLogger *monitoring.AuditLogger // service creates its own; we pass nil and let it use the service's
		mysqlAPIHandler := monitoring.NewMySQLAPIHandler(mysqlRepo, snapshotRepo, outbox, aiQueue, auditLogger, cfg)
		service.SetMySQLAPIHandler(mysqlAPIHandler)

		logger.Printf("MySQL monitoring enabled with %d default rules", len(mysqlRules))
	}

	// Initialize retention job
	retentionCfg := monitoring.DefaultRetentionConfig()
	retentionJob := monitoring.NewRetentionJob(retentionCfg, logger)
	if snapshotRepo != nil {
		retentionJob.Register("snapshots", snapshotRepo, retentionCfg.SnapshotRetentionDays)
	}
	if outbox != nil {
		retentionJob.Register("notifications", outbox, retentionCfg.NotificationRetentionDays)
	}
	if aiQueue != nil {
		retentionJob.Register("ai_queue", aiQueue, retentionCfg.AIQueueRetentionDays)
	}

	// Initialize BYOK AI service
	aiConfigStore, err := monitoring.NewAIConfigStore(dataDir)
	if err != nil {
		logger.Printf("Warning: Failed to init AI config store: %v", err)
	}

	if aiConfigStore != nil && aiQueue != nil {
		aiService := monitoring.NewAIService(aiConfigStore, aiQueue, incidentRepo, snapshotRepo, store, logger)
		aiService.StartWorker()
		defer aiService.StopWorker()

		aiAPIHandler := monitoring.NewAIAPIHandler(aiService, aiConfigStore, nil, cfg)
		service.SetAIAPIHandler(aiAPIHandler)

		// Wire auto-analysis: enqueue AI analysis when incidents are created
		incidentManager.SetOnIncidentCreated(func(incident monitoring.Incident) {
			if err := aiService.EnqueueIncidentAnalysis(incident.ID); err != nil {
				logger.Printf("AI: failed to enqueue analysis for incident %s: %v", incident.ID, err)
			}
		})

		logger.Printf("BYOK AI service initialized (background worker active)")
	}

	stopRetention := make(chan struct{})
	retentionJob.RunDaily(stopRetention)
	defer close(stopRetention)

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("service stopped: %v", err)
	}
}

func resolvePath(envKey string, candidates ...string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
