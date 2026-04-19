import { Download, FileJson, FileSpreadsheet } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Props {
  onExportCSV?: () => void
  onExportJSON?: () => void
  downloadUrl?: string
  className?: string
}

export function ExportButton({ onExportCSV, onExportJSON, downloadUrl, className }: Props) {
  if (downloadUrl) {
    return (
      <a
        href={downloadUrl}
        download
        className={cn(
          'inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700',
          className,
        )}
      >
        <Download className="h-3.5 w-3.5" />
        Export
      </a>
    )
  }

  return (
    <div className={cn('flex items-center gap-1', className)}>
      {onExportCSV && (
        <button
          onClick={onExportCSV}
          className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
        >
          <FileSpreadsheet className="h-3.5 w-3.5" />
          CSV
        </button>
      )}
      {onExportJSON && (
        <button
          onClick={onExportJSON}
          className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
        >
          <FileJson className="h-3.5 w-3.5" />
          JSON
        </button>
      )}
    </div>
  )
}
