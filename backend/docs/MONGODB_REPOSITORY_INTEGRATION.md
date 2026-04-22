# MongoDB Repository Integration Guide

## Overview

This guide provides step-by-step instructions for integrating MongoDB repositories into the HealthOps monitoring service. The integration follows a layered architecture with graceful degradation to file-based storage when MongoDB is unavailable.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        cmd/healthops/main.go                    │
│                      (Service Initialization)                   │
└─────────────────────────────────────────────────────────────────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
                ▼               ▼               ▼
┌──────────────────────┐ ┌─────────────┐ ┌──────────────────┐
│  Repository Layer    │ │  Service    │ │  Hybrid Store    │
│  (MongoDB/File)      │ │  Layer      │ │  (Main State)    │
├──────────────────────┤ └─────────────┘ └──────────────────┘
│ • ServerRepository   │
│ • UserRepository     │        ▲
│ • ChannelRepo        │        │
│ • AlertRuleRepo      │   Uses All
│ • AIConfigRepo       │   Repositories
└──────────────────────┘
                │
        ┌───────┴───────┐
        │               │
        ▼               ▼
┌──────────────┐ ┌──────────────┐
│   MongoDB    │ │  File Store  │
│  (Primary)   │ │  (Fallback)  │
└──────────────┘ └──────────────┘
```

## Current State (File-Based Storage)

The following components currently use file-based storage:

| Component | File Location | Interface |
|-----------|---------------|-----------|
| Users | `data/users.json` | `UserStore` (custom) |
| Notification Channels | `data/channels.json` | `NotificationChannelStore` (custom) |
| Alert Rules | `data/alert_rules.json` | `FileAlertRuleRepository` |
| AI Config | `data/ai_config.json` | `FileAIConfigStore` |
| Servers | `config/servers.json` | `ServerRepository` (already supports Mongo) |
| Main State | `data/state.json` | `HybridStore` (already supports Mongo) |

## Target State (MongoDB + File Fallback)

All repositories will use MongoDB as primary storage with automatic fallback to file-based storage:

| Component | MongoDB Collection | File Fallback |
|-----------|-------------------|---------------|
| Users | `healthops_users` | `data/users.json` |
| Notification Channels | `healthops_notification_channels` | `data/channels.json` |
| Alert Rules | `healthops_alert_rules` | `data/alert_rules.json` |
| AI Config | `healthops_ai_config` | `data/ai_config.json` |
| Servers | `healthops_servers` | `config/servers.json` |
| Main State | `healthops_state` | `data/state.json` |

## Integration Steps

### Step 1: Add MongoDB Initialization Helper

Add a helper function to initialize MongoDB client and repositories with proper error handling.

**File:** `cmd/healthops/main.go`

```go
// mongoSetup holds MongoDB client and configuration
type mongoSetup struct {
    client   *mongo.Client
    db       *mongo.Database
    URI      string
    DBName   string
    Prefix   string
    timeout  time.Duration
}

// initMongoDB initializes MongoDB connection and client
// Returns nil if MONGODB_URI is not set (file-only mode)
func initMongoDB(logger *log.Logger) *mongoSetup {
    mongoURI := os.Getenv("MONGODB_URI")
    if mongoURI == "" {
        logger.Printf("MongoDB not configured (MONGODB_URI not set) - using file-based storage")
        return nil
    }

    mongoDB := os.Getenv("MONGODB_DATABASE")
    if mongoDB == "" {
        mongoDB = "healthops"
    }

    mongoPrefix := envOrDefault("MONGODB_COLLECTION_PREFIX", "healthops")

    // Force IPv4 to avoid IPv6 socket issues on macOS
    mongoURI = strings.ReplaceAll(mongoURI, "localhost", "127.0.0.1")

    clientOpts := options.Client().
        ApplyURI(mongoURI).
        SetServerSelectionTimeout(10 * time.Second).
        SetConnectTimeout(10 * time.Second).
        SetMaxPoolSize(100)

    client, err := mongo.Connect(clientOpts)
    if err != nil {
        logger.Printf("WARNING: Failed to connect to MongoDB: %v", err)
        logger.Printf("Falling back to file-based storage")
        return nil
    }

    // Ping to verify connection
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := client.Ping(ctx, nil); err != nil {
        logger.Printf("WARNING: MongoDB ping failed: %v", err)
        logger.Printf("Falling back to file-based storage")
        _ = client.Disconnect(context.Background())
        return nil
    }

    logger.Printf("MongoDB connected: %s/%s", mongoDB, mongoPrefix)

    return &mongoSetup{
        client:  client,
        db:      client.Database(mongoDB),
        URI:     mongoURI,
        DBName:  mongoDB,
        Prefix:  mongoPrefix,
        timeout: 10 * time.Second,
    }
}

