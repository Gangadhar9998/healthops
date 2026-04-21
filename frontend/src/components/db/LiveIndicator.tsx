import { cn } from '@/lib/utils'

interface Props {
  connected: boolean
  className?: string
}

/** Pulsing dot that shows live connection status. */
export function LiveIndicator({ connected, className }: Props) {
  return (
    <span className={cn('inline-flex items-center gap-1.5 text-xs font-medium', className)}>
      <span className="relative flex h-2 w-2">
        {connected && (
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
        )}
        <span className={cn(
          'relative inline-flex h-2 w-2 rounded-full',
          connected ? 'bg-emerald-500' : 'bg-slate-400'
        )} />
      </span>
      <span className={connected ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-400'}>
        {connected ? 'Live' : 'Disconnected'}
      </span>
    </span>
  )
}
