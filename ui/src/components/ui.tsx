import { type ButtonHTMLAttributes, type HTMLAttributes, type ReactNode } from 'react'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'

export function Button({ variant = 'secondary', size = 'md', className, ...p }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: 'sm' | 'md' }) {
  return (
    <button
      className={cn(
        'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-[10px] font-medium transition-[background,transform,box-shadow] active:scale-[0.98] disabled:pointer-events-none disabled:opacity-50',
        size === 'sm' ? 'h-8 px-3 text-[13px]' : 'h-9 px-4 text-sm',
        variant === 'primary' && 'bg-ink text-bg shadow-[var(--shadow-card)] hover:opacity-90',
        variant === 'secondary' && 'border border-line bg-surface text-ink shadow-[var(--shadow-card)] hover:border-line-strong',
        variant === 'ghost' && 'text-ink-2 hover:bg-sunken hover:text-ink',
        variant === 'danger' && 'bg-danger text-white hover:opacity-90',
        className,
      )}
      {...p}
    />
  )
}

export function Card({ className, ...p }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('rounded-[var(--radius-card)] border border-line bg-surface shadow-[var(--shadow-card)]', className)} {...p} />
}

export type Risk = 'read' | 'notify' | 'reversible' | 'irreversible'

const riskStyle: Record<Risk, { cls: string; dot: string }> = {
  read: { cls: 'bg-read-soft text-read', dot: 'bg-read' },
  notify: { cls: 'bg-read-soft text-read', dot: 'bg-read' },
  reversible: { cls: 'bg-change-soft text-change', dot: 'bg-change' },
  irreversible: { cls: 'bg-danger-soft text-danger', dot: 'bg-danger' },
}

export function RiskBadge({ risk, children }: { risk: Risk; children?: ReactNode }) {
  const t = useT()
  const s = riskStyle[risk]
  return (
    <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11.5px] font-medium', s.cls)}>
      <span className={cn('size-1.5 rounded-full', s.dot)} aria-hidden />
      {children ?? t(`risk.${risk}`)}
    </span>
  )
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded-md border border-line bg-sunken px-1.5 py-0.5 font-mono text-[11px] text-ink-3">{children}</kbd>
}

export function EmptyState({ icon, title, children, action }: { icon: ReactNode; title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-[var(--radius-card)] border border-dashed border-line-strong px-8 py-16 text-center">
      <div className="mb-4 grid size-12 place-items-center rounded-2xl bg-sunken text-ink-2">{icon}</div>
      <h3 className="text-[15px] font-semibold">{title}</h3>
      <p className="mt-1.5 max-w-sm text-sm text-ink-2">{children}</p>
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}

/** The last runs of a routine as dots: green ran fine, red failed, grey did not run. */
export function RunDots({ runs }: { runs: ('ok' | 'failed' | 'skipped')[] }) {
  const t = useT()
  return (
    <div className="flex items-center gap-[3px]" role="img" aria-label={t('ui.runs', { ok: runs.filter((r) => r === 'ok').length, total: runs.length })}>
      {runs.map((r, i) => (
        <span key={i} className={cn('h-3.5 w-1.5 rounded-full', r === 'ok' && 'bg-read', r === 'failed' && 'bg-danger', r === 'skipped' && 'bg-line-strong')} />
      ))}
    </div>
  )
}

/** Skeleton stands in for content while it loads, keeping the layout still. */
export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse-soft rounded-lg bg-sunken', className)} aria-hidden />
}

export function PageSkeleton() {
  const t = useT()
  return (
    <div className="mx-auto max-w-5xl space-y-4" aria-busy="true" aria-label={t('ui.loading')}>
      <Skeleton className="h-7 w-64" />
      <Skeleton className="h-4 w-96 max-w-full" />
      <div className="grid gap-4 pt-4 sm:grid-cols-2 xl:grid-cols-3">
        <Skeleton className="h-44" />
        <Skeleton className="h-44" />
        <Skeleton className="h-44" />
      </div>
    </div>
  )
}
