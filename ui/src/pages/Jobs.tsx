import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Layers, Loader2, Play, Square, X } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, type LongJob, type JobPart } from '../lib/api'
import { useT } from '../lib/i18n'
import { capabilityLabel } from '../components/RoutineCard'
import { ProgressCard, useProgress } from '../components/ProgressCard'
import { Button, Card, Switch } from '../components/ui'
import { cn } from '../lib/cn'

const busy = (j?: LongJob) => j?.state === 'running' || j?.state === 'reporting'

// Jobs are large pieces of work split across agents that run in the
// background: the plan first, then progress, the budget and the report.
export function Jobs() {
  const t = useT()
  const qc = useQueryClient()
  const { id } = useParams()
  const list = useQuery({ queryKey: ['jobs'], queryFn: api.jobs, refetchInterval: (q) => (q.state.data?.some(busy) ? 3000 : false) })
  const [request, setRequest] = useState('')
  const [budget, setBudget] = useState('2')
  const create = useMutation({ mutationFn: () => api.createJob(request.trim(), Number(budget)), onSuccess: () => { setRequest(''); qc.invalidateQueries({ queryKey: ['jobs'] }) } })
  if (id) return <JobDetail id={id} />
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <h1 className="mb-1 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><Layers size={20} /> {t('job.title')}</h1>
        <p className="text-sm text-ink-2">{t('job.text')}</p>
      </div>
      <Card className="space-y-3 p-4">
        <textarea value={request} onChange={(e) => setRequest(e.target.value)} rows={3} placeholder={t('job.placeholder')} aria-label={t('job.placeholder')}
          className="block w-full resize-y rounded-[10px] border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent" />
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-2 text-[13px] text-ink-2">{t('job.budget')}
            <input type="number" min="0.1" max="20" step="0.5" value={budget} onChange={(e) => setBudget(e.target.value)} className="h-9 w-24 rounded-[10px] border border-line bg-bg px-2 text-sm" />
          </label>
          <div className="flex-1" />
          <Button onClick={() => create.mutate()} disabled={!request.trim() || !(Number(budget) > 0) || create.isPending}>
            {create.isPending ? <Loader2 size={14} className="animate-spin" /> : <Layers size={14} />} {t('job.plan')}
          </Button>
        </div>
        {create.error && <p className="text-[13px] text-danger">{create.error.message}</p>}
      </Card>
      {(list.data ?? []).map((j) => (
        <Link key={j.id} to={`/jobs/${j.id}`} className="block">
          <Card className="flex items-center gap-3 p-4 hover:border-line-strong">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px] font-medium">{j.request}</div>
              <div className="text-[12px] text-ink-3">{t(`job.state.${j.state}`)} · {t('job.partsDone', { done: j.parts.filter((p) => p.state === 'done').length, count: j.parts.length })} · ${j.spent_usd.toFixed(2)} / ${j.budget_usd.toFixed(2)}</div>
            </div>
            {busy(j) && <Loader2 size={15} className="animate-spin text-ink-3" />}
          </Card>
        </Link>
      ))}
    </div>
  )
}

