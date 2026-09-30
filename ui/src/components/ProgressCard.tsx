import { useQuery } from '@tanstack/react-query'
import { Activity, Check, Loader2, RotateCcw, X } from 'lucide-react'
import { Link } from 'react-router-dom'
import { api, type Progress } from '../lib/api'
import { cn } from '../lib/cn'
import { relative, usd } from '../lib/format'
import { useT } from '../lib/i18n'
import { Card } from './ui'

// The person's progress records, kept by the server: the same after a
// reload or on the phone, and brought up to date by the live stream.
export function useProgress() {
  return useQuery({ queryKey: ['progress'], queryFn: api.progress })
}

export const running = (p: Progress) => p.state === 'running'

// ProgressCard shows where a job or a routine run is: its parts or steps,
// the step it is on, the cost so far and when it last moved.
export function ProgressCard({ p, link = true, className }: { p: Progress; link?: boolean; className?: string }) {
  const t = useT()
  const state = t(`progress.state.${p.phase ?? p.state}`)
  const count = p.total > 0 ? t('progress.parts', { done: p.done, total: p.total }) : p.done > 0 ? t('progress.steps', { count: p.done }) : ''
  const pct = p.total > 0 ? Math.round((p.done / p.total) * 100) : undefined
  const icon = p.state === 'running' ? <Loader2 size={15} className="animate-spin text-ink-3" aria-hidden />
    : p.state === 'done' ? <Check size={15} className="text-read" aria-hidden /> : <X size={15} className="text-danger" aria-hidden />
  const to = p.kind === 'job' ? `/jobs/${p.job}` : `/routines/${p.routine}`
  const title = link ? <Link to={to} className="hover:underline">{p.title}</Link> : p.title
  return (
    <Card className={cn('p-4', className)}>
      <div className="flex items-start gap-3">
        <span className="mt-0.5">{icon}</span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[14px] font-medium">{title}</div>
          <div className="mt-0.5 flex flex-wrap gap-x-2 text-[12px] text-ink-3" aria-live="polite">
            <span>{state}</span>
            {count && <span>· {count}</span>}
            {p.state === 'running' && !p.phase && p.label && <span className="truncate">· {p.label}</span>}
            <span>· {usd(p.cost_usd)}</span>
            <span>· {t('progress.updated', { when: relative(p.updated_at) })}</span>
          </div>
          {p.resumed && p.state === 'running' && <div className="mt-1 flex items-center gap-1 text-[12px] text-ink-2"><RotateCcw size={12} aria-hidden /> {t('progress.resumed')}</div>}
          {p.error && p.state === 'failed' && <div className="mt-1 truncate text-[12px] text-danger">{p.error}</div>}
        </div>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-sunken" role="progressbar" aria-label={t('progress.bar', { title: p.title })}
        aria-valuemin={0} aria-valuemax={pct === undefined ? undefined : 100} aria-valuenow={pct} aria-valuetext={[state, count].filter(Boolean).join(' · ')}>
        <div className={cn('h-full rounded-full', p.state === 'failed' ? 'bg-danger' : p.state === 'done' ? 'bg-read' : 'bg-ink', pct === undefined && p.state === 'running' && 'animate-pulse-soft')}
          style={{ width: `${pct ?? (p.state === 'running' ? 40 : 100)}%` }} />
      </div>
    </Card>
  )
}

// RunningNow lists what runs now for the person, optionally only some.
export function RunningNow({ only, title = true, className }: { only?: (p: Progress) => boolean; title?: boolean; className?: string }) {
  const t = useT()
  const q = useProgress()
  const list = (q.data ?? []).filter(running).filter((p) => !only || only(p))
  if (list.length === 0) return null
  return (
    <section className={cn('space-y-2', className)} aria-label={t('progress.runningNow')}>
      {title && <h2 className="flex items-center gap-2 text-[13px] font-medium text-ink-2"><Activity size={16} aria-hidden /> {t('progress.runningNow')}</h2>}
      {list.map((p) => <ProgressCard key={p.id} p={p} />)}
    </section>
  )
}
