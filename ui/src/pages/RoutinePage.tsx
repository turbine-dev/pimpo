import * as Tabs from '@radix-ui/react-tabs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Eraser, Pause, Play, RotateCcw, Wrench } from 'lucide-react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Code } from '../components/Code'
import { capRisk, capabilityLabel } from '../components/RoutineCard'
import { Button, Card, PageSkeleton, RiskBadge, RunDots } from '../components/ui'
import { Diff } from '../components/Diff'
import { MakeWidget } from '../components/MakeWidget'
import { Publish } from '../components/Publish'
import { RoutineSettings } from '../components/RoutineSettings'
import { WebhookCard } from '../components/WebhookCard'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { cronText, relative, usd, when } from '../lib/format'
import { fill, useT } from '../lib/i18n'

export function RoutinePage() {
  const t = useT()
  const { id = '' } = useParams()
  const [params] = useSearchParams()
  const qc = useQueryClient()
  const nav = useNavigate()
  const q = useQuery({ queryKey: ['routine', id], queryFn: () => api.routine(id) })
  const act = useMutation({
    mutationFn: (a: 'run' | 'pause' | 'resume' | 'repair' | 'forget') => api.routineAction(id, a),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['routine', id] })
      qc.invalidateQueries({ queryKey: ['routines'] })
      if (res.exploration) nav(`/explorations/${res.exploration}`)
    },
  })
  if (!q.data) return q.error ? <div className="mx-auto max-w-5xl text-sm text-danger">{q.error.message}</div> : <PageSkeleton />
  const { summary: s, routine: r, versions, runs, state = {}, used_by: usedBy = [] } = q.data
  const lastError = runs.find((x) => x.outcome === 'failed')?.error
  const kept = Object.keys(state).length > 0
  const remembers = kept || /\bstate\.(get|set)\(/.test(r.code)

  return (
    <div className="mx-auto max-w-5xl">
      <Link to="/routines" className="mb-5 inline-flex items-center gap-1.5 text-[13px] text-ink-3 hover:text-ink">
        <ArrowLeft size={14} /> {t('nav.routines')}
      </Link>
      <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-[22px] font-semibold tracking-tight">{r.name}</h1>
          <p className="mt-1 text-sm text-ink-2">{r.description}</p>
          <div className="mt-3 flex flex-wrap items-center gap-3 text-[12.5px] text-ink-3">
            <span>{cronText(s.schedule)}</span>
            {s.state === 'active' && s.next_run && <span>· {t('routine.next', { when: when(s.next_run) })}</span>}
            <span>· {t('routine.version', { n: s.version })}</span>
            <span>· {usd(s.cost_month_usd)} {t('routine.thisMonth')}</span>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => act.mutate('run')} disabled={act.isPending}>
            <Play size={15} /> {t('routine.runNow')}
          </Button>
          {s.state === 'active' ? (
            <Button variant="ghost" onClick={() => act.mutate('pause')}>
              <Pause size={15} /> {t('routine.pause')}
            </Button>
          ) : (
            <Button variant="ghost" onClick={() => act.mutate('resume')}>
              <RotateCcw size={15} /> {t('routine.resume')}
            </Button>
          )}
          <MakeWidget id={s.id} shows={s.capabilities.includes('widget.show')} />
          <Publish id={s.id} name={s.name} />
        </div>
      </div>

      {s.state === 'broken' && (
        <Card className="mb-6 flex flex-wrap items-center justify-between gap-4 border-danger/40 bg-danger-soft/50 p-4">
          <div className="min-w-0">
            <div className="text-[14px] font-medium text-danger">{t('routine.failed')}</div>
            {lastError && <div className="mt-1 truncate text-[12.5px] text-ink-2">{lastError}</div>}
          </div>
          <Button variant="primary" onClick={() => act.mutate('repair')}>
            <Wrench size={15} /> {t('inbox.redo')}
          </Button>
        </Card>
      )}
      {act.data?.error && <p className="mb-4 text-sm text-danger">{act.data.error}</p>}

      <RoutineSettings s={s} onRedo={() => act.mutate('repair')} />
      <WebhookCard id={s.id} />

      <Tabs.Root defaultValue={params.get('tab') ?? 'overview'}>
        <Tabs.List className="mb-5 flex gap-1 border-b border-line" aria-label={t('routine.details')}>
          {[
            ['overview', t('routine.tab.runs')],
            ['code', t('routine.tab.code')],
            ['tests', t('routine.tab.tests', { n: r.tests.length })],
            ['caps', t('routine.tab.caps')],
            ...(remembers ? [['memory', t('routine.tab.memory')]] : []),
            ['history', t('routine.tab.history', { n: versions.length })],
          ].map(([v, l]) => (
            <Tabs.Trigger key={v} value={v} className="-mb-px border-b-2 border-transparent px-3 py-2.5 text-[13.5px] text-ink-3 hover:text-ink data-[state=active]:border-ink data-[state=active]:font-medium data-[state=active]:text-ink">
              {l}
            </Tabs.Trigger>
          ))}
        </Tabs.List>

        <Tabs.Content value="overview">
          {s.runs.length > 0 && <Card className="mb-4 flex items-center justify-between p-5">
            <RunDots runs={s.runs.map((o) => (o === 'ok' ? 'ok' : 'failed')) as ('ok' | 'failed')[]} />
            <span className="text-[13px] text-ink-3">
              {t('routine.recentOk', { ok: s.runs.filter((o) => o === 'ok').length, total: s.runs.length })}
            </span>
          </Card>}
          <Card className="divide-y divide-line">
            {runs.length === 0 && <div className="px-5 py-6 text-sm text-ink-3">{t('routine.notYet')} {s.next_run ? t('routine.firstRun', { when: when(s.next_run) }) : ''}</div>}
            {runs.map((run) => (
              <div key={run.id} className="flex items-center gap-3 px-5 py-3">
                <span className={cn('size-2 rounded-full', run.outcome === 'ok' ? 'bg-read' : run.outcome === 'failed' ? 'bg-danger' : 'animate-pulse-soft bg-explore')} />
                <span className="w-40 shrink-0 text-[13px]">{when(run.started_at)}</span>
                <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2">{run.error || t('routine.calls', { count: run.calls })}</span>
                <span className="text-[12px] tabular-nums text-ink-3">{usd(run.cost_usd)}</span>
              </div>
            ))}
          </Card>
        </Tabs.Content>

        <Tabs.Content value="code">
          <p className="mb-3 text-[13px] text-ink-2">
            {fill(t('routine.codeText'), { caps: <span className="rounded bg-change-soft px-1 text-change">{t('routine.capabilities')}</span> })}
          </p>
          <Code code={r.code} />
        </Tabs.Content>

        <Tabs.Content value="tests" className="space-y-3">
          {r.tests.length === 0 && <p className="text-sm text-ink-3">{t('routine.noTests')}</p>}
          {r.tests.map((test) => (
            <Card key={test.name} className="p-4">
              <div className="mb-2 text-[14px] font-medium">{test.name}</div>
              <ul className="space-y-1 text-[13px] text-ink-2">
                {test.expect.map((e, i) => (
                  <li key={i}>
                    {capabilityLabel(e.capability)}
                    {e.count !== undefined && ` · ${e.count}×`}
                    {e.contains?.length ? ` · ${t('routine.mentions', { list: e.contains.map((c) => `“${c}”`).join(', ') })}` : ''}
                    {e.not_contains?.length ? ` · ${t('routine.never', { list: e.not_contains.map((c) => `“${c}”`).join(', ') })}` : ''}
                  </li>
                ))}
              </ul>
            </Card>
          ))}
        </Tabs.Content>

        <Tabs.Content value="caps" className="space-y-2">
          {r.manifest.capabilities.map((c) => {
            const risk = capRisk[c.split(':')[0]] ?? 'read'
            return (
              <Card key={c} className="flex items-center justify-between gap-4 p-4">
                <div>
                  <div className="text-[14px] font-medium">{capabilityLabel(c)}</div>
                  <div className="text-[12.5px] text-ink-3">{t(`routine.risk.${risk}`)}</div>
                </div>
                <RiskBadge risk={risk} />
              </Card>
            )
          })}
          {Object.entries(r.manifest.judgments ?? {}).map(([name, q]) => (
            <Card key={name} className="p-4">
              <div className="text-[14px] font-medium">{t('routine.judgment', { name })}</div>
              <div className="text-[12.5px] text-ink-3">{t('routine.judgmentText', { question: q })}</div>
            </Card>
          ))}
          {(r.manifest.uses?.length ?? 0) > 0 && (
            <Card className="p-4">
              <div className="text-[14px] font-medium">{t('routine.uses')}</div>
              <div className="mt-1 flex flex-wrap gap-2 text-[13px]">
                {r.manifest.uses!.map((u) => <Link key={u} to={`/routines/${u}`} className="rounded-md bg-sunken px-2 py-0.5 hover:text-accent">{u}</Link>)}
              </div>
              <div className="mt-2 text-[12.5px] text-ink-3">{t('routine.usesNote')}</div>
            </Card>
          )}
          {usedBy.length > 0 && (
            <Card className="p-4">
              <div className="text-[14px] font-medium">{t('routine.usedBy')}</div>
              <div className="mt-1 flex flex-wrap gap-2 text-[13px]">
                {usedBy.map((u) => <Link key={u} to={`/routines/${u}`} className="rounded-md bg-sunken px-2 py-0.5 hover:text-accent">{u}</Link>)}
              </div>
            </Card>
          )}
          <p className="pt-2 text-[12.5px] text-ink-3">{t('routine.capsNote')}</p>
        </Tabs.Content>

        <Tabs.Content value="memory" className="space-y-3">
          <p className="text-[13px] text-ink-2">{t('routine.memoryText')}</p>
          {kept ? <Code code={JSON.stringify(state, null, 2)} /> : <p className="text-sm text-ink-3">{t('routine.memoryEmpty')}</p>}
          {kept && (
            <Button variant="ghost" onClick={() => window.confirm(t('routine.forgetConfirm')) && act.mutate('forget')} disabled={act.isPending}>
              <Eraser size={15} /> {t('routine.forget')}
            </Button>
          )}
        </Tabs.Content>

        <Tabs.Content value="history" className="space-y-3">
          {versions.map((v, i) => (
            <Card key={v.version} className="p-4">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <div className="text-[14px] font-medium">{t('routine.versionTitle', { n: v.version })}</div>
                  <div className="text-[12.5px] text-ink-3">{v.reason}</div>
                </div>
                <span className="text-[12px] text-ink-3">{relative(v.created_at)}</span>
              </div>
              {versions[i + 1] && versions[i + 1].routine.code !== v.routine.code && (
                <details className="mt-3">
                  <summary className="cursor-pointer text-[12.5px] text-ink-2">{t('routine.changed', { n: versions[i + 1].version })}</summary>
                  <div className="mt-2">
                    <Diff before={versions[i + 1].routine.code} after={v.routine.code} />
                  </div>
                </details>
              )}
            </Card>
          ))}
        </Tabs.Content>
      </Tabs.Root>
    </div>
  )
}