// closeMongoDB gracefully closes MongoDB connection
func closeMongoDB(ms *mongoSetup, logger *log.Logger) {
    if ms != nil && ms.client != nil {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := ms.client.Disconnect(ctx); err != nil {
            logger.Printf("WARNING: MongoDB disconnect error: %v", err)
        }
    }
}
```

### Step 2: Initialize User Repository

**File:** `cmd/healthops/main.go`

Replace the existing user store initialization:

```go
// OLD CODE (remove this):
// userStore, err := monitoring.NewUserStore(dataDir)
// if err != nil {
//     logger.Printf("Warning: Failed to init user store: %v", err)
// } else {
//     service.SetUserStore(userStore)
//     if userStore.IsUsingDefaultCredentials() {
//         logger.Printf("WARNING: User management using default credentials — change immediately in production")
//     } else {
//         logger.Printf("User management initialized")
//     }
// }

// NEW CODE (add this):
// Initialize user repository (MongoDB or file-based)
var userRepo repositories.UserRepository
if mongoSetup != nil {
    mongoUserRepo, err := repositories.NewMongoUserRepository(mongoSetup.client, mongoSetup.DBName, mongoSetup.Prefix)
    if err != nil {
        logger.Printf("WARNING: Failed to init MongoDB user repository: %v", err)
        logger.Printf("Falling back to file-based user store")
        // Fall back to file-based
        fileUserStore, err := monitoring.NewUserStore(dataDir)
        if err != nil {
            logger.Printf("ERROR: Failed to init file user store: %v", err)
        } else {
            userRepo = fileUserStore
        }
    } else {
        userRepo = mongoUserRepo
        // Seed default admin user if collection is empty
        seedCtx, seedCancel := context.WithTimeout(context.Background(), 5*time.Second)
        if err := mongoUserRepo.SeedDefaultAdmin(seedCtx); err != nil {
            logger.Printf("WARNING: Failed to seed default admin: %v", err)
        }
        seedCancel()
        logger.Printf("User repository initialized (MongoDB)")
    }
} else {
    fileUserStore, err := monitoring.NewUserStore(dataDir)
    if err != nil {
        logger.Printf("WARNING: Failed to init file user store: %v", err)
    } else {
        userRepo = fileUserStore
        logger.Printf("User repository initialized (file-based)")
    }
}

if userRepo != nil {
    // Convert UserRepository to UserStore interface expected by service
    // Note: You may need to add an adapter if interfaces differ
    service.SetUserStore(userRepo)
}
```

### Step 3: Initialize Notification Channel Repository

**File:** `cmd/healthops/main.go`

Replace the existing notification channel store initialization:

```go
// OLD CODE (remove this):
// channelStore, err := notify.NewNotificationChannelStore(dataDir)
// if err != nil {
//     logger.Printf("Warning: Failed to init notification channel store: %v", err)
// }

// NEW CODE (add this):
// Initialize notification channel repository (MongoDB or file-based)
var channelRepo repositories.NotificationChannelRepository
if mongoSetup != nil {
    mongoChannelRepo, err := repositories.NewMongoChannelRepository(
        mongoSetup.URI,
        mongoSetup.DBName,
        mongoSetup.Prefix,
        10, // timeout seconds
    )
    if err != nil {
        logger.Printf("WARNING: Failed to init MongoDB channel repository: %v", err)
        logger.Printf("Falling back to file-based channel store")
        fileChannelStore, err := notify.NewNotificationChannelStore(dataDir)
        if err != nil {
            logger.Printf("ERROR: Failed to init file channel store: %v", err)
        } else {
            channelRepo = fileChannelStore
        }
    } else {
        channelRepo = mongoChannelRepo
        logger.Printf("Notification channel repository initialized (MongoDB)")
    }
} else {
    fileChannelStore, err := notify.NewNotificationChannelStore(dataDir)
    if err != nil {
        logger.Printf("WARNING: Failed to init file channel store: %v", err)
    } else {
        channelRepo = fileChannelStore
        logger.Printf("Notification channel repository initialized (file-based)")
    }
}

