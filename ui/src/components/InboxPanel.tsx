import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Inbox, ShieldQuestion, Sparkles, Unplug } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button } from './ui'

type Tab = 'all' | 'approvals' | 'routines' | 'system'
const tabs: [Tab, TKey][] = [['all', 'inbox.tabAll'], ['approvals', 'inbox.tabApprovals'], ['routines', 'inbox.tabRoutines'], ['system', 'inbox.tabSystem']]

// InboxPanel opens from the bell: what waits for a decision, answered in
// place, with the full page one click away.
export function InboxPanel({ onClose }: { onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const [tab, setTab] = useState<Tab>('all')
  const approvals = useQuery({ queryKey: ['approvals'], queryFn: api.approvals, refetchInterval: 10_000 })
  const ready = useQuery({ queryKey: ['explorations', 'ready'], queryFn: () => api.explorations('ready') })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const system = useQuery({ queryKey: ['system'], queryFn: api.system })
  const answer = useMutation({ mutationFn: ({ id, a }: { id: string; a: 'once' | 'deny' }) => api.answer(id, a), onSettled: () => { qc.invalidateQueries({ queryKey: ['approvals'] }); qc.invalidateQueries({ queryKey: ['state'] }) } })
  const run = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'run'), onSettled: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  const broken = (routines.data ?? []).filter((r) => r.state === 'broken')
  const problems = (system.data?.components ?? []).filter((c) => c.state === 'error')
  const go = (to: string) => { onClose(); nav(to) }

  const rows: { key: string; kind: Tab; node: ReactNode }[] = [
    ...(approvals.data ?? []).map((ap) => ({ key: ap.id, kind: 'approvals' as Tab, node: (
      <Row icon={<ShieldQuestion size={15} />} tone={ap.action.risk >= 3 ? 'danger' : 'change'} title={ap.text} hint={relative(ap.created)}>
        <Button size="sm" variant="ghost" onClick={() => answer.mutate({ id: ap.id, a: 'deny' })}>{t('inbox.deny')}</Button>
        <Button size="sm" variant="primary" onClick={() => answer.mutate({ id: ap.id, a: 'once' })}>{t('inbox.allow')}</Button>
      </Row>
    ) })),
    ...broken.map((r) => ({ key: r.id, kind: 'routines' as Tab, node: (
      <Row icon={<AlertTriangle size={15} />} tone="danger" title={t('inbox.didntRun', { name: r.name })} hint={t('inbox.paused')}>
        <Button size="sm" onClick={() => run.mutate(r.id)}>{t('inbox.runAgain')}</Button>
      </Row>
    ) })),
    ...(ready.data ?? []).map((e) => ({ key: e.id, kind: 'routines' as Tab, node: (
      <Row icon={<Sparkles size={15} />} tone="explore" title={e.request} hint={t('inbox.readyHint')}>
        <Button size="sm" variant="primary" onClick={() => go(`/explorations/${e.id}`)}>{t('inbox.see')}</Button>
      </Row>
    ) })),
    ...problems.map((c) => ({ key: c.id, kind: 'system' as Tab, node: (
      <Row icon={<Unplug size={15} />} tone="danger" title={c.name} hint={c.detail} />
    ) })),
  ]
  const shown = rows.filter((r) => tab === 'all' || r.kind === tab)
  const count = (k: Tab) => rows.filter((r) => k === 'all' || r.kind === k).length

  return (
    <div role="dialog" aria-label={t('inbox.title')}
      className="absolute right-0 top-full z-40 mt-2 w-[min(400px,calc(100vw-24px))] rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] md:bottom-full md:left-0 md:right-auto md:top-auto md:mb-2 md:mt-0">
      <div className="flex items-center gap-2 px-4 pt-3.5">
        <Inbox size={16} className="text-ink-3" />
        <span className="flex-1 text-[14px] font-semibold">{t('inbox.title')}</span>
        <Link to="/inbox" onClick={onClose} className="text-[12px] text-ink-3 hover:text-ink">{t('inbox.openAll')}</Link>
      </div>
      <div role="tablist" className="mt-2 flex gap-1 overflow-x-auto border-b border-line px-3">
        {tabs.map(([k, label]) => (
          <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
            className={cn('-mb-px shrink-0 border-b-2 px-2 py-2 text-[12.5px]', tab === k ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-3 hover:text-ink')}>
            {t(label)}{count(k) > 0 && <span className="ml-1 text-ink-3 tabular-nums">{count(k)}</span>}
          </button>
        ))}
      </div>
      <div className="max-h-[60vh] overflow-y-auto p-2">
        {shown.length === 0 ? (
          <div className="grid place-items-center px-4 py-10 text-center">
            <Inbox size={22} className="mb-2 text-ink-3" />
            <div className="text-[13.5px] font-medium">{t('inbox.nothing')}</div>
            <div className="text-[12.5px] text-ink-3">{t('inbox.nothingText')}</div>
          </div>
        ) : shown.map((r) => <div key={r.key}>{r.node}</div>)}
      </div>
    </div>
  )
}

function Row({ icon, tone, title, hint, children }: { icon: ReactNode; tone: 'danger' | 'change' | 'explore'; title: string; hint?: string; children?: ReactNode }) {
  const tones = { danger: 'bg-danger-soft text-danger', change: 'bg-change-soft text-change', explore: 'bg-explore-soft text-explore' }
  return (
    <div className="flex items-start gap-3 rounded-xl px-2 py-2.5 hover:bg-sunken/50">
      <div className={cn('grid size-8 shrink-0 place-items-center rounded-lg', tones[tone])}>{icon}</div>
      <div className="min-w-0 flex-1">
        <div className="line-clamp-2 text-[13px] font-medium">{title}</div>
        {hint && <div className="truncate text-[11.5px] text-ink-3">{hint}</div>}
        {children && <div className="mt-2 flex flex-wrap gap-1.5">{children}</div>}
      </div>
    </div>
  )
}
