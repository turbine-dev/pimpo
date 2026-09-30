import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlarmClock, ArrowDownToLine, Loader2, Plus, Repeat, Sparkles, X } from 'lucide-react'
import { motion } from 'motion/react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { RoutineCard } from '../components/RoutineCard'
import { RepoPanel } from '../components/RepoPanel'
import { RoutineApprovals } from '../components/RoutineApprovals'
import { RunHistory } from '../components/RunHistory'
import { SinceYesterday } from '../components/SinceYesterday'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Exploration } from '../lib/api'
import { cn } from '../lib/cn'
import { relative, when } from '../lib/format'
import { useT } from '../lib/i18n'

export function Routines({ onNew }: { onNew: () => void }) {
  const t = useT()
  const nav = useNavigate()
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const pending = useQuery({ queryKey: ['explorations', 'pending'], queryFn: () => api.explorations('running,ready,compiling') })
  const imported = useQuery({ queryKey: ['explorations', 'imported'], queryFn: () => api.explorations('imported') })
  const list = routines.data ?? []
  const open = pending.data ?? []
  const [tab, setTab] = useState<'routines' | 'runs' | 'repo'>('routines')

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">{t('routines.title')}</h1>
          <p className="mt-1 text-sm text-ink-2">{t('routines.subtitle')}</p>
        </div>
        <Button variant="primary" onClick={onNew}>
          <Plus size={16} /> {t('common.newTask')}
        </Button>
      </div>

      {(
        <div role="tablist" aria-label={t('routines.title')} className="mb-5 flex gap-1 border-b border-line">
          {(['routines', 'runs', 'repo'] as const).map((k) => (
            <button key={k} type="button" role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
              className={cn('-mb-px border-b-2 px-3 py-2 text-[13px] transition', tab === k ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-3 hover:text-ink')}>
              {t(k === 'routines' ? 'routines.tabRoutines' : k === 'runs' ? 'routines.tabRuns' : 'routines.tabRepo')}
            </button>
          ))}
        </div>
      )}

      {tab === 'runs' ? <RunHistory /> : tab === 'repo' ? <RepoPanel /> : <>
      {list.length > 0 && <SinceYesterday />}

      {open.length > 0 && (
        <div className="mb-6 grid gap-3 sm:grid-cols-2">
          {open.map((e) => (
            <ExplorationCard key={e.id} e={e} onOpen={() => nav(`/explorations/${e.id}`)} />
          ))}
        </div>
      )}

      <Reminders />

      <RoutineApprovals />

      {(imported.data?.length ?? 0) > 0 && <ImportedTasks items={imported.data!} />}

      {routines.isPending ? null : list.length === 0 ? (
        <EmptyState icon={<Repeat size={22} />} title={t('routines.emptyTitle')} action={<Button variant="primary" onClick={onNew}>{t('routines.emptyAction')}</Button>}>
          {t('routines.emptyText')}
        </EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {list.map((r) => (
            <RoutineCard key={r.id} r={r} onOpen={() => nav(`/routines/${r.id}`)} />
          ))}
        </div>
      )}
      </>}
    </div>
  )
}

// Reminders are the one-time messages waiting for their time.
function Reminders() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['reminders'], queryFn: api.reminders, refetchInterval: 30_000 })
  const cancel = useMutation({ mutationFn: api.cancelReminder, onSuccess: () => qc.invalidateQueries({ queryKey: ['reminders'] }) })
  const list = q.data ?? []
  if (list.length === 0) return null
  return (
    <Card className="mb-6 p-4">
      <div className="mb-2 flex items-center gap-2 text-[14px] font-medium"><AlarmClock size={15} /> {t('rem.title')}</div>
      <ul className="divide-y divide-line">
        {list.map((r) => (
          <li key={r.id} className="flex items-center gap-3 py-2 text-[13.5px]">
            <span className="w-36 shrink-0 text-[12.5px] tabular-nums text-ink-3">{when(r.at)}</span>
            <span className="min-w-0 flex-1">{r.text}</span>
            <button type="button" aria-label={t('rem.cancel', { text: r.text })} onClick={() => cancel.mutate(r.id)}
              className="grid size-7 place-items-center rounded-lg text-ink-3 hover:bg-sunken hover:text-ink"><X size={14} /></button>
          </li>
        ))}
      </ul>
    </Card>
  )
}

function ExplorationCard({ e, onOpen }: { e: Exploration; onOpen: () => void }) {
  const t = useT()
  const running = e.state === 'running' || e.state === 'compiling'
  return (
    <motion.div initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}>
      <Card role="button" tabIndex={0} onClick={onOpen} onKeyDown={(k) => k.key === 'Enter' && onOpen()} className="flex cursor-pointer items-center gap-4 border-dashed border-explore/50 p-4 hover:border-explore">
        <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore">{running ? <Loader2 size={18} className="animate-spin" /> : <Sparkles size={18} />}</div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[14px] font-medium">{e.request}</div>
          <div className="text-[12.5px] text-ink-3">{e.state === 'running' ? t('routines.running') : e.state === 'compiling' ? t('routines.compiling') : t('routines.ready', { when: relative(e.updated_at) })}</div>
        </div>
      </Card>
    </motion.div>
  )
}

function ImportedTasks({ items }: { items: Exploration[] }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const explore = useMutation({ mutationFn: api.exploreImported, onSuccess: (r) => { qc.invalidateQueries({ queryKey: ['explorations'] }); nav(`/explorations/${r.id}`) } })
  const discard = useMutation({ mutationFn: api.discard, onSuccess: () => qc.invalidateQueries({ queryKey: ['explorations'] }) })
  return (
    <section className="mb-6" aria-label={t('routines.imported')}>
      <h2 className="mb-2 flex items-center gap-2 text-[13px] font-medium text-ink-2"><ArrowDownToLine size={14} /> {t('routines.importedTitle')}</h2>
      <Card className="divide-y divide-line">
        {items.map((e) => (
          <div key={e.id} className="flex items-center gap-3 p-3.5">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px]">{e.request.split('\n')[0]}</div>
              <div className="truncate text-[12px] text-ink-3">{e.summary}</div>
            </div>
            <Button size="sm" variant="primary" disabled={explore.isPending} onClick={() => explore.mutate(e.id)}>{t('routines.explore')}</Button>
            <Button size="sm" variant="ghost" aria-label={t('routines.discard')} onClick={() => discard.mutate(e.id)}><X size={15} /></Button>
          </div>
        ))}
      </Card>
    </section>
  )
}
