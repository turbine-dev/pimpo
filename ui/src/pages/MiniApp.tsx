import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleHelp, LayoutGrid, Pause, Play, ShieldQuestion, Wallet, Workflow } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { WidgetBody, WidgetCard } from '../components/widgets/Widget'
import { Button, Card, RunDots, Skeleton } from '../components/ui'
import { api, ApiError, setBearer, type Role, type WidgetView } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'

// The Telegram Mini App: a compact dashboard Telegram opens inside the chat
// with Pimpo's bot. Telegram hands the page initData, signed for this bot;
// the server checks it and answers with a short session of the person who
// paired that Telegram account. The session lives only in memory.

type TelegramWebApp = { initData: string; ready: () => void; expand: () => void; initDataUnsafe?: { user?: { language_code?: string } } }
type Session = { person: string; role: Role; name: string }

const SDK = 'https://telegram.org/js/telegram-web-app.js'

// telegram loads Telegram's script, allowed on this page only, and gives
// what it found; outside Telegram there is nothing.
export function loadTelegram(): Promise<TelegramWebApp | undefined> {
  const found = () => (window as { Telegram?: { WebApp?: TelegramWebApp } }).Telegram?.WebApp
  if (found()) return Promise.resolve(found())
  return new Promise((resolve) => {
    const s = document.createElement('script')
    s.src = SDK
    s.onload = () => resolve(found())
    s.onerror = () => resolve(undefined)
    document.head.appendChild(s)
  })
}

// The language Telegram says the person uses, for the page's locale.
export function telegramLanguage(): string | undefined {
  const hash = new URLSearchParams(location.hash.slice(1)).get('tgWebAppData')
  const user = hash ? new URLSearchParams(hash).get('user') : null
  try {
    return user ? (JSON.parse(user).language_code as string) : undefined
  } catch {
    return undefined
  }
}

type Tab = 'needs' | 'routines' | 'spending' | 'widgets'

export function MiniApp({ initData }: { initData?: string }) {
  const t = useT()
  const [session, setSession] = useState<Session>()
  const [problem, setProblem] = useState<string>()
  const [ended, setEnded] = useState(false)
  useEffect(() => {
    if (initData === undefined) return
    if (!initData) {
      setProblem(t('tg.notTelegram'))
      return
    }
    let gone = false
    api.miniAppSession(initData).then((s) => {
      if (gone) return
      setBearer(s.token)
      setSession(s)
    }, (e: unknown) => {
      if (gone) return
      setProblem(e instanceof ApiError && e.status === 403 ? t('tg.unpaired') : t('tg.failed', { error: e instanceof Error ? e.message : String(e) }))
    })
    return () => { gone = true }
  }, [initData, t])
  // When the session runs out every answer is 401: say so once.
  const onError = (e: unknown) => { if (e instanceof ApiError && e.status === 401) setEnded(true) }

  if (problem || ended) return <Screen>{ended ? t('tg.ended') : problem}</Screen>
  if (!session) return <Screen busy>{t('tg.signingIn')}</Screen>
  return <Board session={session} onError={onError} />
}

function Screen({ children, busy }: { children: ReactNode; busy?: boolean }) {
  return (
    <main className="grid min-h-dvh place-items-center px-6 text-center">
      <p role={busy ? 'status' : 'alert'} className="max-w-xs text-[14px] text-ink-2">{children}</p>
    </main>
  )
}

