import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface Props {
  label: string
  value: string | number
  subValue?: string
  icon?: ReactNode
  trend?: 'up' | 'down' | 'flat'
  className?: string
}

export function MetricCard({ label, value, subValue, icon, className }: Props) {
  return (
    <div className={cn(
      'rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900',
      className,
    )}>
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <p className="text-sm font-medium text-slate-500 dark:text-slate-400">{label}</p>
          <p className="text-2xl font-bold tracking-tight text-slate-900 dark:text-slate-100">
            {value}
          </p>
          {subValue && (
            <p className="text-xs text-slate-400 dark:text-slate-500">{subValue}</p>
          )}
        </div>
        {icon && (
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
            {icon}
          </div>
        )}
      </div>
    </div>
  )
}
