package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"medics-health-check/backend/internal/logging"
	"medics-health-check/backend/internal/monitoring"
	"medics-health-check/backend/internal/monitoring/ai"
	airepositories "medics-health-check/backend/internal/monitoring/ai/repositories"
	"medics-health-check/backend/internal/monitoring/mysql"
	"medics-health-check/backend/internal/monitoring/notify"
	"medics-health-check/backend/internal/monitoring/repositories"

	"github.com/joho/godotenv"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	slogLogger := logging.Init("")
	// Legacy *log.Logger shim so existing constructors and helpers (which
	// still take *log.Logger) emit through the structured sink at INFO level.
	// Per-package conversion of mysql/ai/notify/repositories is deferred.
	logger := logging.LegacyAdapter(slogLogger)

	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		slogLogger.Warn("env file not loaded", "error", logging.RedactErr(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configPath := resolvePath("CONFIG_PATH", filepath.Join("backend", "config", "default.json"), filepath.Join("config", "default.json"))
	mongoURI := os.Getenv("MONGODB_URI")
	mongoDB := envOrDefault("MONGODB_DATABASE", "healthops")
	mongoPrefix := envOrDefault("MONGODB_COLLECTION_PREFIX", "healthops")

	cfg, err := monitoring.LoadConfig(configPath)
	if err != nil {
		slogLogger.Error("load config", "error", logging.RedactErr(err), "path", configPath)
		os.Exit(1)
	}

	var store monitoring.Store
	var hybridStore *monitoring.HybridStore
	statePath := resolvePath("STATE_PATH", filepath.Join("backend", "data", "state.json"), filepath.Join("data", "state.json"))

	if mongoURI != "" {
		// Use HybridStore with MongoDB as primary, local file as fallback
		hs, err := monitoring.NewHybridStore(statePath, cfg.Checks, mongoURI, mongoDB, mongoPrefix, cfg.RetentionDays, logger)
		if err != nil {
			slogLogger.Error("init hybrid store", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		hybridStore = hs
		store = hs
		slogLogger.Info("storage backend selected", "mode", "mongodb_primary", "fallback", "local_file")
	} else {
		// Use only local file store
		store, err = monitoring.NewFileStore(statePath, cfg.Checks)
		if err != nil {
			slogLogger.Error("init file store", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		slogLogger.Info("storage backend selected", "mode", "local_file")
	}

	service := monitoring.NewService(cfg, store, logger)

	// Warn about insecure auth configuration
	if !cfg.Auth.Enabled {
		slogLogger.Warn("authentication disabled", "remediation", "set auth.enabled=true in config for production use")
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
			slogLogger.Warn("mongodb connect for repositories failed", "error", logging.RedactErr(err), "fallback", "file_based")
			mongoClient = nil
		} else {
			// Ping to verify connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := mongoClient.Ping(ctx, nil); err != nil {
				slogLogger.Warn("mongodb ping for repositories failed", "error", logging.RedactErr(err), "fallback", "file_based")
				_ = mongoClient.Disconnect(context.Background())
				mongoClient = nil
			} else {
				slogLogger.Info("mongodb connection for repositories established")
				cancel()
			}
		}
	}

	// Initialize user store with MongoDB if available, otherwise file-based
	dataDir := resolvePath("DATA_DIR", filepath.Join("backend", "data"), "data")
	monitoring.InitJWTSecret(dataDir)

	// Track which user-store backend is active so we can run prod-mode safety
	// checks below (default-credentials detection differs by backend).
	var activeUserStore monitoring.UserStoreBackend
	var userStoreBackendKind string // "file" or "mongo"
	mongoBootstrapProvided := os.Getenv("HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD") != ""

	if mongoClient != nil {
		mongoUserRepo, err := repositories.NewMongoUserRepository(mongoClient, mongoDB, mongoPrefix)
		if err != nil {
			slogLogger.Warn("init mongodb user repository failed", "error", logging.RedactErr(err), "fallback", "file_based_user_store")
			userStore, err := monitoring.NewUserStore(dataDir)
			if err != nil {
				slogLogger.Warn("init file-based user store failed", "error", logging.RedactErr(err))
			} else {
				service.SetUserStore(userStore)
				activeUserStore = userStore
				userStoreBackendKind = "file"
				if userStore.IsUsingDefaultCredentials() {
					slogLogger.Warn("user management using default credentials", "remediation", "change immediately in production")
				} else {
					slogLogger.Info("user management initialized", "backend", "file")
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
					slogLogger.Warn("bootstrap mongodb admin user failed", "error", logging.RedactErr(err))
				} else if changed {
					slogLogger.Info("mongodb admin bootstrap applied")
				}
			}

			adapter := repositories.NewUserStoreAdapter(mongoUserRepo)
			service.SetUserStore(adapter)
			activeUserStore = adapter
			userStoreBackendKind = "mongo"
			slogLogger.Info("user management initialized", "backend", "mongodb")
		}
	} else {
		userStore, err := monitoring.NewUserStore(dataDir)
		if err != nil {
			slogLogger.Warn("init user store failed", "error", logging.RedactErr(err))
		} else {
			service.SetUserStore(userStore)
			activeUserStore = userStore
			userStoreBackendKind = "file"
			if userStore.IsUsingDefaultCredentials() {
				slogLogger.Warn("user management using default credentials", "remediation", "change immediately in production")
			} else {
				slogLogger.Info("user management initialized", "backend", "file")
			}
		}
	}

	// Production hardening gate: when HEALTHOPS_REQUIRE_PROD_AUTH=true, refuse
	// to start with insecure defaults. This is opt-in so dev/test flows keep
	// working unchanged.
	if envOrDefault("HEALTHOPS_REQUIRE_PROD_AUTH", "false") == "true" {
		// Block on command-check RCE risk regardless of user-store backend.
		if cfg.AllowCommandChecks {
			slogLogger.Error("prod auth gate: refusing to start with allowCommandChecks=true",
				"reason", "shell command checks are an RCE risk",
				"remediation", "set allowCommandChecks=false in config or via API")
			os.Exit(1)
		}

		switch userStoreBackendKind {
		case "file":
			if activeUserStore == nil {
				slogLogger.Error("prod auth gate: user store failed to initialize",
					"reason", "refusing to start without authentication")
				os.Exit(1)
			}
			if activeUserStore.IsUsingDefaultCredentials() {
				slogLogger.Error("prod auth gate: refusing to start with default admin credentials",
					"remediation", "set HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD before first start, or change admin password via API and restart")
				os.Exit(1)
			}
		case "mongo":
			// The Mongo adapter cannot cheaply prove a non-default password
			// is in use (passwords are hashed; we'd have to attempt a login).
			// Pragmatic policy: require that the operator either bootstrapped
			// via env or accepts responsibility for credential management.
			if !mongoBootstrapProvided {
				slogLogger.Warn("prod auth gate: cannot verify admin credentials are non-default",
					"backend", "mongodb",
					"remediation", "set HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD or confirm out-of-band that the admin password has been rotated")
			}
		default:
			slogLogger.Error("prod auth gate: no user store backend was initialized",
				"reason", "refusing to start without authentication")
			os.Exit(1)
		}

		slogLogger.Info("prod auth gate: production hardening checks passed")
	}

	// Initialize incident manager
	incidentRepo := monitoring.NewMemoryIncidentRepository()
	incidentManager := monitoring.NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	// Start MongoDB health monitor — creates incidents when MongoDB goes down
	if hybridStore != nil && mongoURI != "" {
		stopMongoMonitor := make(chan struct{})
		go monitorMongoDB(hybridStore, incidentManager, slogLogger, stopMongoMonitor)
		defer close(stopMongoMonitor)
	}

	// Initialize generic repositories (notification outbox, AI queue, snapshots)
	outbox, err := notify.NewFileNotificationOutbox(filepath.Join(dataDir, "notification_outbox.jsonl"))
	if err != nil {
		slogLogger.Warn("init notification outbox failed", "error", logging.RedactErr(err))
	}

	// Initialize notification channel store and dispatcher
	var channelStore notify.ChannelStore
	if hybridStore != nil && hybridStore.HasMongo() && !hybridStore.IsMongoDown() && mongoURI != "" {
		mongoChannelRepo, err := repositories.NewMongoChannelRepository(mongoURI, mongoDB, mongoPrefix, 5)
		if err != nil {
			slogLogger.Warn("init mongodb channel repository failed", "error", logging.RedactErr(err), "fallback", "file_based_channel_store")
			channelStore, err = notify.NewNotificationChannelStore(dataDir)
			if err != nil {
				slogLogger.Warn("init file-based channel store failed", "error", logging.RedactErr(err))
			}
		} else {
			channelStore = repositories.NewChannelStoreAdapter(mongoChannelRepo)
			slogLogger.Info("notification channel repository initialized", "backend", "mongodb")
		}
	} else {
		channelStore, err = notify.NewNotificationChannelStore(dataDir)
		if err != nil {
			slogLogger.Warn("init notification channel store failed", "error", logging.RedactErr(err))
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
		slogLogger.Info("notification channels initialized")
	}

	aiQueue, err := ai.NewFileAIQueue(dataDir)
	if err != nil {
		slogLogger.Warn("init ai queue failed", "error", logging.RedactErr(err))
	}

	snapshotRepo, err := monitoring.NewFileSnapshotRepository(filepath.Join(dataDir, "incident_snapshots.jsonl"))
	if err != nil {
		slogLogger.Warn("init snapshot repository failed", "error", logging.RedactErr(err))
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
			slogLogger.Error("init mysql repository", "error", logging.RedactErr(err))
			os.Exit(1)
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
				slogLogger.Warn("init mongodb alert rule repository failed", "error", logging.RedactErr(err), "fallback", "default_rules_file_state")
				mysqlRules := monitoring.DefaultMySQLRules()
				ruleEngine, err = monitoring.NewMySQLRuleEngine(mysqlRules, dataDir)
				if err != nil {
					slogLogger.Error("init mysql rule engine", "error", logging.RedactErr(err))
					os.Exit(1)
				}
			} else {
				slogLogger.Info("mongodb alert rule repository initialized")
				// Seed default MySQL rules if empty
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				mysqlRules := monitoring.DefaultMySQLRules()
				// Check if we need to seed
				rules, err := alertRuleRepo.List(ctx)
				if err != nil || len(rules) == 0 {
					slogLogger.Info("seeding default mysql alert rules to mongodb")
					for i := range mysqlRules {
						if err := alertRuleRepo.Create(ctx, &mysqlRules[i]); err != nil {
							slogLogger.Warn("seed mysql alert rule failed", "rule_id", mysqlRules[i].ID, "error", logging.RedactErr(err))
						}
					}
				}
				// Load rules from MongoDB for the engine
				loadedRules, err := alertRuleRepo.List(ctx)
				if err != nil {
					slogLogger.Warn("load mysql rules from mongodb failed", "error", logging.RedactErr(err), "fallback", "default_rules")
					loadedRules = mysqlRules
				}
				ruleEngine, err = monitoring.NewMySQLRuleEngine(loadedRules, dataDir)
				if err != nil {
					slogLogger.Error("init mysql rule engine", "error", logging.RedactErr(err))
					os.Exit(1)
				}
			}
		} else {
			mysqlRules := monitoring.DefaultMySQLRules()
			ruleEngine, err = monitoring.NewMySQLRuleEngine(mysqlRules, dataDir)
			if err != nil {
				slogLogger.Error("init mysql rule engine", "error", logging.RedactErr(err))
				os.Exit(1)
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

		slogLogger.Info("mysql monitoring enabled")
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
			slogLogger.Warn("init mongodb ai config repository failed", "error", logging.RedactErr(err), "fallback", "file_based_ai_config")
			aiConfigStore, err = ai.NewAIConfigStore(dataDir)
			if err != nil {
				slogLogger.Warn("init file-based ai config store failed", "error", logging.RedactErr(err))
			}
		} else {
			slogLogger.Info("mongodb ai config repository initialized")
			aiConfigStore = airepositories.NewMongoAIConfigStoreAdapter(mongoAIConfigRepo)
		}
	} else {
		aiConfigStore, err = ai.NewAIConfigStore(dataDir)
		if err != nil {
			slogLogger.Warn("init ai config store failed", "error", logging.RedactErr(err))
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
				slogLogger.Warn("ai enqueue analysis failed", "incident_id", incident.ID, "error", logging.RedactErr(err))
			}
			if notificationDispatcher != nil {
				channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
				notificationDispatcher.NotifyIncident(incident, nil, channelIDs...)
			}
		})

		slogLogger.Info("byok ai service initialized", "worker", "active")
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
		slogLogger.Error("service stopped", "error", logging.RedactErr(err))
		os.Exit(1)
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
func monitorMongoDB(store *monitoring.HybridStore, im *monitoring.IncidentManager, logger *slog.Logger, stop <-chan struct{}) {
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
		logger.Error("incident: mongodb unreachable at startup", "check_id", checkID, "fallback", "local_file")
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
				logger.Error("incident: mongodb connectivity lost", "check_id", checkID, "error", logging.RedactErr(err))
			} else if !isDown && wasDown {
				// MongoDB recovered — auto-resolve
				_ = im.AutoResolveOnRecovery(checkID)
				logger.Info("resolved: mongodb connectivity restored", "check_id", checkID)
			}
			wasDown = isDown
		}
	}
}
