import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard, Activity, AlertTriangle, Database,
  BarChart3, Brain, Settings, ChevronsLeft, X, Heart,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const ICON_MAP = {
  LayoutDashboard, Activity, AlertTriangle, Database,
  BarChart3, Brain, Settings,
} as const

const NAV = [
  { label: 'Dashboard', path: '/', icon: 'LayoutDashboard' as const },
  { label: 'Checks', path: '/checks', icon: 'Activity' as const },
  { label: 'Incidents', path: '/incidents', icon: 'AlertTriangle' as const },
  { label: 'MySQL', path: '/mysql', icon: 'Database' as const },
  { label: 'Analytics', path: '/analytics', icon: 'BarChart3' as const },
  { label: 'AI Analysis', path: '/ai', icon: 'Brain' as const },
  { label: 'Settings', path: '/settings', icon: 'Settings' as const },
]

interface Props {
  collapsed: boolean
  mobileOpen: boolean
  onCollapse: () => void
  onMobileClose: () => void
}

export function Sidebar({ collapsed, mobileOpen, onCollapse, onMobileClose }: Props) {
  return (
    <aside
      className={cn(
        'fixed inset-y-0 left-0 z-50 flex flex-col border-r border-slate-200 bg-white transition-all duration-200 ease-out dark:border-slate-800 dark:bg-slate-900',
        'lg:relative lg:z-auto',
        collapsed ? 'lg:w-[68px]' : 'lg:w-60',
        mobileOpen ? 'w-60 translate-x-0' : '-translate-x-full lg:translate-x-0',
      )}
    >
      {/* Brand */}
      <div className="flex h-14 items-center gap-3 border-b border-slate-200 px-4 dark:border-slate-800">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-blue-600 text-white">
          <Heart className="h-4 w-4" />
        </div>
        {!collapsed && (
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            HealthOps
          </span>
        )}
        {/* Mobile close */}
        <button
          onClick={onMobileClose}
          className="ml-auto rounded-md p-1 text-slate-400 hover:text-slate-600 lg:hidden"
          aria-label="Close navigation"
        >
          <X className="h-5 w-5" />
        </button>
        {/* Desktop collapse */}
        <button
          onClick={onCollapse}
          className={cn(
            'ml-auto hidden rounded-md p-1 text-slate-400 transition-colors hover:text-slate-600 lg:block',
            collapsed && 'ml-0',
          )}
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          <ChevronsLeft className={cn('h-4 w-4 transition-transform', collapsed && 'rotate-180')} />
        </button>
      </div>

      {/* Nav */}
      <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-3">
        {NAV.map((item) => {
          const Icon = ICON_MAP[item.icon]
          return (
            <NavLink
              key={item.path}
              to={item.path}
              end={item.path === '/'}
              onClick={onMobileClose}
              className={({ isActive }) =>
                cn(
                  'group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300'
                    : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-200',
                  collapsed && 'justify-center px-2',
                )
              }
            >
              <Icon className="h-[18px] w-[18px] shrink-0" />
              {!collapsed && <span>{item.label}</span>}
            </NavLink>
          )
        })}
      </nav>

      {/* Footer */}
      {!collapsed && (
        <div className="border-t border-slate-200 px-4 py-3 dark:border-slate-800">
          <p className="text-xs text-slate-400">HealthOps v0.1.0</p>
        </div>
      )}
    </aside>
  )
}
