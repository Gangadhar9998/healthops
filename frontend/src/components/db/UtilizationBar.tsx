import { cn } from '@/lib/utils'

interface Props {
  label: string
  value: number
  max: number
  className?: string
}

/** Reusable utilization bar with color thresholds. Works for connections, CPU, memory, etc. */
export function UtilizationBar({ label, value, max, className }: Props) {
  const pct = max > 0 ? (value / max) * 100 : 0
  const color = pct > 80 ? 'text-red-600' : pct > 60 ? 'text-amber-600' : 'text-emerald-600'
  const barColor = pct > 80 ? 'bg-red-500' : pct > 60 ? 'bg-amber-500' : 'bg-emerald-500'

  return (
    <div className={cn('rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900', className)}>
      <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">{label}</h2>
      <div className="flex items-center gap-4">
        <div className="flex-1">
          <div className="h-4 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
            <div className={cn('h-full rounded-full transition-all', barColor)} style={{ width: `${Math.min(pct, 100)}%` }} />
          </div>
        </div>
        <span className={cn('text-lg font-bold', color)}>{pct.toFixed(1)}%</span>
      </div>
      <div className="mt-2 flex justify-between text-xs text-slate-400">
        <span>0</span>
        <span>{max} max</span>
      </div>
    </div>
  )
}
