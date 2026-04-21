// ---------------------------------------------------------------------------
// TypeScript types matching the backend Go structs exactly
// ---------------------------------------------------------------------------

// --- Core ---

export interface CheckConfig {
  id: string
  name: string
  type: 'api' | 'tcp' | 'process' | 'command' | 'log' | 'mysql' | 'ssh'
  server?: string
  application?: string
  target?: string
  host?: string
  port?: number
  command?: string
  path?: string
  expectedStatus?: number
  expectedContains?: string
  timeoutSeconds?: number
  warningThresholdMs?: number
  freshnessSeconds?: number
  intervalSeconds?: number
  retryCount?: number
  retryDelaySeconds?: number
  cooldownSeconds?: number
  enabled?: boolean
  tags?: string[]
  metadata?: Record<string, string>
  mysql?: MySQLCheckConfig
  ssh?: SSHCheckConfig
  serverId?: string
  notificationChannelIds?: string[]
}

export interface SSHCheckConfig {
  host: string
  port?: number
  user: string
  keyPath?: string
  keyEnv?: string
  password?: string
  passwordEnv?: string
  metrics?: string[]
}

export interface MySQLCheckConfig {
  dsnEnv?: string
  host?: string
  port?: number
  username?: string
  password?: string
  database?: string
  connectTimeoutSeconds?: number
  queryTimeoutSeconds?: number
  processlistLimit?: number
  statementLimit?: number
  hostUserLimit?: number
}

export interface CheckResult {
  id: string
  checkId: string
  name: string
  type: string
  server?: string
  application?: string
  status: 'healthy' | 'warning' | 'critical' | 'unknown'
  healthy: boolean
  message?: string
  durationMs: number
  startedAt: string
  finishedAt: string
  metrics?: Record<string, number>
  tags?: string[]
}

export interface State {
  checks: CheckConfig[]
  results: CheckResult[]
  lastRunAt?: string
  updatedAt?: string
}

export interface StatusCount {
  total: number
  healthy: number
  warning: number
  critical: number
  unknown: number
}

export interface Summary {
  totalChecks: number
  enabledChecks: number
  healthy: number
  warning: number
  critical: number
  unknown: number
  lastRunAt?: string
  byServer: Record<string, StatusCount>
  byApplication: Record<string, StatusCount>
  latest: CheckResult[]
}

export interface DashboardSnapshot {
  state: State
  summary: Summary
  generatedAt: string
}

export interface RunSummary {
  startedAt: string
  finishedAt: string
  skipped?: boolean
  results: CheckResult[]
  summary: Summary
}

// --- Incidents ---

export interface Incident {
  id: string
  checkId: string
  checkName: string
  type: string
  status: 'open' | 'acknowledged' | 'resolved'
  severity: 'warning' | 'critical'
  message: string
  startedAt: string
  updatedAt: string
  resolvedAt?: string
  acknowledgedAt?: string
  acknowledgedBy?: string
  resolvedBy?: string
  metadata?: Record<string, string>
}

export interface IncidentSnapshot {
  incidentId: string
  snapshotType: string
  timestamp: string
  payloadJson: string
}

// --- Alert Rules ---

export type AlertOperator = 'equals' | 'not_equals' | 'greater_than' | 'less_than'

export interface AlertCondition {
  field: string
  operator: AlertOperator
  value: unknown
}

export interface AlertChannel {
  type: string
  config: Record<string, unknown>
}

export interface AlertRule {
  id: string
  name: string
  type?: string
  enabled: boolean
  checkIds: string[]
  conditions: AlertCondition[]
  severity: string
  channels: AlertChannel[]
  cooldownMinutes: number
  threshold?: number
  description?: string
  consecutiveBreaches?: number
  recoverySamples?: number
  thresholdNum?: number
  ruleCode?: string
}

// --- Notifications ---

export interface NotificationEvent {
  notificationId: string
  incidentId: string
  channel: string
  payloadJson: string
  status: 'pending' | 'sent' | 'failed'
  retryCount: number
  lastError?: string
  createdAt: string
  sentAt?: string
}

// --- AI ---

export type AIProviderType = 'openai' | 'anthropic' | 'google' | 'ollama' | 'custom'

export interface AIProviderConfig {
  id: string
  provider: AIProviderType
  name: string
  apiKey?: string
  apiKeyMasked?: string
  baseURL?: string
  model: string
  maxTokens?: number
  temperature?: number
  enabled: boolean
  isDefault?: boolean
  createdAt: string
  updatedAt: string
}

export interface AIPromptTemplate {
  id: string
  name: string
  description?: string
  systemMsg: string
  userMsg: string
  version: string
  isDefault?: boolean
}

export interface AIServiceConfig {
  enabled: boolean
  autoAnalyze: boolean
  maxConcurrent: number
  timeoutSeconds: number
  retryCount: number
  retryDelayMs: number
  activeProviderId?: string
  providers: AIProviderConfig[]
  defaultPromptId?: string
  prompts: AIPromptTemplate[]
}

export interface AIAnalysisResult {
  incidentId: string
  provider?: string
  model?: string
  analysis: string
  suggestions?: string[]
  severity?: string
  createdAt: string
}

export interface AIQueueItem {
  incidentId: string
  promptVersion: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  createdAt: string
  claimedAt?: string
  completedAt?: string
  lastError?: string
}

// --- Analytics ---

export interface UptimeStats {
  checkId: string
  checkName: string
  period: string
  totalResults: number
  healthyCount: number
  uptimePct: number
  avgDurationMs: number
  maxDurationMs: number
  minDurationMs: number
}

