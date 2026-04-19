import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { formatDistanceToNow, format } from 'date-fns'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function relativeTime(dateStr: string): string {
  return formatDistanceToNow(new Date(dateStr), { addSuffix: true })
}

export function formatDate(dateStr: string, fmt = 'MMM d, HH:mm'): string {
  return format(new Date(dateStr), fmt)
}

export function formatDuration(ms: number): string {
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60_000).toFixed(1)}m`
}

export function formatUptime(pct: number): string {
  if (pct >= 99.99) return '100%'
  if (pct >= 99.9) return `${pct.toFixed(2)}%`
  return `${pct.toFixed(1)}%`
}

export function statusColor(status: string): string {
  switch (status) {
    case 'healthy': return 'text-emerald-600 dark:text-emerald-400'
    case 'warning': return 'text-amber-600 dark:text-amber-400'
    case 'critical': return 'text-red-600 dark:text-red-400'
    default: return 'text-slate-500 dark:text-slate-400'
  }
}

export function statusBg(status: string): string {
  switch (status) {
    case 'healthy': return 'bg-emerald-50 dark:bg-emerald-950/40'
    case 'warning': return 'bg-amber-50 dark:bg-amber-950/40'
    case 'critical': return 'bg-red-50 dark:bg-red-950/40'
    default: return 'bg-slate-50 dark:bg-slate-800'
  }
}

export function severityColor(severity: string): string {
  switch (severity) {
    case 'critical': return 'text-red-600 dark:text-red-400'
    case 'warning': return 'text-amber-600 dark:text-amber-400'
    default: return 'text-slate-600 dark:text-slate-400'
  }
}

export function incidentStatusLabel(status: string): string {
  switch (status) {
    case 'open': return 'Open'
    case 'acknowledged': return 'Acknowledged'
    case 'resolved': return 'Resolved'
    default: return status
  }
}

export function checkTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    api: 'API',
    tcp: 'TCP',
    process: 'Process',
    command: 'Command',
    log: 'Log',
    mysql: 'MySQL',
  }
  return labels[type] ?? type
}

export function downloadFile(content: string, filename: string, mime = 'text/csv') {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
