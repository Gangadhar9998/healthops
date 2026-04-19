import { Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import type { ReactNode } from 'react'

interface Props {
  backTo: string
  backLabel: string
  title: string
  subtitle?: string
  badge?: ReactNode
  actions?: ReactNode
  children: ReactNode
}

/** Reusable detail page layout for any database monitoring section. */
export function DetailPageLayout({ backTo, backLabel, title, subtitle, badge, actions, children }: Props) {
  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            to={backTo}
            className="rounded-md p-1 text-slate-400 transition-colors hover:text-slate-600 dark:hover:text-slate-300"
            title={backLabel}
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <div>
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{title}</h1>
            {subtitle && <p className="text-sm text-slate-500">{subtitle}</p>}
          </div>
          {badge}
        </div>
        {actions}
      </div>
      {children}
    </div>
  )
}