export interface ResponseTimeBucket {
  timestamp: string
  avgDurationMs: number
  p50DurationMs: number
  p95DurationMs: number
  p99DurationMs: number
  maxDurationMs: number
  minDurationMs: number
  count: number
}

export interface StatusTimelineEntry {
  timestamp: string
  checkId?: string
  checkName?: string
  status: string
  durationMs: number
  message?: string
}

export interface FailureRateEntry {
  timestamp?: string
  group: string
  totalResults: number
  failedCount: number
  failureRate: number
  rate?: number
}

export interface IncidentStats {
  total: number
  open: number
  acknowledged: number
  resolved: number
  mttaMinutes: number
  mttrMinutes: number
  bySeverity: Record<string, number>
}

export interface OverviewStats {
  totalChecks: number
  enabledChecks: number
  healthyChecks: number
  activeIncidents: number
  avgUptimePct: number
  checksByType: Record<string, number>
  checksByServer: Record<string, number>
}

export interface CheckDetail {
  config: CheckConfig
  latestResult?: CheckResult
  uptime24h: number
  uptime7d: number
  avgDurationMs: number
  recentResults: CheckResult[]
  openIncidents?: Incident[]
}

// --- Frontend API ---

export interface SafeConfigView {
  server: { addr: string; readTimeoutSeconds: number; writeTimeoutSeconds: number; idleTimeoutSeconds: number }
  authEnabled: boolean
  retentionDays: number
  checkIntervalSeconds: number
  workers: number
  allowCommandChecks: boolean
  totalChecks: number
  totalServers: number
}

// --- Remote Servers ---

export interface RemoteServer {
  id: string
  name: string
  host: string
  port: number
  user: string
  keyPath?: string
  keyEnv?: string
  password?: string
  passwordEnv?: string
  tags?: string[]
  enabled: boolean
}

export interface ServerTestResult {
  success: boolean
  output?: string
  error?: string
}

// --- Server Metrics ---

export interface ProcessInfo {
  pid: number
  user: string
  cpuPercent: number
  memPercent: number
  memMB: number
  command: string
}

export interface ServerSnapshot {
  serverId: string
  timestamp: string
  cpuPercent: number
  memoryTotalMB: number
  memoryUsedMB: number
  memoryPercent: number
  diskTotalGB: number
  diskUsedGB: number
  diskPercent: number
  loadAvg1: number
  loadAvg5: number
  loadAvg15: number
  uptimeHours: number
  topProcesses?: ProcessInfo[]
}

export interface MetricsPoint {
  timestamp: string
  cpuPercent: number
  memoryPercent: number
  memoryUsedMB: number
  diskPercent: number
  loadAvg1: number
}

// --- Users ---

export interface User {
  id: string
  username: string
  role: 'admin' | 'ops'
  displayName?: string
  email?: string
  createdAt: string
  updatedAt: string
}

export interface CreateUserRequest {
  username: string
  password: string
  role: string
  displayName?: string
  email?: string
}

export interface UpdateUserRequest {
  password?: string
  role?: string
  displayName?: string
  email?: string
}

export interface SSEPayload {
  type: string
  timestamp: string
  summary: Summary
  activeIncidents: number
}

// --- API Envelope ---

export interface APIResponse<T = unknown> {
  success: boolean
  data?: T
  error?: { code: number; message: string }
}

export interface PaginatedData<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

// --- MySQL ---

export interface MySQLProcess {
  id: number
  user: string
  host: string
  db: string
  command: string
  time: number
  state: string
  info: string
}

export interface MySQLUserStat {
  user: string
  currentConnections: number
  totalConnections: number
}

export interface MySQLHostStat {
  host: string
  currentConnections: number
  totalConnections: number
}

export interface MySQLDigestStat {
  digestText: string
  countStar: number
  sumTimerWait: number
  avgTimerWait: number
  sumRowsSent: number
  sumRowsExam: number
  sumErrors: number
  sumWarnings: number
  firstSeen: string
  lastSeen: string
}

export interface MySQLHealthCard {
  checkId: string
  status: string
  uptime: number
  connections: number
  maxConnections: number
  connectionUtilPct: number
  queriesPerSec: number
  slowQueries: number
  threadsRunning: number
  replicationLag?: number
  lastSampleAt?: string
  processList?: MySQLProcess[]
  userStats?: MySQLUserStat[]
  hostStats?: MySQLHostStat[]
  topQueries?: MySQLDigestStat[]
  totalSlowQueries: number
  abortedConnects: number
  abortedClients: number
  maxUsedConnections: number
  innodbRowLockWaits: number
  innodbRowLockTime: number
  questions: number
  // Performance & danger indicators
  selectScan: number
  selectFullJoin: number
  sortMergePasses: number
  tableLocksWaited: number
  tableLocksImmediate: number
  bufferPoolHitRate: number
  openFiles: number
  openFilesLimit: number
  openTables: number
  tableOpenCache: number
  openedTables: number
  connectionsRefused: number
}

export interface MySQLLiveSnapshot {
  timestamp: string
  connections: number
  maxConnections: number
  connectionUtilPct: number
  threadsRunning: number
  threadsConnected: number
  queriesPerSec: number
  slowQueries: number
  uptimeSeconds: number
  processList: MySQLProcess[]
  longRunning: MySQLProcess[]
  activeQueries: number
  longRunningCount: number
  // Extended real-time fields
  status: string
  abortedConnects: number
  abortedClients: number
  connectionsRefused: number
  maxUsedConnections: number
  innodbRowLockWaits: number
  tableLocksWaited: number
  bufferPoolHitRate: number
  userStats?: MySQLUserStat[]
  hostStats?: MySQLHostStat[]
}