function JobDetail({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['job', id], queryFn: () => api.job(id), refetchInterval: (q) => (busy(q.state.data) ? 2000 : false) })
  const done = () => { qc.invalidateQueries({ queryKey: ['job', id] }); qc.invalidateQueries({ queryKey: ['jobs'] }) }
  const [follow, setFollow] = useState(false)
  const start = useMutation({ mutationFn: () => api.startJob(id, follow), onSuccess: done })
  const stop = useMutation({ mutationFn: () => api.stopJob(id), onSuccess: done })
  const followJob = useMutation({ mutationFn: (on: boolean) => api.followJob(id, on), onSuccess: done })
  const progress = useProgress().data?.find((p) => p.id === `job:${id}`)
  const j = q.data
  if (!j) return <div className="mx-auto max-w-3xl"><Loader2 className="animate-spin" /></div>
  const pct = Math.min(100, Math.round((j.spent_usd / j.budget_usd) * 100))
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <Link to="/jobs" className="text-[13px] text-ink-3 underline">{t('job.all')}</Link>
      <h1 className="text-[20px] font-semibold tracking-tight">{j.request}</h1>
      <div className="flex flex-wrap items-center gap-3 text-[13px] text-ink-2">
        <span>{t(`job.state.${j.state}`)}</span>
        <span className="flex items-center gap-2">${j.spent_usd.toFixed(2)} / ${j.budget_usd.toFixed(2)}
          <span className="inline-block h-1.5 w-24 overflow-hidden rounded-full bg-sunken"><span className="block h-full bg-ink" style={{ width: `${pct}%` }} /></span>
        </span>
        <div className="flex-1" />
        {j.state === 'planned' && <Button size="sm" variant="primary" onClick={() => start.mutate()} disabled={start.isPending}><Play size={14} /> {t('job.start')}</Button>}
        {(j.state === 'planned' || j.state === 'running') && <Button size="sm" variant="ghost" onClick={() => stop.mutate()} disabled={stop.isPending}><Square size={13} /> {t('job.stop')}</Button>}
      </div>
      {j.state === 'planned' && <p className="text-[13px] text-ink-2">{t('job.review')}</p>}
      {progress && <ProgressCard p={progress} link={false} />}
      {(j.state === 'planned' || busy(j)) && (
        <div className="flex items-start gap-3">
          <Switch on={j.state === 'planned' ? follow : !!j.follow} onChange={(on) => (j.state === 'planned' ? setFollow(on) : followJob.mutate(on))} label={t('progress.follow')} disabled={followJob.isPending} />
          <div className="text-[13px]">
            <div>{t('progress.follow')}</div>
            <div className="text-[12px] text-ink-3">{t('progress.followHint')}</div>
          </div>
        </div>
      )}
      {j.error && <p className="text-[13px] text-danger">{j.error}</p>}
      <div className="space-y-2">
        {j.parts.map((p) => <Part key={p.id} p={p} />)}
      </div>
      {j.report && (
        <Card className="p-5">
          <h2 className="mb-2 text-[15px] font-medium">{t('job.report')}</h2>
          <div className="whitespace-pre-wrap text-[14px] leading-relaxed">{j.report}</div>
        </Card>
      )}
    </div>
  )
}

function Part({ p }: { p: JobPart }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const icon = p.state === 'running' ? <Loader2 size={14} className="animate-spin" /> : p.state === 'done' ? <Check size={14} className="text-read" /> : p.state === 'failed' ? <X size={14} className="text-danger" /> : <span className="block size-3.5 rounded-full border border-line-strong" />
  return (
    <Card className="p-3">
      <button type="button" className="flex w-full items-start gap-3 text-left" onClick={() => setOpen(!open)} aria-expanded={open}>
        <span className="mt-0.5">{icon}</span>
        <span className="min-w-0 flex-1">
          <span className="block text-[14px] font-medium">{p.title}</span>
          <span className="block text-[12px] text-ink-3">
            {p.capabilities.filter((c) => c !== 'job.none').map((c) => capabilityLabel(c)).join(' · ') || t('job.noTools')}
            {p.cost_usd > 0 && ` · $${p.cost_usd.toFixed(2)}`}
          </span>
        </span>
      </button>
      {open && (
        <div className={cn('mt-2 space-y-2 border-t border-line pt-2 text-[13px]')}>
          <p className="whitespace-pre-wrap text-ink-2">{p.instructions}</p>
          {p.summary && <p className="whitespace-pre-wrap">{p.summary}</p>}
          {p.error && <p className="text-danger">{p.error}</p>}
          {p.exploration && <Link to={`/explorations/${p.exploration}`} className="text-[12.5px] text-ink-3 underline">{t('chat.steps')}</Link>}
        </div>
      )}
    </Card>
  )
}
