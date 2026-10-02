import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Play, Plus, Square } from 'lucide-react'
import { useState } from 'react'
import { Field, Modal } from '../components/Modal'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type AgentRoutine, type Hours, type Org, type Work } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { slug, unique } from '../lib/org'
import { area, field } from './Companies'

const usd = (n: number) => `$${n.toFixed(2)}`
const days = [1, 2, 3, 4, 5, 6, 0]

export function HoursEditor({ value, onChange }: { value: Hours; onChange: (h: Hours) => void }) {
  const t = useT()
  const always = !value.days?.length
  const week = new Intl.DateTimeFormat(undefined, { weekday: 'short' })
  // 2026-01-04 is a Sunday.
  const dayName = (d: number) => week.format(new Date(2026, 0, 4 + d))
  return (
    <fieldset className="space-y-2">
      <legend className="mb-1 text-[12.5px] text-ink-2">{t('co.hours')}</legend>
      <label className="flex items-center gap-2 text-[13px]">
        <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={always} onChange={(e) => onChange(e.target.checked ? {} : { days: [1, 2, 3, 4, 5], from: '09:00', to: '18:00' })} />
        {t('co.alwaysOn')}
      </label>
      {!always && (<>
        <div className="flex flex-wrap gap-1.5">
          {days.map((d) => (
            <label key={d} className={cn('cursor-pointer rounded-lg border px-2.5 py-1 text-[12.5px]', value.days?.includes(d) ? 'border-ink bg-ink text-bg' : 'border-line')}>
              <input type="checkbox" className="sr-only" checked={!!value.days?.includes(d)}
                onChange={(e) => onChange({ ...value, days: e.target.checked ? [...(value.days ?? []), d] : (value.days ?? []).filter((x) => x !== d) })} />
              {dayName(d)}
            </label>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <input type="time" aria-label={t('co.from')} className={field + ' w-32'} value={value.from ?? '09:00'} onChange={(e) => onChange({ ...value, from: e.target.value })} />
          <span className="text-[13px] text-ink-3">–</span>
          <input type="time" aria-label={t('co.to')} className={field + ' w-32'} value={value.to ?? '18:00'} onChange={(e) => onChange({ ...value, to: e.target.value })} />
        </div>
      </>)}
    </fieldset>
  )
}

const stateCls: Record<Work['state'], string> = {
  queued: 'bg-sunken text-ink-2', running: 'bg-explore-soft text-explore', done: 'bg-read-soft text-read', failed: 'bg-danger-soft text-danger', stopped: 'bg-sunken text-ink-3',
}

export function WorkLog({ org, can }: { org: Org; can: boolean }) {
  const t = useT()
  const qc = useQueryClient()
  const work = useQuery({ queryKey: ['company-work', org.id], queryFn: () => api.companyWork(org.id), refetchInterval: (q) => (q.state.data?.some((w) => w.state === 'running') ? 3000 : false) })
  const stop = useMutation({ mutationFn: (w: Work) => api.stopWork(org.id, w.id), onSuccess: () => qc.invalidateQueries({ queryKey: ['company-work', org.id] }) })
  const name = (id: string) => org.members.find((m) => m.id === id)?.name ?? id
  const list = work.data ?? []
  if (work.isSuccess && list.length === 0) return <EmptyState icon={<Activity />} title={t('co.empty.work')}>{t('co.noWork')}</EmptyState>
  return (
    <Card className="divide-y divide-line">
      {list.map((w) => (
        <div key={w.id} className="flex items-start gap-2 px-4 py-3">
          <details className="min-w-0 flex-1">
            <summary className="flex cursor-pointer list-none items-center gap-3">
              <span className={cn('shrink-0 rounded-md px-1.5 py-0.5 text-[11.5px] font-medium', stateCls[w.state])}>{t(`co.work.${w.state}`)}</span>
              <span className="w-20 shrink-0 truncate text-[13px] font-medium sm:w-24">{name(w.member)}</span>
              <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2">{w.request}</span>
              <span className="text-[12px] tabular-nums text-ink-3">{usd(w.cost_usd)}</span>
            </summary>
            <div className="mt-2 space-y-1 text-[12.5px]">
              <p className="whitespace-pre-line text-ink-2">{w.request}</p>
              {w.summary && <p className="whitespace-pre-line">{w.summary}</p>}
              {w.error && <p className="text-danger">{w.error}</p>}
              <p className="text-ink-3">{t('co.workLimit', { max: usd(w.max_usd) })}</p>
            </div>
          </details>
          {can && (w.state === 'queued' || w.state === 'running') && (
            <Button size="sm" variant="ghost" aria-label={t('co.stopWork')} onClick={() => stop.mutate(w)}><Square size={13} /></Button>
          )}
        </div>
      ))}
    </Card>
  )
}

// presets are schedules most agent routines need, in cron.
const presets: [string, string][] = [['', 'co.sched.none'], ['0 * * * *', 'co.sched.hourly'], ['0 8 * * *', 'co.sched.daily'], ['0 8 * * 1-5', 'co.sched.weekdays'], ['0 9 * * 1', 'co.sched.weekly']]

function AgentRoutineDialog({ org, start, onClose, onSaved }: { org: Org; start: AgentRoutine; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [r, setR] = useState(start)
  const custom = !presets.some(([v]) => v === (r.schedule ?? ''))
  const save = useMutation({
    mutationFn: () => api.saveAgentRoutine(org.id, { ...r, id: r.id || unique(slug(r.name, 'rotina'), org.agent_routines.map((x) => x.id)) }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteAgentRoutine(org.id, r.id), onSuccess: (o) => { onSaved(o); onClose() } })
  return (
    <Modal title={start.id ? start.name : t('co.newAgentRoutine')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.routineName')}><input className={field} value={r.name} maxLength={80} onChange={(e) => setR({ ...r, name: e.target.value })} /></Field>
        <Field label={t('co.routineInstructions')}><textarea className={area + ' min-h-28'} value={r.instructions} maxLength={8000} placeholder={t('co.routineInstructionsHint')} onChange={(e) => setR({ ...r, instructions: e.target.value })} /></Field>
        <Field label={t('co.when')}>
          <select className={field} value={custom ? 'custom' : (r.schedule ?? '')} onChange={(e) => setR({ ...r, schedule: e.target.value === 'custom' ? '30 9 * * 1-5' : e.target.value || undefined })}>
            {presets.map(([v, l]) => <option key={v} value={v}>{t(l as 'co.sched.none')}</option>)}
            <option value="custom">{t('co.sched.custom')}</option>
          </select>
        </Field>
        {custom && <Field label={t('co.cron')}><input className={field + ' font-mono'} value={r.schedule ?? ''} onChange={(e) => setR({ ...r, schedule: e.target.value })} /></Field>}
        <Field label={t('co.maxUsd')}><input type="number" min={0.05} max={5} step={0.05} className={field + ' w-32'} value={r.max_usd ?? 0.5} onChange={(e) => setR({ ...r, max_usd: Number(e.target.value) })} /></Field>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!r.name.trim() || !r.instructions.trim() || save.isPending}>{t('common.save')}</Button>
          {start.id && <Button type="button" variant="ghost" className="text-danger hover:bg-danger-soft hover:text-danger" onClick={() => remove.mutate()}>{t('co.deleteRoutine')}</Button>}
        </div>
      </form>
    </Modal>
  )
}

// MemberWork is what a member does: work to give it now, its agent
// routines and the compiled routines it was given.
export function MemberWork({ org, member, onSaved }: { org: Org; member: string; onSaved: (o: Org) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [request, setRequest] = useState('')
  const [editing, setEditing] = useState<AgentRoutine | null>(null)
  const mine = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const refresh = () => { qc.invalidateQueries({ queryKey: ['company', org.id] }); qc.invalidateQueries({ queryKey: ['company-work', org.id] }) }
  const give = useMutation({ mutationFn: () => api.giveWork(org.id, member, request), onSuccess: () => { setRequest(''); refresh() } })
  const run = useMutation({ mutationFn: (id: string) => api.runAgentRoutine(org.id, id), onSuccess: refresh })
  const giveRoutine = useMutation({ mutationFn: (id: string) => api.giveRoutine(org.id, member, id), onSuccess: refresh })
  const takeRoutine = useMutation({ mutationFn: (id: string) => api.takeRoutine(org.id, member, id), onSuccess: refresh })
  const agents = org.agent_routines.filter((r) => r.member === member)
  const compiled = (org.routines ?? []).filter((r) => r.member === member)
  const free = (mine.data ?? []).filter((r) => !(org.routines ?? []).some((x) => x.id === r.id))
  const error = give.error ?? run.error ?? giveRoutine.error ?? takeRoutine.error
  return (
    <div className="space-y-4">
      <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); give.mutate() }}>
        <Field label={t('co.giveWork')}><textarea className={area} value={request} maxLength={8000} placeholder={t('co.giveWorkHint')} onChange={(e) => setRequest(e.target.value)} /></Field>
        <Button type="submit" size="sm" disabled={!request.trim() || give.isPending}><Play size={13} /> {t('co.giveWorkNow')}</Button>
      </form>
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-[13px] font-medium">{t('co.agentRoutines')}</span>
          <Button type="button" size="sm" variant="ghost" onClick={() => setEditing({ id: '', member, name: '', instructions: '', schedule: '0 8 * * 1-5', max_usd: 0.5 })}><Plus size={13} /> {t('co.newAgentRoutine')}</Button>
        </div>
        {agents.length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noAgentRoutines')}</p>}
        {agents.map((r) => (
          <div key={r.id} className="flex items-center gap-2 rounded-lg border border-line px-3 py-2">
            <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setEditing(r)}>
              <span className="block truncate text-[13px]">{r.name}</span>
              <span className="block truncate font-mono text-[11.5px] text-ink-3">{r.schedule || t('co.sched.none')}</span>
            </button>
            <Button type="button" size="sm" variant="ghost" aria-label={t('co.runNow', { name: r.name })} onClick={() => run.mutate(r.id)}><Play size={13} /></Button>
          </div>
        ))}
      </div>
      <div className="space-y-2">
        <span className="text-[13px] font-medium">{t('co.compiledRoutines')}</span>
        <p className="text-[12.5px] text-ink-3">{t('co.compiledHint')}</p>
        {compiled.map((r) => (
          <div key={r.id} className="flex items-center gap-2 rounded-lg border border-line px-3 py-2 text-[13px]">
            <span className="min-w-0 flex-1 truncate">{r.name}</span>
            <Button type="button" size="sm" variant="ghost" onClick={() => takeRoutine.mutate(r.id)}>{t('co.takeBack')}</Button>
          </div>
        ))}
        {free.length > 0 && (
          <select className={field} value="" aria-label={t('co.giveRoutine')} onChange={(e) => e.target.value && giveRoutine.mutate(e.target.value)}>
            <option value="">{t('co.giveRoutine')}</option>
            {free.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
          </select>
        )}
      </div>
      {error && <p className="text-[13px] text-danger">{error.message}</p>}
      {editing && <AgentRoutineDialog org={org} start={editing} onClose={() => setEditing(null)} onSaved={onSaved} />}
    </div>
  )
}
