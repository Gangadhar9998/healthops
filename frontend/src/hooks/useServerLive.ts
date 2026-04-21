import { useEffect, useRef, useCallback, useState } from 'react'
import { SSE_RECONNECT_DELAY } from '@/lib/constants'
import type { ServerSnapshot } from '@/types'

export interface ServerLiveState {
  snapshot: ServerSnapshot | null
  history: ServerSnapshot[]
  connected: boolean
}

const MAX_HISTORY = 60

export function useServerLive(serverId: string | undefined, enabled = true): ServerLiveState {
  const [snapshot, setSnapshot] = useState<ServerSnapshot | null>(null)
  const [history, setHistory] = useState<ServerSnapshot[]>([])
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  const connect = useCallback(() => {
    if (!serverId || !enabled) return
    if (esRef.current) esRef.current.close()

    const token = localStorage.getItem('healthops_token')
    const params = new URLSearchParams()
    params.set('interval', '5')
    if (token) params.set('token', token)
    const url = `/api/v1/servers/${encodeURIComponent(serverId)}/live?${params}`
    const es = new EventSource(url)
    esRef.current = es

    es.onopen = () => setConnected(true)

    es.addEventListener('snapshot', (event) => {
      try {
        const snap = JSON.parse((event as MessageEvent).data) as ServerSnapshot
        setSnapshot(snap)
        setHistory(prev => {
          const next = [...prev, snap]
          return next.length > MAX_HISTORY ? next.slice(-MAX_HISTORY) : next
        })
      } catch { /* ignore malformed */ }
    })

    es.onerror = () => {
      setConnected(false)
      es.close()
      setTimeout(connect, SSE_RECONNECT_DELAY)
    }
  }, [serverId, enabled])

  useEffect(() => {
    if (enabled && serverId) {
      connect()
    }
    return () => {
      esRef.current?.close()
      esRef.current = null
    }
  }, [connect, enabled, serverId])

  return { snapshot: enabled ? snapshot : null, history, connected: enabled && connected }
}
