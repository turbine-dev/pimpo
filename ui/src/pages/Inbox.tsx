import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, BellOff, ShieldQuestion, Sparkles } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'

export function Inbox() {
  const t = useT()
  const nav = useNavigate()
  const qc = useQueryClient()
  const ready = useQuery({ queryKey: ['explorations', 'ready'], queryFn: () => api.explorations('ready') })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const repair = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'repair'), onSuccess: (r) => r.exploration && nav(`/explorations/${r.exploration}`) })
  const run = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'run'), onSettled: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  const approvals = useQuery({ queryKey: ['approvals'], queryFn: api.approvals, refetchInterval: 10_000 })
  const answer = useMutation({ mutationFn: ({ id, a }: { id: string; a: 'once' | 'run' | 'always' | 'deny' }) => api.answer(id, a), onSettled: () => qc.invalidateQueries({ queryKey: ['approvals'] }) })
  const broken = (routines.data ?? []).filter((r) => r.state === 'broken')
  const items = (ready.data?.length ?? 0) + broken.length + (approvals.data?.length ?? 0)

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('inbox.title')}</h1>
      <p className="mb-6 text-sm text-ink-2">{t('inbox.subtitle')}</p>
      {items === 0 && (
        <EmptyState icon={<BellOff size={22} />} title={t('inbox.emptyTitle')}>
          {t('inbox.emptyText')}
        </EmptyState>
      )}
      <div className="space-y-3">
        {(approvals.data ?? []).map((ap) => (
          <Card key={ap.id} className={`flex flex-wrap items-center gap-4 p-4 ${ap.action.risk >= 3 ? 'border-danger/40' : 'border-change/40'}`}>
            <div className={`grid size-10 place-items-center rounded-xl ${ap.action.risk >= 3 ? 'bg-danger-soft text-danger' : 'bg-change-soft text-change'}`}>
              <ShieldQuestion size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{ap.text}</div>
              <div className="text-[12.5px] text-ink-3">{t('inbox.rule', { reason: ap.reason })}</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="ghost" onClick={() => answer.mutate({ id: ap.id, a: 'deny' })}>{t('inbox.deny')}</Button>
              <Button size="sm" onClick={() => answer.mutate({ id: ap.id, a: 'run' })} title={t('inbox.allRunHint')}>{t('inbox.allRun')}</Button>
              <Button size="sm" onClick={() => answer.mutate({ id: ap.id, a: 'always' })}>{t('inbox.always')}</Button>
              <Button size="sm" variant="primary" onClick={() => answer.mutate({ id: ap.id, a: 'once' })}>{t('inbox.allow')}</Button>
            </div>
          </Card>
        ))}
        {broken.map((r) => (
          <Card key={r.id} className="flex flex-wrap items-center gap-4 border-danger/40 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-danger-soft text-danger">
              <AlertTriangle size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{t('inbox.didntRun', { name: r.name })}</div>
              <div className="text-[12.5px] text-ink-3">{t('inbox.paused')}</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => run.mutate(r.id)}>{t('inbox.runAgain')}</Button>
              <Button size="sm" variant="primary" onClick={() => repair.mutate(r.id)}>{t('inbox.redo')}</Button>
            </div>
          </Card>
        ))}
        {(ready.data ?? []).map((e) => (
          <Card key={e.id} className="flex flex-wrap items-center gap-4 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-accent/12 text-accent">
              <Sparkles size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px] font-medium">{e.request}</div>
              <div className="text-[12.5px] text-ink-3">{t('routines.ready', { when: relative(e.updated_at) })}</div>
            </div>
            <Button size="sm" variant="primary" onClick={() => nav(`/explorations/${e.id}`)}>{t('inbox.review')}</Button>
          </Card>
        ))}
      </div>
    </div>
  )
}
