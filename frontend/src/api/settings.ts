import { api } from './client'
import type { SafeConfigView, AlertRule } from '@/types'

export const settingsApi = {
  config: () => api.get<SafeConfigView>('/config'),
  updateConfig: (data: Record<string, unknown>) => api.put<SafeConfigView>('/config', data),
  alertRules: () => api.get<AlertRule[]>('/alert-rules'),
  createAlertRule: (rule: Partial<AlertRule>) => api.post<AlertRule>('/alert-rules', rule),
  updateAlertRule: (id: string, rule: Partial<AlertRule>) => api.put<AlertRule>(`/alert-rules/${encodeURIComponent(id)}`, rule),
  deleteAlertRule: (id: string) => api.delete(`/alert-rules/${encodeURIComponent(id)}`),
  exportResults: (format = 'csv') => `/api/v1/export/results?format=${format}`,
  exportIncidents: (format = 'csv') => `/api/v1/export/incidents?format=${format}`,
  exportMysqlSamples: (format = 'csv') => `/api/v1/export/mysql/samples?format=${format}`,
  exportAuditLog: (format = 'csv') => `/api/v1/export/audit?format=${format}`,
}
