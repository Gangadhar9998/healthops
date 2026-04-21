import { useCallback, useState } from 'react'
import { useSSE } from './useSSE'
import type { SSEPayload, Summary, CheckResult } from '@/types'

export interface LiveSummaryState {
  summary: Summary | null
  activeIncidents: number
  lastUpdated: string | null
  connected: boolean
  /** Latest result per check, keyed by checkId */
  latestByCheck: Map<string, CheckResult>
}

export function useLiveSummary(enabled = true): LiveSummaryState {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [activeIncidents, setActiveIncidents] = useState(0)
  const [lastUpdated, setLastUpdated] = useState<string | null>(null)
  const [latestByCheck, setLatestByCheck] = useState<Map<string, CheckResult>>(new Map())

  const handleMessage = useCallback((payload: SSEPayload) => {
    setSummary(payload.summary)
    setActiveIncidents(payload.activeIncidents)
    setLastUpdated(payload.timestamp)

    if (payload.summary.latest) {
      const map = new Map<string, CheckResult>()
      for (const r of payload.summary.latest) {
        map.set(r.checkId, r)
      }
      setLatestByCheck(map)
    }
  }, [])

  const { connected } = useSSE(enabled ? handleMessage : () => {})

  return {
    summary: enabled ? summary : null,
    activeIncidents,
    lastUpdated,
    connected: enabled && connected,
    latestByCheck,
  }
}