var notificationDispatcher *notify.NotificationDispatcher
if channelRepo != nil {
    // Note: You may need to adapt NotificationChannelRepository to the interface expected by NotificationDispatcher
    notificationDispatcher = notify.NewNotificationDispatcher(channelRepo, outbox, logger)
    defer notificationDispatcher.Stop()

    addr := cfg.Server.Addr
    if addr == "" || addr == ":8080" {
        addr = "http://localhost:8080"
    }
    notificationDispatcher.SetDashboardURL(addr)
    notificationAPIHandler := notify.NewNotificationAPIHandler(channelRepo, notificationDispatcher, cfg)
    service.SetNotifyRoutes(notificationAPIHandler)
    logger.Printf("Notification channels initialized")
}
```

### Step 4: Initialize Alert Rule Repository

**File:** `cmd/healthops/main.go`

Add alert rule repository initialization:

```go
// Initialize alert rule repository (MongoDB or file-based)
var alertRuleRepo repositories.AlertRuleRepository
if mongoSetup != nil {
    mongoAlertRuleRepo, err := repositories.NewMongoAlertRuleRepository(
        mongoSetup.URI,
        mongoSetup.DBName,
        mongoSetup.Prefix,
    )
    if err != nil {
        logger.Printf("WARNING: Failed to init MongoDB alert rule repository: %v", err)
        logger.Printf("Falling back to file-based alert rule store")
        fileAlertRuleRepo, err := monitoring.NewFileAlertRuleRepository(dataDir)
        if err != nil {
            logger.Printf("ERROR: Failed to init file alert rule store: %v", err)
        } else {
            alertRuleRepo = fileAlertRuleRepo
        }
    } else {
        alertRuleRepo = mongoAlertRuleRepo
        logger.Printf("Alert rule repository initialized (MongoDB)")
    }
} else {
    fileAlertRuleRepo, err := monitoring.NewFileAlertRuleRepository(dataDir)
    if err != nil {
        logger.Printf("WARNING: Failed to init file alert rule store: %v", err)
    } else {
        alertRuleRepo = fileAlertRuleRepo
        logger.Printf("Alert rule repository initialized (file-based)")
    }
}

if alertRuleRepo != nil {
    service.SetAlertRuleRepo(alertRuleRepo)
}
```

### Step 5: Initialize AI Config Repository

**File:** `cmd/healthops/main.go`

Replace the existing AI config store initialization:

```go
// OLD CODE (remove this):
// aiConfigStore, err := ai.NewAIConfigStore(dataDir)
// if err != nil {
//     logger.Printf("Warning: Failed to init AI config store: %v", err)
// }

// NEW CODE (add this):
// Initialize AI config repository (MongoDB or file-based)
var aiConfigRepo repositories.AIConfigRepository
if mongoSetup != nil {
    mongoAIConfigRepo, err := repositories.NewMongoAIConfigRepository(repositories.MongoAIConfigRepositoryConfig{
        MongoURI:       mongoSetup.URI,
        DatabaseName:   mongoSetup.DBName,
        CollectionName: mongoSetup.Prefix + "_ai_config",
        DataDir:        dataDir,
        RetentionDays:  cfg.RetentionDays,
    })
    if err != nil {
        logger.Printf("WARNING: Failed to init MongoDB AI config repository: %v", err)
        logger.Printf("Falling back to file-based AI config store")
        fileAIConfigStore, err := ai.NewAIConfigStore(dataDir)
        if err != nil {
            logger.Printf("ERROR: Failed to init file AI config store: %v", err)
        } else {
            aiConfigRepo = fileAIConfigStore
        }
    } else {
        aiConfigRepo = mongoAIConfigRepo
        logger.Printf("AI config repository initialized (MongoDB)")
    }
} else {
    fileAIConfigStore, err := ai.NewAIConfigStore(dataDir)
    if err != nil {
        logger.Printf("WARNING: Failed to init file AI config store: %v", err)
    } else {
        aiConfigRepo = fileAIConfigStore
        logger.Printf("AI config repository initialized (file-based)")
    }
}

