package main

import (
	"context"
	"errors"
	"fmt"
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
)

func main() {
	slogLogger := logging.Init("")
	logger := logging.LegacyAdapter(slogLogger)

	if err := godotenv.Load(); err != nil {
		slogLogger.Warn("env file not loaded", "error", logging.RedactErr(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configPath := resolvePath("CONFIG_PATH", filepath.Join("backend", "config", "default.json"), filepath.Join("config", "default.json"))
	cfg, err := monitoring.LoadConfig(configPath)
	if err != nil {
		slogLogger.Error("load config", "error", logging.RedactErr(err), "path", configPath)
		os.Exit(1)
	}

	mongoURI, err := requiredEnv("MONGODB_URI")
	if err != nil {
		slogLogger.Error("missing required mongodb configuration", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	mongoDB, err := requiredEnv("MONGODB_DATABASE")
	if err != nil {
		slogLogger.Error("missing required mongodb configuration", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	mongoPrefix, err := requiredEnv("MONGODB_COLLECTION_PREFIX")
	if err != nil {
		slogLogger.Error("missing required mongodb configuration", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	if _, err := requiredSecret("HEALTHOPS_JWT_SECRET", 32); err != nil {
		slogLogger.Error("missing required jwt secret", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	if _, err := requiredSecret("HEALTHOPS_AI_ENCRYPTION_KEY", 32); err != nil {
		slogLogger.Error("missing required ai encryption secret", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	bootstrapPassword, err := requiredSecret("HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD", 8)
	if err != nil {
		slogLogger.Error("missing required bootstrap admin password", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	if err := monitoring.InitJWTSecretFromEnv(); err != nil {
		slogLogger.Error("init jwt secret", "error", logging.RedactErr(err))
		os.Exit(1)
	}

	mongoClient, err := connectMongo(ctx, mongoURI)
	if err != nil {
		slogLogger.Error("connect mongodb", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(shutdownCtx); err != nil {
			slogLogger.Warn("disconnect mongodb", "error", logging.RedactErr(err))
		}
	}()

	mirror, err := monitoring.NewMongoMirrorFromClient(mongoClient, mongoDB, mongoPrefix, cfg.RetentionDays)
	if err != nil {
		slogLogger.Error("init mongodb mirror", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	store, err := monitoring.NewMongoStore(mirror, cfg.Checks)
	if err != nil {
		slogLogger.Error("init mongodb store", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	slogLogger.Info("storage backend selected", "mode", "mongodb_required", "database", mongoDB, "prefix", mongoPrefix)

	serverRepo, err := monitoring.NewMongoServerRepositoryFromClient(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb server repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	serverCtx, serverCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := serverRepo.SeedIfEmpty(serverCtx, cfg.Servers); err != nil {
		serverCancel()
		slogLogger.Error("seed servers", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	servers, err := serverRepo.List(serverCtx)
	serverCancel()
	if err != nil {
		slogLogger.Error("load servers", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	cfg.Servers = servers

	service := monitoring.NewService(cfg, store, logger)
	service.SetServerRepo(serverRepo)
	service.ReplaceServers(servers)

	userRepo, err := repositories.NewMongoUserRepository(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb user repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	bootstrapCtx, bootstrapCancel := context.WithTimeout(ctx, 5*time.Second)
	changed, err := userRepo.BootstrapAdmin(
		bootstrapCtx,
		bootstrapPassword,
		envOrDefault("HEALTHOPS_BOOTSTRAP_ADMIN_EMAIL", "admin@healthops.local"),
		envOrDefault("HEALTHOPS_BOOTSTRAP_ADMIN_RESET", "false") == "true",
	)
	bootstrapCancel()
	if err != nil {
		slogLogger.Error("bootstrap mongodb admin user", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	if changed {
		slogLogger.Info("mongodb admin bootstrap applied")
	}
	service.SetUserStore(repositories.NewUserStoreAdapter(userRepo))

	auditRepo, err := monitoring.NewMongoAuditRepository(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb audit repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	auditLogger := monitoring.NewAuditLogger(auditRepo, logger)
	service.SetAuditLogger(auditLogger)

	incidentRepo, err := monitoring.NewMongoIncidentRepository(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb incident repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	incidentManager := monitoring.NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	outbox, err := notify.NewMongoNotificationOutbox(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb notification outbox", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	channelRepo, err := repositories.NewMongoChannelRepositoryFromClient(mongoClient, mongoDB, mongoPrefix, 5)
	if err != nil {
		slogLogger.Error("init mongodb channel repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	channelStore := repositories.NewChannelStoreAdapter(channelRepo)
	notificationDispatcher := notify.NewNotificationDispatcher(channelStore, outbox, logger)
	defer notificationDispatcher.Stop()
	addr := cfg.Server.Addr
	if addr == "" || addr == ":8080" {
		addr = "http://localhost:8080"
	}
	notificationDispatcher.SetDashboardURL(addr)
	service.SetNotifyRoutes(notify.NewNotificationAPIHandler(channelStore, notificationDispatcher, cfg))

	aiQueue, err := ai.NewMongoAIQueue(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb ai queue", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	snapshotRepo, err := monitoring.NewMongoSnapshotRepository(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb snapshot repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	service.SetSnapshotRepo(snapshotRepo)
	serverMetricsRepo, err := monitoring.NewMongoServerMetricsRepository(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb server metrics repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	service.SetServerMetricsRepo(serverMetricsRepo)

	alertRuleRepo, err := repositories.NewMongoAlertRuleRepositoryFromClient(mongoClient, mongoDB, mongoPrefix)
	if err != nil {
		slogLogger.Error("init mongodb alert rule repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	if err := seedDefaultAlertRules(ctx, alertRuleRepo, monitoring.DefaultAlertRules()); err != nil {
		slogLogger.Error("seed default alert rules", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	alertRules, err := alertRuleRepo.List(ctx)
	if err != nil {
		slogLogger.Error("load alert rules", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	service.SetAlertRuleRepo(alertRuleRepo)
	service.SetAlertEngine(monitoring.NewAlertRuleEngine(alertRules, logger))

	var mysqlRepo monitoring.MySQLMetricsRepository
	if hasMySQLChecks(store.Snapshot().Checks) {
		mysqlMongoRepo, err := mysql.NewMongoMySQLRepository(mongoClient, mongoDB, mongoPrefix)
		if err != nil {
			slogLogger.Error("init mongodb mysql repository", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		mysqlRepo = mysqlMongoRepo

		service.Runner().SetMySQLSampler(mysql.NewLiveMySQLSampler())
		service.Runner().SetMySQLRepo(mysqlRepo)

		if err := seedDefaultAlertRules(ctx, alertRuleRepo, monitoring.DefaultMySQLRules()); err != nil {
			slogLogger.Error("seed default mysql rules", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		allRules, err := alertRuleRepo.List(ctx)
		if err != nil {
			slogLogger.Error("load mysql rules", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		mysqlRuleStateStore, err := monitoring.NewMongoMySQLRuleStateStore(mongoClient, mongoDB, mongoPrefix)
		if err != nil {
			slogLogger.Error("init mongodb mysql rule state store", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		ruleEngine, err := monitoring.NewMySQLRuleEngineWithStateStore(mysqlRulesOnly(allRules), mysqlRuleStateStore)
		if err != nil {
			slogLogger.Error("init mysql rule engine", "error", logging.RedactErr(err))
			os.Exit(1)
		}
		service.Runner().SetMySQLRuleEngine(ruleEngine)
		service.Runner().SetIncidentManager(incidentManager)
		service.Runner().SetNotificationOutbox(outbox)
		service.Runner().SetSnapshotRepo(snapshotRepo)

		mysqlAPIHandler := mysql.NewMySQLAPIHandler(mysqlRepo, snapshotRepo, outbox, aiQueue, auditLogger, cfg)
		service.SetMySQLRoutes(mysqlAPIHandler)
		slogLogger.Info("mysql monitoring enabled")
	}

	retentionCfg := monitoring.DefaultRetentionConfig()
	retentionJob := monitoring.NewRetentionJob(retentionCfg, logger)
	retentionJob.Register("snapshots", snapshotRepo, retentionCfg.SnapshotRetentionDays)
	retentionJob.Register("notifications", outbox, retentionCfg.NotificationRetentionDays)
	retentionJob.Register("ai_queue", aiQueue, retentionCfg.AIQueueRetentionDays)
	retentionJob.Register("incidents", incidentRepo, retentionCfg.IncidentRetentionDays)
	retentionJob.Register("server_metrics", serverMetricsRepo, retentionCfg.SnapshotRetentionDays)
	if prunable, ok := mysqlRepo.(interface{ PruneBefore(time.Time) error }); ok {
		retentionJob.Register("mysql_metrics", prunable, retentionCfg.SnapshotRetentionDays)
	}

	mongoAIConfigRepo, err := airepositories.NewMongoAIConfigRepository(airepositories.MongoAIConfigRepositoryConfig{
		MongoURI:       mongoURI,
		DatabaseName:   mongoDB,
		CollectionName: mongoPrefix + "_ai_config",
		RetentionDays:  cfg.RetentionDays,
	})
	if err != nil {
		slogLogger.Error("init mongodb ai config repository", "error", logging.RedactErr(err))
		os.Exit(1)
	}
	defer func() {
		if err := mongoAIConfigRepo.Close(); err != nil {
			slogLogger.Warn("close ai config repository", "error", logging.RedactErr(err))
		}
	}()
	aiConfigStore := airepositories.NewMongoAIConfigStoreAdapter(mongoAIConfigRepo)
	aiService := ai.NewAIService(aiConfigStore, aiQueue, incidentRepo, snapshotRepo, store, logger)
	aiService.StartWorker()
	defer aiService.StopWorker()

	aiAPIHandler := ai.NewAIAPIHandler(aiService, aiConfigStore, auditLogger, cfg)
	aiAPIHandler.SetMongoAIRepo(mongoAIConfigRepo)
	if mysqlRepo != nil {
		aiAPIHandler.SetMySQLRepo(mysqlRepo)
	}
	service.SetAIRoutes(aiAPIHandler)

	incidentManager.SetOnIncidentCreated(func(incident monitoring.Incident) {
		if err := aiService.EnqueueIncidentAnalysis(incident.ID); err != nil {
			slogLogger.Warn("ai enqueue analysis failed", "incident_id", incident.ID, "error", logging.RedactErr(err))
		}
		channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
		notificationDispatcher.NotifyIncident(incident, nil, channelIDs...)
	})
	incidentManager.SetOnIncidentResolved(func(incident monitoring.Incident) {
		channelIDs := lookupCheckChannelIDs(store, incident.CheckID)
		notificationDispatcher.NotifyResolved(incident, nil, channelIDs...)
	})

	stopRetention := make(chan struct{})
	retentionJob.RunDaily(stopRetention)
	defer close(stopRetention)

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slogLogger.Error("service stopped", "error", logging.RedactErr(err))
		os.Exit(1)
	}
}

func seedDefaultAlertRules(ctx context.Context, repo monitoring.AlertRuleRepository, rules []monitoring.AlertRule) error {
	seedCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	existing, err := repo.List(seedCtx)
	if err != nil {
		return err
	}
	existingIDs := make(map[string]struct{}, len(existing))
	for _, rule := range existing {
		existingIDs[rule.ID] = struct{}{}
	}
	for i := range rules {
		if _, ok := existingIDs[rules[i].ID]; ok {
			continue
		}
		if err := repo.Create(seedCtx, &rules[i]); err != nil {
			return fmt.Errorf("seed alert rule %q: %w", rules[i].ID, err)
		}
	}
	return nil
}

func hasMySQLChecks(checks []monitoring.CheckConfig) bool {
	for _, check := range checks {
		if check.Type == "mysql" {
			return true
		}
	}
	return false
}

func mysqlRulesOnly(rules []monitoring.AlertRule) []monitoring.AlertRule {
	out := make([]monitoring.AlertRule, 0, len(rules))
	for _, rule := range rules {
		if rule.RuleCode != "" {
			out = append(out, rule)
		}
	}
	return out
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
