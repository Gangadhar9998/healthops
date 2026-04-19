import { cn, statusColor } from '@/lib/utils'
import { CircleCheck, AlertTriangle, XCircle, HelpCircle } from 'lucide-react'

const ICONS = {
  healthy: CircleCheck,
  warning: AlertTriangle,
  critical: XCircle,
  unknown: HelpCircle,
}

interface Props {
  status: string
  size?: 'sm' | 'md'
  label?: boolean
}

export function StatusBadge({ status, size = 'sm', label = true }: Props) {
  const Icon = ICONS[status as keyof typeof ICONS] ?? HelpCircle
  const isSmall = size === 'sm'

  return (
    <span className={cn('inline-flex items-center gap-1 font-medium', statusColor(status), isSmall ? 'text-xs' : 'text-sm')}>
      <Icon className={isSmall ? 'h-3.5 w-3.5' : 'h-4 w-4'} />
      {label && <span className="capitalize">{status}</span>}
    </span>
  )
}
