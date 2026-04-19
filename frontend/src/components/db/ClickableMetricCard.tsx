import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface Props {
  to: string
  icon: ReactNode
  label: string
  value: string | number
  subValue?: string
  status?: 'healthy' | 'warning' | 'critical' | 'neutral'
  className?: string
}

/** Clickable metric card that navigates to a detail page. Reusable across any DB type. */
export function ClickableMetricCard({ to, icon, label, value, subValue, status = 'neutral', className }: Props) {
  return (
    <Link
      to={to}
      className={cn(
        'group relative rounded-xl border border-slate-200 bg-white p-5 transition-all hover:shadow-md hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700',
        status === 'healthy' && 'ring-1 ring-emerald-200 dark:ring-emerald-900',
        status === 'warning' && 'ring-1 ring-amber-200 dark:ring-amber-900',
        status === 'critical' && 'ring-1 ring-red-200 dark:ring-red-900',
        className,
      )}
    >
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <p className="text-sm font-medium text-slate-500 dark:text-slate-400">{label}</p>
          <p className="text-2xl font-bold tracking-tight text-slate-900 dark:text-slate-100">{value}</p>
          {subValue && <p className="text-xs text-slate-400 dark:text-slate-500">{subValue}</p>}
        </div>
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
          {icon}
        </div>
      </div>
      <ChevronRight className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-300 opacity-0 transition-opacity group-hover:opacity-100 dark:text-slate-600" />
    </Link>
  )
}
