import { useEffect, useRef, useCallback, useState } from 'react'
import type { MySQLLiveSnapshot } from '@/types'
import { API_BASE, SSE_RECONNECT_DELAY } from '@/lib/constants'

const MAX_HISTORY = 60 // Keep last 60 snapshots for sparklines (~3 min at 3s interval)

export function useMySQLLive(enabled = true, interval = 3) {
  const [snapshot, setSnapshot] = useState<MySQLLiveSnapshot | null>(null)
  const [history, setHistory] = useState<MySQLLiveSnapshot[]>([])
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const esRef = useRef<EventSource | null>(null)

  const connect = useCallback(() => {
    if (!enabled) return
    if (esRef.current) esRef.current.close()

    const token = localStorage.getItem('healthops_token')
    let url = `${API_BASE}/mysql/live?interval=${interval}`
    if (token) url += `&token=${encodeURIComponent(token)}`

    const es = new EventSource(url)
    esRef.current = es

    es.addEventListener('mysql_live', (event) => {
      try {
        const data = JSON.parse(event.data) as MySQLLiveSnapshot
        setSnapshot(data)
        setHistory(prev => {
          const next = [...prev, data]
          return next.length > MAX_HISTORY ? next.slice(-MAX_HISTORY) : next
        })
        setConnected(true)
        setError(null)
      } catch { /* ignore malformed events */ }
    })

    es.addEventListener('error', (event) => {
      try {
        const data = JSON.parse((event as MessageEvent).data)
        setError(data.error || 'Connection error')
      } catch {
        setError('Connection lost')
      }
    })

    es.onerror = () => {
      setConnected(false)
      es.close()
      setTimeout(connect, SSE_RECONNECT_DELAY)
    }
  }, [enabled, interval])

  useEffect(() => {
    if (enabled) {
      connect()
    }
    return () => {
      esRef.current?.close()
    }
  }, [connect, enabled])

  return { snapshot, history, connected, error }
}