if aiConfigRepo != nil && aiQueue != nil {
    aiService := ai.NewAIService(aiConfigRepo, aiQueue, incidentRepo, snapshotRepo, store, logger)
    aiService.StartWorker()
    defer aiService.StopWorker()

    aiAPIHandler := ai.NewAIAPIHandler(aiService, aiConfigRepo, nil, cfg)
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
```

### Step 6: Update Service Shutdown

**File:** `cmd/healthops/main.go`

Add MongoDB cleanup to graceful shutdown:

```go
// Add this after the existing defer statements
defer closeMongoDB(mongoSetup, logger)
```

### Step 7: Add Health Check Monitoring

**File:** `cmd/healthops/main.go`

Add monitoring for MongoDB connectivity with incident creation:

```go
// Start MongoDB health monitor for repositories
if mongoSetup != nil {
    stopMongoHealthMonitor := make(chan struct{})
    go monitorMongoRepositories(mongoSetup, incidentManager, logger, stopMongoHealthMonitor)
    defer close(stopMongoHealthMonitor)
}

// Add this function after main()
func monitorMongoRepositories(ms *mongoSetup, im *monitoring.IncidentManager, logger *log.Logger, stop <-chan struct{}) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    const checkID = "internal-mongodb-repositories"
    const checkName = "MongoDB Repository Storage"
    wasDown := false

    // Check if down at startup
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := ms.client.Ping(ctx, nil); err != nil {
        wasDown = true
        _ = im.ProcessAlert(checkID, checkName, "internal", "critical",
            "MongoDB repositories unreachable — operating in file fallback mode",
            map[string]string{"component": "mongodb-repositories"},
        )
        logger.Printf("INCIDENT: MongoDB repositories unreachable at startup")
    }

    for {
        select {
        case <-stop:
            return
        case <-ticker.C:
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            err := ms.client.Ping(ctx, nil)
            cancel()

            isDown := err != nil
            if isDown && !wasDown {
                _ = im.ProcessAlert(checkID, checkName, "internal", "critical",
                    "MongoDB repositories unreachable — operating in file fallback mode: "+err.Error(),
                    map[string]string{"component": "mongodb-repositories"},
                )
                logger.Printf("INCIDENT: MongoDB repositories connectivity lost: %v", err)
            } else if !isDown && wasDown {
                _ = im.AutoResolveOnRecovery(checkID)
                logger.Printf("RESOLVED: MongoDB repositories connectivity restored")
            }
            wasDown = isDown
        }
    }
}
```

## Migration Path

### Phase 1: Prepare Infrastructure (1 day)

1. **Create database indexes** for optimal performance:
   ```javascript
   // Connect to MongoDB
   use healthops

   // Create indexes
   db.healthops_users.createIndex({username: 1}, {unique: true})
   db.healthops_notification_channels.createIndex({name: 1})
   db.healthops_notification_channels.createIndex({enabled: 1})
   db.healthops_alert_rules.createIndex({name: 1})
   db.healthops_alert_rules.createIndex({enabled: 1})
   db.healthops_ai_config.createIndex({provider: 1})
   db.healthops_ai_config.createIndex({enabled: 1})
   db.healthops_ai_config.createIndex({default: 1})
   ```

2. **Verify MongoDB connectivity**:
   ```bash
   # Test connection
   mongosh "mongodb://localhost:27017/healthops"

   # Check environment variable
   echo $MONGODB_URI
   ```

### Phase 2: Integration Development (2-3 days)

1. Implement Step 1-7 from Integration Steps above
2. Add unit tests for repository initialization
3. Add integration tests with test MongoDB instance

### Phase 3: Testing (2 days)

1. **Unit Tests**: Test each repository independently
2. **Integration Tests**: Test full service with MongoDB
3. **Failover Tests**: Test fallback to file-based storage
4. **Performance Tests**: Compare MongoDB vs file performance

### Phase 4: Staging Deployment (1 day)

1. Deploy to staging environment
2. Monitor logs for errors
3. Verify all repositories work correctly
4. Test failover scenario

### Phase 5: Production Deployment (1 day)

1. Deploy during low-traffic period
2. Monitor metrics and logs
3. Verify incident creation for MongoDB failures
4. Prepare rollback plan if needed

## Rollback Plan

If issues occur after deployment, follow these steps:

### Immediate Rollback (5 minutes)

1. **Disable MongoDB**:
   ```bash
   unset MONGODB_URI
   ```

2. **Restart service**:
   ```bash
   systemctl restart healthops
   # or
   sudo systemctl restart healthops
   ```

3. **Verify fallback to file-based storage**:
   ```bash
   # Check logs
   journalctl -u healthops -f | grep "Falling back to file-based"
   ```

### Data Recovery (if needed)

If MongoDB data is corrupted:

1. **Export from file-based storage**:
   ```bash
   # All data is already in files in data/ directory
   ls -la data/
   ```

2. **Clear MongoDB collections** (optional):
   ```javascript
   use healthops
   db.healthops_users.drop()
   db.healthops_notification_channels.drop()
   db.healthops_alert_rules.drop()
   db.healthops_ai_config.drop()
   ```

3. **Re-enable MongoDB** after fixing issues:
   ```bash
   export MONGODB_URI="mongodb://localhost:27017"
   systemctl restart healthops
   ```

## Testing Checklist

### Pre-Integration Testing

- [ ] MongoDB server is running and accessible
- [ ] `MONGODB_URI` environment variable is set
- [ ] Database `healthops` exists
- [ ] Required indexes are created
- [ ] File-based storage works as fallback
- [ ] Service starts without errors

### Post-Integration Testing

- [ ] All repositories initialize successfully
- [ ] MongoDB repositories are used when available
- [ ] File-based fallback works when MongoDB is down
- [ ] Incidents are created when MongoDB goes down
- [ ] Incidents are resolved when MongoDB recovers
- [ ] User CRUD operations work
- [ ] Notification channel CRUD operations work
- [ ] Alert rule CRUD operations work
- [ ] AI config CRUD operations work
- [ ] API keys are encrypted in MongoDB
- [ ] Data persists across service restarts
- [ ] No data loss during failover/failback

### Performance Testing

- [ ] Repository read operations < 50ms (p95)
- [ ] Repository write operations < 100ms (p95)
- [ ] MongoDB connection pool is utilized efficiently
- [ ] No memory leaks in repository layer
- [ ] Concurrent operations handle correctly

### Security Testing

- [ ] API keys are encrypted at rest
- [ ] Passwords are hashed (bcrypt)
- [ ] No sensitive data in logs
- [ ] MongoDB connection uses TLS (production)
- [ ] Authentication works correctly
- [ ] Authorization checks work correctly

## Verification Commands

### Check MongoDB Connectivity

```bash
# From application server
mongosh "$MONGODB_URI" --eval "db.adminCommand('ping')"

