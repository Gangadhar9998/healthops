import { cn } from '@/lib/utils'

interface StatItem {
  icon: React.ReactNode
  label: string
  value: string
  warn?: boolean
}

interface Props {
  title: string
  stats: StatItem[]
  id?: string
  highlighted?: boolean
}

/** Reusable server stats card with icon+label+value rows. Works for any DB type. */
export function ServerStatsCard({ title, stats, id, highlighted }: Props) {
  return (
    <div id={id} className={cn(
      'rounded-xl border bg-white p-5 dark:bg-slate-900 transition-all',
      highlighted
        ? 'border-blue-400 ring-2 ring-blue-400/50 shadow-lg shadow-blue-100 dark:border-blue-500 dark:ring-blue-500/30 dark:shadow-blue-900/30'
        : 'border-slate-200 dark:border-slate-800'
    )}>
      <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h2>
      <div className="space-y-3">
        {stats.map((s) => (
          <div key={s.label} className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm text-slate-500">{s.icon}<span>{s.label}</span></div>
            <span className={cn('text-sm font-semibold', s.warn ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100')}>{s.value}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
