export const API_BASE = '/api/v1'

export const REFETCH_INTERVAL = 30_000 // 30 seconds
export const SSE_RECONNECT_DELAY = 3_000

export const STATUS_ORDER = ['critical', 'warning', 'unknown', 'healthy'] as const

export const CHECK_TYPES = ['api', 'tcp', 'process', 'command', 'log', 'mysql'] as const

export const NAV_ITEMS = [
  { label: 'Dashboard', path: '/', icon: 'LayoutDashboard' },
  { label: 'Checks', path: '/checks', icon: 'Activity' },
  { label: 'Incidents', path: '/incidents', icon: 'AlertTriangle' },
  { label: 'MySQL', path: '/mysql', icon: 'Database' },
  { label: 'Analytics', path: '/analytics', icon: 'BarChart3' },
  { label: 'AI Analysis', path: '/ai', icon: 'Brain' },
  { label: 'Settings', path: '/settings', icon: 'Settings' },
] as const

export const CHART_COLORS = {
  healthy: '#059669',
  warning: '#d97706',
  critical: '#dc2626',
  unknown: '#64748b',
  primary: '#2563eb',
  primaryLight: '#93c5fd',
  p50: '#2563eb',
  p95: '#f59e0b',
  p99: '#ef4444',
} as const
