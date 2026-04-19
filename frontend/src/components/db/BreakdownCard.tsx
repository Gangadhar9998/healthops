import { cn } from '@/lib/utils'

interface BarItem {
  label: string
  value: number
  total: number
  maxValue: number
}

interface Props {
  title: string
  items: BarItem[]
  barColor?: (value: number) => string
  emptyMessage?: string
  mono?: boolean
}

/** Horizontal bar chart for breakdown data (users, hosts, etc). Reusable across any DB type. */
export function BreakdownCard({ title, items, barColor, emptyMessage = 'No data available', mono }: Props) {
  const defaultColor = (v: number) =>
    v > 20 ? 'bg-red-500' : v > 5 ? 'bg-amber-500' : 'bg-blue-500'

  const colorFn = barColor ?? defaultColor

  return (
    <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h2>
      </div>
      {items.length > 0 ? (
        <div className="p-4 space-y-3">
          {items.map((item) => (
            <div key={item.label}>
              <div className="flex items-center justify-between mb-1">
                <span className={cn('text-sm font-medium text-slate-700 dark:text-slate-300', mono && 'font-mono')}>{item.label}</span>
                <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  {item.value} <span className="text-xs text-slate-400 font-normal">active</span>
                </span>
              </div>
              <div className="flex items-center gap-2">
                <div className="flex-1 h-2 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
                  <div
                    className={cn('h-full rounded-full', colorFn(item.value))}
                    style={{ width: `${Math.min((item.value / Math.max(item.maxValue, 1)) * 100, 100)}%` }}
                  />
                </div>
                <span className="text-xs text-slate-400 w-20 text-right">{formatCompact(item.total)} total</span>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="p-6 text-center text-sm text-slate-400">{emptyMessage}</div>
      )}
    </div>
  )
}

function formatCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}
