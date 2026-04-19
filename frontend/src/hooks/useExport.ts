import { useCallback } from 'react'
import { downloadFile } from '@/lib/utils'

export function useExport() {
  const exportUrl = useCallback((url: string, filename: string) => {
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
  }, [])

  const exportJSON = useCallback((data: unknown, filename: string) => {
    downloadFile(JSON.stringify(data, null, 2), filename, 'application/json')
  }, [])

  const exportCSV = useCallback((rows: Record<string, unknown>[], filename: string) => {
    if (rows.length === 0) return
    const headers = Object.keys(rows[0])
    const csvLines = [
      headers.join(','),
      ...rows.map(row =>
        headers.map(h => {
          const val = row[h]
          const str = val == null ? '' : String(val)
          return str.includes(',') || str.includes('"') || str.includes('\n')
            ? `"${str.replace(/"/g, '""')}"`
            : str
        }).join(',')
      ),
    ]
    downloadFile(csvLines.join('\n'), filename)
  }, [])

  return { exportUrl, exportJSON, exportCSV }
}
