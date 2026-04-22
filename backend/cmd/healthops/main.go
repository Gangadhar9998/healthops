package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"medics-health-check/backend/internal/monitoring"
	"medics-health-check/backend/internal/monitoring/ai"
	airepositories "medics-health-check/backend/internal/monitoring/ai/repositories"
	"medics-health-check/backend/internal/monitoring/mysql"
	"medics-health-check/backend/internal/monitoring/notify"
	"medics-health-check/backend/internal/monitoring/repositories"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	logger := log.New(os.Stdout, "healthops ", log.LstdFlags|log.Lmicroseconds)

	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		logger.Printf("Warning: .env file not found or error loading: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configPath := resolvePath("CONFIG_PATH", filepath.Join("backend", "config", "default.json"), filepath.Join("config", "default.json"))
	mongoURI := os.Getenv("MONGODB_URI")
	mongoDB := envOrDefault("MONGODB_DATABASE", "healthops")
	mongoPrefix := envOrDefault("MONGODB_COLLECTION_PREFIX", "healthops")

	cfg, err := monitoring.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	var store monitoring.Store
	var hybridStore *monitoring.HybridStore
	statePath := resolvePath("STATE_PATH", filepath.Join("backend", "data", "state.json"), filepath.Join("data", "state.json"))

	if mongoURI != "" {
		// Use HybridStore with MongoDB as primary, local file as fallback
		hs, err := monitoring.NewHybridStore(statePath, cfg.Checks, mongoURI, mongoDB, mongoPrefix, cfg.RetentionDays, logger)
		if err != nil {
			logger.Fatalf("init hybrid store: %v", err)
		}
		hybridStore = hs
		store = hs
		logger.Printf("running with MongoDB as primary storage (local file fallback)")
	} else {
		// Use only local file store
		store, err = monitoring.NewFileStore(statePath, cfg.Checks)
		if err != nil {
			logger.Fatalf("init file store: %v", err)
		}
		logger.Printf("running with local file persistence")
	}

	service := monitoring.NewService(cfg, store, logger)

	// Warn about insecure auth configuration
	if !cfg.Auth.Enabled {
		logger.Printf("SECURITY WARNING: Authentication is disabled — set auth.enabled=true in config for production use")
	}

	// Initialize MongoDB client for repositories (separate from HybridStore's MongoMirror)
	var mongoClient *mongo.Client
	if mongoURI != "" {
		clientOpts := options.Client().
			ApplyURI(mongoURI).
			SetServerSelectionTimeout(10 * time.Second).
			SetConnectTimeout(10 * time.Second).
			SetMaxPoolSize(100)

		var err error
		mongoClient, err = mongo.Connect(clientOpts)
		if err != nil {
			logger.Printf("Warning: Failed to connect to MongoDB for repositories: %v", err)
			logger.Printf("Repositories will use file-based fallback")
			mongoClient = nil
		} else {
			// Ping to verify connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := mongoClient.Ping(ctx, nil); err != nil {
				logger.Printf("Warning: MongoDB ping for repositories failed: %v", err)
				logger.Printf("Repositories will use file-based fallback")
				_ = mongoClient.Disconnect(context.Background())
				mongoClient = nil
			} else {
				logger.Printf("MongoDB connection for repositories established")
				cancel()
			}
		}
	}

	// Initialize user store with MongoDB if available, otherwise file-based
	dataDir := resolvePath("DATA_DIR", filepath.Join("backend", "data"), "data")
	monitoring.InitJWTSecret(dataDir)

	if mongoClient != nil {
		mongoUserRepo, err := repositories.NewMongoUserRepository(mongoClient, mongoDB, mongoPrefix)
		if err != nil {
			logger.Printf("Warning: Failed to init MongoDB user repository: %v", err)
			logger.Printf("Falling back to file-based user store")
			userStore, err := monitoring.NewUserStore(dataDir)
			if err != nil {
				logger.Printf("Warning: Failed to init file-based user store: %v", err)
			} else {
				service.SetUserStore(userStore)
				if userStore.IsUsingDefaultCredentials() {
					logger.Printf("WARNING: User management using default credentials — change immediately in production")
				} else {
					logger.Printf("User management initialized (file-based)")
				}
			}
		} else {
			bootstrapPassword := os.Getenv("HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD")
			bootstrapEmail := envOrDefault("HEALTHOPS_BOOTSTRAP_ADMIN_EMAIL", "admin@healthops.local")
			bootstrapReset := envOrDefault("HEALTHOPS_BOOTSTRAP_ADMIN_RESET", "false") == "true"
			if bootstrapPassword != "" {
				bootstrapCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				changed, err := mongoUserRepo.BootstrapAdmin(bootstrapCtx, bootstrapPassword, bootstrapEmail, bootstrapReset)
				cancel()
				if err != nil {
					logger.Printf("Warning: Failed to bootstrap MongoDB admin user: %v", err)
				} else if changed {
					logger.Printf("MongoDB admin bootstrap applied")
				}
			}

			service.SetUserStore(repositories.NewUserStoreAdapter(mongoUserRepo))
			logger.Printf("User management initialized (MongoDB)")
		}
	} else {
		userStore, err := monitoring.NewUserStore(dataDir)
		if err != nil {
			logger.Printf("Warning: Failed to init user store: %v", err)
		} else {
			service.SetUserStore(userStore)
			if userStore.IsUsingDefaultCredentials() {
				logger.Printf("WARNING: User management using default credentials — change immediately in production")
			} else {
				logger.Printf("User management initialized")
			}
		}
	}

	// Initialize incident manager
	incidentRepo := monitoring.NewMemoryIncidentRepository()
	incidentManager := monitoring.NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	// Start MongoDB health monitor — creates incidents when MongoDB goes down
	if hybridStore != nil && mongoURI != "" {
		stopMongoMonitor := make(chan struct{})
		go monitorMongoDB(hybridStore, incidentManager, logger, stopMongoMonitor)
		defer close(stopMongoMonitor)
	}

	// Initialize generic repositories (notification outbox, AI queue, snapshots)
	outbox, err := notify.NewFileNotificationOutbox(filepath.Join(dataDir, "notification_outbox.jsonl"))
	if err != nil {
		logger.Printf("Warning: Failed to init notification outbox: %v", err)
	}

	// Initialize notification channel store and dispatcher
	var channelStore notify.ChannelStore
	if hybridStore != nil && hybridStore.HasMongo() && !hybridStore.IsMongoDown() && mongoURI != "" {
		mongoChannelRepo, err := repositories.NewMongoChannelRepository(mongoURI, mongoDB, mongoPrefix, 5)
		if err != nil {
			logger.Printf("Warning: Failed to init MongoDB channel repository: %v", err)
			logger.Printf("Falling back to file-based channel store")
			channelStore, err = notify.NewNotificationChannelStore(dataDir)
			if err != nil {
				logger.Printf("Warning: Failed to init file-based channel store: %v", err)
			}
		} else {
			channelStore = repositories.NewChannelStoreAdapter(mongoChannelRepo)
			logger.Printf("MongoDB notification channel repository initialized")
		}
	} else {
		channelStore, err = notify.NewNotificationChannelStore(dataDir)
		if err != nil {
			logger.Printf("Warning: Failed to init notification channel store: %v", err)
		}
	}

	var notificationDispatcher *notify.NotificationDispatcher
	if channelStore != nil {
		notificationDispatcher = notify.NewNotificationDispatcher(channelStore, outbox, logger)
		defer notificationDispatcher.Stop() // flush pending notifications on shutdown
		// Set dashboard URL for email links
		addr := cfg.Server.Addr
		if addr == "" || addr == ":8080" {
			addr = "http://localhost:8080"
		}
		notificationDispatcher.SetDashboardURL(addr)
		notificationAPIHandler := notify.NewNotificationAPIHandler(channelStore, notificationDispatcher, cfg)
		service.SetNotifyRoutes(notificationAPIHandler)
		logger.Printf("Notification channels initialized")
	}

	aiQueue, err := ai.NewFileAIQueue(dataDir)
	if err != nil {
		logger.Printf("Warning: Failed to init AI queue: %v", err)
	}

	snapshotRepo, err := monitoring.NewFileSnapshotRepository(filepath.Join(dataDir, "incident_snapshots.jsonl"))
	if err != nil {
		logger.Printf("Warning: Failed to init snapshot repository: %v", err)
	}

	// Initialize server metrics repository (for SSH process/metrics history)
	serverMetricsRepo := monitoring.NewServerMetricsRepository(dataDir)
	service.SetServerMetricsRepo(serverMetricsRepo)

	// Initialize MySQL-specific repositories if mysql checks exist
	hasMySQLChecks := false
	var mysqlRepo monitoring.MySQLMetricsRepository
	for _, check := range cfg.Checks {
		if check.Type == "mysql" {
			hasMySQLChecks = true
			break
		}
	}

	if hasMySQLChecks {
		repo, err := mysql.NewFileMySQLRepository(dataDir)
		if err != nil {
			logger.Fatalf("init mysql repository: %v", err)
		}
		mysqlRepo = repo

		sampler := mysql.NewLiveMySQLSampler()
		service.Runner().SetMySQLSampler(sampler)
		service.Runner().SetMySQLRepo(mysqlRepo)

		// Initialize MySQL rule engine with MongoDB if available
		var ruleEngine *monitoring.MySQLRuleEngine
		if hybridStore != nil && hybridStore.HasMongo() && !hybridStore.IsMongoDown() && mongoURI != "" {
			alertRuleRepo, err := repositories.NewMongoAlertRuleRepository(mongoURI, mongoDB, mongoPrefix)
			if err != nil {
				logger.Printf("Warning: Failed to init MongoDB alert rule repository: %v", err)
				logger.Printf("Using default MySQL rules with file-based state")
				mysqlRules := monitoring.DefaultMySQLRules()
				ruleEngine, err = monitoring.NewMySQLRuleEngine(mysqlRules, dataDir)
				if err != nil {
					logger.Fatalf("init mysql rule engine: %v", err)
				}
			} else {
				logger.Printf("MongoDB alert rule repository initialized")
				// Seed default MySQL rules if empty
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				mysqlRules := monitoring.DefaultMySQLRules()
				// Check if we need to seed
				rules, err := alertRuleRepo.List(ctx)
				if err != nil || len(rules) == 0 {
					logger.Printf("Seeding default MySQL alert rules to MongoDB")
					for i := range mysqlRules {
						if err := alertRuleRepo.Create(ctx, &mysqlRules[i]); err != nil {
							logger.Printf("Warning: Failed to seed rule %s: %v", mysqlRules[i].ID, err)
						}
					}
				}
				// Load rules from MongoDB for the engine
				loadedRules, err := alertRuleRepo.List(ctx)
				if err != nil {
					logger.Printf("Warning: Failed to load rules from MongoDB: %v", err)
					logger.Printf("Using default MySQL rules")
					loadedRules = mysqlRules
				}
				ruleEngine, err = monitoring.NewMySQLRuleEngine(loadedRules, dataDir)
				if err != nil {
					logger.Fatalf("init mysql rule engine: %v", err)
				}
			}
		} else {
			mysqlRules := monitoring.DefaultMySQLRules()
			ruleEngine, err = monitoring.NewMySQLRuleEngine(mysqlRules, dataDir)
			if err != nil {
				logger.Fatalf("init mysql rule engine: %v", err)
			}
		}

		// Wire rule engine + incident manager + outbox + snapshots into runner
		service.Runner().SetMySQLRuleEngine(ruleEngine)
		service.Runner().SetIncidentManager(incidentManager)
		if outbox != nil {
			service.Runner().SetNotificationOutbox(outbox)
		}
		if snapshotRepo != nil {
			service.Runner().SetSnapshotRepo(snapshotRepo)
		}

		// Set up MySQL API handler
		var auditLogger *monitoring.AuditLogger // service creates its own; we pass nil and let it use the service's
		mysqlAPIHandler := mysql.NewMySQLAPIHandler(mysqlRepo, snapshotRepo, outbox, aiQueue, auditLogger, cfg)
		service.SetMySQLRoutes(mysqlAPIHandler)
		service.SetSnapshotRepo(snapshotRepo)

		logger.Printf("MySQL monitoring enabled")
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
	// Prune resolved incidents to prevent unbounded memory growth
	retentionJob.Register("incidents", incidentRepo, retentionCfg.IncidentRetentionDays)
	// Prune old server metric snapshots
	retentionJob.Register("server_metrics", serverMetricsRepo, retentionCfg.SnapshotRetentionDays)

	// Initialize BYOK AI service with MongoDB if available, otherwise file-based
	var aiConfigStore ai.AIConfigStoreInterface
	if hybridStore != nil && hybridStore.HasMongo() && !hybridStore.IsMongoDown() && mongoURI != "" {
		mongoAIConfigRepo, err := airepositories.NewMongoAIConfigRepository(airepositories.MongoAIConfigRepositoryConfig{
			MongoURI:       mongoURI,
			DatabaseName:   mongoDB,
			CollectionName: mongoPrefix + "_ai_config",
			DataDir:        dataDir,
			RetentionDays:  cfg.RetentionDays,
		})
		if err != nil {
			logger.Printf("Warning: Failed to init MongoDB AI config repository: %v", err)
			logger.Printf("Falling back to file-based AI config store")
			aiConfigStore, err = ai.NewAIConfigStore(dataDir)
			if err != nil {
				logger.Printf("Warning: Failed to init file-based AI config store: %v", err)
			}
		} else {
			logger.Printf("MongoDB AI config repository initialized")
			aiConfigStore = airepositories.NewMongoAIConfigStoreAdapter(mongoAIConfigRepo)
		}
	} else {
		aiConfigStore, err = ai.NewAIConfigStore(dataDir)
		if err != nil {
			logger.Printf("Warning: Failed to init AI config store: %v", err)
		}
	}
	if aiConfigStore != nil && aiQueue != nil {
		aiService := ai.NewAIService(aiConfigStore, aiQueue, incidentRepo, snapshotRepo, store, logger)
		aiService.StartWorker()
		defer aiService.StopWorker()

		aiAPIHandler := ai.NewAIAPIHandler(aiService, aiConfigStore, nil, cfg)
		if mysqlRepo != nil {
			aiAPIHandler.SetMySQLRepo(mysqlRepo)
		}
		service.SetAIRoutes(aiAPIHandler)

		// Wire auto-analysis + notifications: trigger when incidents are created
		incidentManager.SetOnIncidentCreated(func(incident monitoring.Incident) {
			if err := aiService.EnqueueIncidentAnalysis(incident.ID); err != nil {
				logger.Printf("AI: failed to enqueue analysis for incident %s: %v", incident.ID, err)
			}
			if notificationDispatcher != nil {
				channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
				notificationDispatcher.NotifyIncident(incident, nil, channelIDs...)
			}
		})

		logger.Printf("BYOK AI service initialized (background worker active)")
	}

	// If AI not configured, still wire notification dispatch for incidents
	if aiConfigStore == nil || aiQueue == nil {
		if notificationDispatcher != nil {
			incidentManager.SetOnIncidentCreated(func(incident monitoring.Incident) {
				channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
				notificationDispatcher.NotifyIncident(incident, nil, channelIDs...)
			})
		}
	}

	// Wire resolution notifications (always, regardless of AI config)
	if notificationDispatcher != nil {
		incidentManager.SetOnIncidentResolved(func(incident monitoring.Incident) {
			channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
			notificationDispatcher.NotifyResolved(incident, nil, channelIDs...)
		})
	}

	stopRetention := make(chan struct{})
	retentionJob.RunDaily(stopRetention)
	defer close(stopRetention)

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("service stopped: %v", err)
	}
}