function Board({ session, onError }: { session: Session; onError: (e: unknown) => void }) {
  const t = useT()
  const guest = session.role === 'guest'
  const tabs: [Tab, TKey, ReactNode][] = [
    ['needs', 'tg.needs', <ShieldQuestion key="n" size={15} />],
    ...(guest ? [] : [['routines', 'tg.routines', <Workflow key="r" size={15} />] as [Tab, TKey, ReactNode]]),
    ['spending', 'tg.spending', <Wallet key="s" size={15} />],
    ['widgets', 'tg.widgets', <LayoutGrid key="w" size={15} />],
  ]
  const [tab, setTab] = useState<Tab>('needs')
  return (
    <main className="mx-auto min-h-dvh max-w-xl px-4 pb-8 pt-4">
      <h1 className="mb-3 text-[17px] font-semibold">{t('tg.hello', { name: session.name })}</h1>
      <div role="tablist" aria-label={t('tg.sections')} className="sticky top-0 z-10 -mx-4 mb-4 flex gap-1 overflow-x-auto border-b border-line bg-bg px-4">
        {tabs.map(([k, label, icon]) => (
          <button key={k} role="tab" id={`tab-${k}`} aria-controls={`panel-${k}`} aria-selected={tab === k} onClick={() => setTab(k)}
            className={cn('-mb-px flex shrink-0 items-center gap-1.5 border-b-2 px-2.5 py-2.5 text-[13px]', tab === k ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-3')}>
            {icon}{t(label)}
          </button>
        ))}
      </div>
      <section role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
        {tab === 'needs' && <Needs guest={guest} onError={onError} />}
        {tab === 'routines' && <RoutineList onError={onError} />}
        {tab === 'spending' && <Spending guest={guest} onError={onError} />}
        {tab === 'widgets' && <Widgets onError={onError} />}
      </section>
    </main>
  )
}

// watch passes a query's failure on, so a session that ran out is noticed.
function useWatch(error: unknown, onError: (e: unknown) => void) {
  useEffect(() => { if (error) onError(error) }, [error, onError])
}

function Needs({ guest, onError }: { guest: boolean; onError: (e: unknown) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const approvals = useQuery({ queryKey: ['approvals'], queryFn: api.approvals, enabled: !guest, refetchInterval: 15_000 })
  const questions = useQuery({ queryKey: ['questions'], queryFn: api.questions, refetchInterval: 15_000 })
  useWatch(approvals.error ?? questions.error, onError)
  const answer = useMutation({ mutationFn: ({ id, a }: { id: string; a: 'once' | 'deny' }) => api.answer(id, a), onError, onSettled: () => qc.invalidateQueries({ queryKey: ['approvals'] }) })
  const reply = useMutation({ mutationFn: ({ id, i }: { id: string; i: number }) => api.answerQuestion(id, i), onError, onSettled: () => qc.invalidateQueries({ queryKey: ['questions'] }) })
  if (questions.isLoading || approvals.isLoading) return <Loading />
  const none = (approvals.data ?? []).length === 0 && (questions.data ?? []).length === 0
  if (none) return <Empty>{t('tg.nothing')}</Empty>
  return (
    <ul className="space-y-3">
      {(questions.data ?? []).map((q) => (
        <li key={q.id}>
          <Card className="p-4">
            <div className="mb-3 flex gap-2 text-[14px] font-medium"><CircleHelp size={16} className="mt-0.5 shrink-0 text-accent" />{q.question}</div>
            <div className="flex flex-wrap gap-2">
              {q.options.map((o, i) => <Button key={i} size="sm" disabled={reply.isPending} onClick={() => reply.mutate({ id: q.id, i })}>{o}</Button>)}
            </div>
          </Card>
        </li>
      ))}
      {(approvals.data ?? []).map((ap) => (
        <li key={ap.id}>
          <Card className={cn('p-4', ap.action.risk >= 3 ? 'border-danger/40' : 'border-change/40')}>
            <div className="text-[14px] font-medium">{ap.text}</div>
            <div className="mb-3 text-[12.5px] text-ink-3">{t('inbox.rule', { reason: ap.reason })}</div>
            <div className="flex gap-2">
              <Button size="sm" variant="ghost" disabled={answer.isPending} onClick={() => answer.mutate({ id: ap.id, a: 'deny' })}>{t('inbox.deny')}</Button>
              <Button size="sm" variant="primary" disabled={answer.isPending} onClick={() => answer.mutate({ id: ap.id, a: 'once' })}>{t('inbox.allow')}</Button>
            </div>
          </Card>
        </li>
      ))}
    </ul>
  )
}

function RoutineList({ onError }: { onError: (e: unknown) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  useWatch(routines.error, onError)
  const act = useMutation({ mutationFn: ({ id, a }: { id: string; a: 'run' | 'pause' | 'resume' }) => api.routineAction(id, a), onError, onSettled: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  if (routines.isLoading) return <Loading />
  if (!routines.data?.length) return <Empty>{t('tg.noRoutines')}</Empty>
  return (
    <ul className="space-y-2">
      {routines.data.map((r) => (
        <li key={r.id}>
          <Card className="flex items-center gap-3 p-3">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px] font-medium">{r.name}</div>
              <div className="flex items-center gap-2 text-[12px] text-ink-3">
                <RunDots runs={r.runs} />
                {r.state === 'paused' && <span>{t('tg.paused')}</span>}
                {r.state === 'broken' && <span className="text-danger">{t('tg.broken')}</span>}
              </div>
            </div>
            <Button size="sm" aria-label={`${t('tg.run')}: ${r.name}`} disabled={act.isPending} onClick={() => act.mutate({ id: r.id, a: 'run' })}><Play size={14} /></Button>
            {r.state === 'paused'
              ? <Button size="sm" variant="ghost" disabled={act.isPending} onClick={() => act.mutate({ id: r.id, a: 'resume' })}>{t('tg.resume')}</Button>
              : <Button size="sm" variant="ghost" aria-label={`${t('tg.pause')}: ${r.name}`} disabled={act.isPending} onClick={() => act.mutate({ id: r.id, a: 'pause' })}><Pause size={14} /></Button>}
          </Card>
        </li>
      ))}
    </ul>
  )
}

function Spending({ guest, onError }: { guest: boolean; onError: (e: unknown) => void }) {
  const t = useT()
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const cost = useQuery({ queryKey: ['cost'], queryFn: api.cost, enabled: !guest })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines, enabled: !guest })
  useWatch(state.error ?? cost.error ?? routines.error, onError)
  if (state.isLoading) return <Loading />
  const b = state.data?.budget ?? { spent: 0, limit: 0 }
  const share = b.limit > 0 ? Math.min(1, b.spent / b.limit) : 0
  const mine = (routines.data ?? []).filter((r) => r.cost_month_usd > 0).sort((x, y) => y.cost_month_usd - x.cost_month_usd)
  return (
    <div className="space-y-3">
      <Card className="p-4">
        <div className="text-[12.5px] text-ink-3">{t('tg.today')}</div>
        <div className="text-[24px] font-semibold tabular-nums">{usd(b.spent)}</div>
        <div className="mb-2 text-[12.5px] text-ink-3">{t('tg.ofLimit', { limit: usd(b.limit) })}</div>
        <div role="progressbar" aria-label={t('tg.today')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(share * 100)} className="h-2 overflow-hidden rounded-full bg-sunken">
          <div className={cn('h-full rounded-full', share >= 0.9 ? 'bg-danger' : 'bg-accent')} style={{ width: `${share * 100}%` }} />
        </div>
      </Card>
      {cost.data && (
        <Card className="flex items-baseline justify-between p-4 text-[13.5px]">
          <span className="text-ink-2">{t('tg.month')}</span><span className="font-medium tabular-nums">{usd(cost.data.month)}</span>
        </Card>
      )}
      {mine.length > 0 && (
        <Card className="p-4">
          <div className="mb-2 text-[13px] font-medium">{t('tg.yours')}</div>
          <ul className="space-y-1.5 text-[13px]">
            {mine.map((r) => <li key={r.id} className="flex justify-between gap-3"><span className="truncate">{r.name}</span><span className="tabular-nums text-ink-2">{usd(r.cost_month_usd)}</span></li>)}
          </ul>
        </Card>
      )}
    </div>
  )
}

function Widgets({ onError }: { onError: (e: unknown) => void }) {
  const t = useT()
  const boards = useQuery({ queryKey: ['dashboards'], queryFn: api.dashboards })
  const [board, setBoard] = useState('')
  const widgets = useQuery({ queryKey: ['widgets'], queryFn: api.widgets, enabled: board === '' })
  const onBoard = useQuery({ queryKey: ['dashboard-widgets', board], queryFn: () => api.dashboardWidgets(board), enabled: board !== '' })
  useWatch(boards.error ?? widgets.error ?? onBoard.error, onError)
  const current = boards.data?.find((d) => d.id === board)
  const shown: WidgetView[] = board === ''
    ? (widgets.data ?? [])
    : (current?.layout ?? []).map((l) => onBoard.data?.[l.id]).filter((w): w is WidgetView => !!w && !('hidden' in w))
  return (
    <div>
      {(boards.data ?? []).length > 0 && (
        <div role="group" aria-label={t('tg.widgets')} className="mb-3 flex gap-1.5 overflow-x-auto">
          {[{ id: '', label: t('tg.allDashboards') }, ...boards.data!.map((d) => ({ id: d.id, label: `${d.emoji} ${d.name}`.trim() }))].map((d) => (
            <button key={d.id} aria-pressed={board === d.id} onClick={() => setBoard(d.id)}
              className={cn('shrink-0 rounded-full border px-3 py-1 text-[12.5px]', board === d.id ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2')}>{d.label}</button>
          ))}
        </div>
      )}
      {(board === '' ? widgets.isLoading : onBoard.isLoading) ? <Loading /> : shown.length === 0 ? <Empty>{t('tg.noWidgets')}</Empty> : (
        <ul className="grid gap-3">
          {shown.map((w) => <li key={w.id}><WidgetCard w={w}><WidgetBody w={w} size="wide" /></WidgetCard></li>)}
        </ul>
      )}
    </div>
  )
}

function Loading() {
  return <div className="space-y-2"><Skeleton className="h-16" /><Skeleton className="h-16" /></div>
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-10 text-center text-[13.5px] text-ink-3">{children}</p>
}