# Check collections
mongosh "$MONGODB_URI" --eval "db.getSiblingDB('healthops').getCollectionNames()"
```

### Check Repository Data

```bash
# Count documents in each collection
mongosh "$MONGODB_URI" --eval "
db = db.getSiblingDB('healthops');
print('Users:', db.healthops_users.countDocuments());
print('Channels:', db.healthops_notification_channels.countDocuments());
print('Alert Rules:', db.healthops_alert_rules.countDocuments());
print('AI Config:', db.healthops_ai_config.countDocuments());
"
```

### Check Service Logs

```bash
# For systemd service
journalctl -u healthops -f | grep -E "MongoDB|repository|fallback"

# For direct process
tail -f /var/log/healthops/healthops.log | grep -E "MongoDB|repository|fallback"
```

### Test Failover

```bash
# Stop MongoDB
sudo systemctl stop mongod

# Watch logs for fallback message
journalctl -u healthops -f | grep "Falling back"

# Start MongoDB
sudo systemctl start mongod

# Watch logs for recovery
journalctl -u healthops -f | grep "RESOLVED"
```

## Troubleshooting

### Issue: MongoDB connection fails

**Symptoms**: Logs show "Failed to connect to MongoDB"

**Solutions**:
1. Verify MongoDB is running: `systemctl status mongod`
2. Check `MONGODB_URI` is correct: `echo $MONGODB_URI`
3. Test connection: `mongosh "$MONGODB_URI"`
4. Check firewall rules
5. Verify MongoDB authentication credentials

### Issue: Repository initialization fails

**Symptoms**: Logs show "Failed to init MongoDB repository"

**Solutions**:
1. Check MongoDB logs: `journalctl -u mongod -n 50`
2. Verify database exists: `mongosh "$MONGODB_URI" --eval "db.getSiblingDB('healthops')"`
3. Check indexes are created
4. Verify collection prefix is correct
5. Check MongoDB permissions

### Issue: Data not persisting

**Symptoms**: Data created but not found after restart

**Solutions**:
1. Check write concern settings
2. Verify MongoDB journaling is enabled
3. Check disk space
4. Review MongoDB logs for write errors
5. Test write operations manually

### Issue: Slow repository operations

**Symptoms**: Repository operations take > 100ms

**Solutions**:
1. Check indexes are created
2. Review query execution plan
3. Check MongoDB connection pool size
4. Monitor MongoDB performance: `mongosh --eval "db.serverStatus().connections"`
5. Consider scaling MongoDB resources

## Monitoring and Alerts

### Key Metrics to Monitor

1. **MongoDB Connectivity**
   - Connection success rate
   - Connection latency
   - Connection pool utilization

2. **Repository Operations**
   - Read/write latency (p50, p95, p99)
   - Operation success rate
   - Fallback activation count

3. **Data Integrity**
   - Document count per collection
   - Storage size per collection
   - Index usage statistics

4. **Incidents**
   - MongoDB down incidents
   - Repository failover incidents
   - Data consistency alerts

### Recommended Grafana Dashboards

1. **MongoDB Health Dashboard**
   - Connection status
   - Replication lag
   - Operation metrics
   - Storage metrics

2. **Repository Performance Dashboard**
   - Read/write latency
   - Operation throughput
   - Error rate
   - Fallback activation

## Appendix: Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `MONGODB_URI` | MongoDB connection string | - | Yes (for Mongo) |
| `MONGODB_DATABASE` | Database name | `healthops` | No |
| `MONGODB_COLLECTION_PREFIX` | Collection name prefix | `healthops` | No |
| `DATA_DIR` | Data directory path | `data` | No |
| `STATE_PATH` | State file path | `data/state.json` | No |

## Appendix: Collection Schema Reference

### healthops_users

```javascript
{
  _id: "username",
  password: "bcrypt_hash",
  role: "admin|viewer",
  email: "user@example.com",
  enabled: true,
  createdAt: ISODate("2024-01-01T00:00:00Z"),
  updatedAt: ISODate("2024-01-01T00:00:00Z")
}
```

### healthops_notification_channels

```javascript
{
  _id: "ch-1234567890",
  name: "Email Alerts",
  type: "email|slack|discord|telegram|webhook|pagerduty",
  enabled: true,
  config: {
    // type-specific config
  },
  createdAt: ISODate("2024-01-01T00:00:00Z"),
  updatedAt: ISODate("2024-01-01T00:00:00Z")
}
```

### healthops_alert_rules

```javascript
{
  _id: "rule-123",
  name: "High CPU Alert",
  description: "Alert when CPU > 90%",
  enabled: true,
  checkIds: ["check-1", "check-2"],
  conditions: [
    {
      field: "cpu_usage",
      operator: "gt",
      value: 90
    }
  ],
  severity: "critical|warning|info",
  channels: [
    {
      type: "email",
      config: {/* */}
    }
  ],
  cooldownMinutes: 5,
  consecutiveBreaches: 3,
  recoverySamples: 2,
  thresholdNum: 90.0,
  ruleCode: "cpu_high"
}
```

### healthops_ai_config

```javascript
{
  _id: "provider-123",
  name: "OpenAI GPT-4",
  provider: "openai|anthropic|google|ollama|custom",
  baseUrl: "https://api.openai.com/v1",
  apiKey: "encrypted_api_key",
  model: "gpt-4",
  maxTokens: 4096,
  temperature: 0.7,
  enabled: true,
  default: true,
  metadata: {/* */},
  createdAt: ISODate("2024-01-01T00:00:00Z"),
  updatedAt: ISODate("2024-01-01T00:00:00Z")
}
```

## Summary

This integration guide provides a complete roadmap for migrating HealthOps from file-based storage to MongoDB with graceful fallback. The layered architecture ensures:

1. **Zero Downtime**: Service continues operating if MongoDB fails
2. **Data Safety**: Automatic fallback to file-based storage
3. **Operational Visibility**: Incidents created for MongoDB failures
4. **Easy Rollback**: Simple environment variable change to revert
5. **Performance**: MongoDB provides better performance for large datasets

Follow the integration steps in order, test thoroughly at each phase, and use the rollback plan if issues arise.