// lookupCheckChannelIDs returns the NotificationChannelIDs configured on a check.
func lookupCheckChannelIDs(store monitoring.Store, checkID string) []string {
	state := store.Snapshot()
	for _, c := range state.Checks {
		if c.ID == checkID {
			return c.NotificationChannelIDs
		}
	}
	return nil
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

// monitorMongoDB periodically pings MongoDB and creates/resolves incidents.
func monitorMongoDB(store *monitoring.HybridStore, im *monitoring.IncidentManager, logger *log.Logger, stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	const checkID = "internal-mongodb"
	const checkName = "MongoDB Primary Database"
	wasDown := store.IsMongoDown()

	// If already down at startup, create an incident immediately
	if wasDown {
		_ = im.ProcessAlert(checkID, checkName, "internal", "critical",
			"MongoDB is unreachable — operating in local file fallback mode",
			map[string]string{"component": "mongodb"},
		)
		logger.Printf("INCIDENT: MongoDB unreachable at startup — running on local file fallback")
	}

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := store.PingMongo(ctx)
			cancel()

			isDown := err != nil
			if isDown && !wasDown {
				// MongoDB just went down — create incident
				_ = im.ProcessAlert(checkID, checkName, "internal", "critical",
					"MongoDB is unreachable — operating in local file fallback mode: "+err.Error(),
					map[string]string{"component": "mongodb"},
				)
				logger.Printf("INCIDENT: MongoDB connectivity lost: %v", err)
			} else if !isDown && wasDown {
				// MongoDB recovered — auto-resolve
				_ = im.AutoResolveOnRecovery(checkID)
				logger.Printf("RESOLVED: MongoDB connectivity restored")
			}
			wasDown = isDown
		}
	}
}
