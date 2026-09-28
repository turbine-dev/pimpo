import { useMutation, useQuery } from '@tanstack/react-query'
import { CalendarClock, Check, ChevronRight, CircleAlert, Coins, MessageSquare, ShieldQuestion, Sparkles } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Card } from '../components/ui'
import { api, type RecentRun, type RoutineSummary } from '../lib/api'
import { cn } from '../lib/cn'
import { date, relative, time, usd } from '../lib/format'
import { useT } from '../lib/i18n'
import { AssistantPicker, Composer, Suggestions } from './Chat'

function sameDay(a: Date, b: Date) {
  return a.toDateString() === b.toDateString()
}

type Entry = { key: string; at: Date; name: string; routine: string; state: 'ok' | 'failed' | 'running' | 'next' }

// Home is where the day starts: ask for anything, and see what ran, what
// comes next and what waits for you.
export function Home() {
  const t = useT()
  const nav = useNavigate()
  const [who, setWho] = useState('')
  const [model, setModel] = useState('auto')
  const [effort, setEffort] = useState('auto')
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const runs = useQuery({ queryKey: ['runs', ''], queryFn: () => api.recentRuns() })
  const chats = useQuery({ queryKey: ['chats'], queryFn: api.chats })
  const start = useMutation({
    mutationFn: ({ text }: { text: string; spoken: boolean }) => api.newChat(text, who, model, effort),
    onSuccess: (r, v) => nav(`/chat/${r.chat}`, { state: v.spoken ? { readAloud: r.turn } : undefined }),
  })

  const now = new Date()
  const hour = now.getHours()
  const greeting = t(hour < 12 ? 'home.morning' : hour < 18 ? 'home.afternoon' : 'home.evening')
  const s = state.data
  const waiting = (s?.approvals ?? 0) + (s?.awaiting ?? 0) + (s?.broken ?? 0)
  const day: Entry[] = [
    ...(runs.data ?? []).filter((r: RecentRun) => sameDay(new Date(r.started_at), now)).map((r) => ({
      key: `r${r.id}`, at: new Date(r.started_at), name: r.name, routine: r.routine,
      state: (r.outcome === 'skipped' ? 'ok' : r.outcome) as Entry['state'],
    })),
    ...(routines.data ?? []).filter((r: RoutineSummary) => r.next_run && r.state === 'active' && sameDay(new Date(r.next_run), now)).map((r) => ({
      key: `n${r.id}`, at: new Date(r.next_run!), name: r.name, routine: r.id, state: 'next' as const,
    })),
  ].sort((a, b) => a.at.getTime() - b.at.getTime())
  const pct = s && s.budget.limit > 0 ? Math.min(100, (s.budget.spent / s.budget.limit) * 100) : 0

  return (
    <div className="mx-auto max-w-4xl">
      <div className="mb-5">
        <p className="text-[13px] text-ink-3">{date(now, { weekday: 'long', day: 'numeric', month: 'long' })}</p>
        <h1 className="text-[24px] font-semibold tracking-tight">{greeting}</h1>
      </div>

      <Composer disabled={start.isPending} onSend={(text, spoken) => start.mutate({ text, spoken })} model={model} onModel={setModel} effort={effort} onEffort={setEffort} />
      {start.error && <p className="mt-2 text-[13px] text-danger">{start.error.message}</p>}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <AssistantPicker value={who} onChange={setWho} />
      </div>
      <Suggestions onPick={(text) => start.mutate({ text, spoken: false })} disabled={start.isPending} className="mt-3 sm:grid-cols-4" />

      <div className="mt-8 grid gap-4 md:grid-cols-3">
        <Link to="/inbox" className="block">
          <Card className={cn('h-full p-5 transition hover:border-line-strong', waiting > 0 && 'border-change/40')}>
            <div className="flex items-center gap-2 text-[13px] font-medium text-ink-2">
              {waiting > 0 ? <ShieldQuestion size={16} className="text-change" /> : <Check size={16} className="text-read" />} {t('home.needsYou')}
            </div>
            <div className="mt-2 text-[28px] font-semibold tabular-nums">{waiting}</div>
            <div className="text-[12.5px] text-ink-3">
              {waiting === 0 ? t('home.nothingWaiting') : [
                s?.approvals ? t('home.approvals', { count: s.approvals }) : '',
                s?.awaiting ? t('home.ready', { count: s.awaiting }) : '',
                s?.broken ? t('home.broken', { count: s.broken }) : '',
              ].filter(Boolean).join(' · ')}
            </div>
          </Card>
        </Link>

        <Card className="p-5 md:col-span-2">
          <div className="mb-3 flex items-center justify-between">
            <div className="flex items-center gap-2 text-[13px] font-medium text-ink-2"><CalendarClock size={16} /> {t('home.today')}</div>
            <Link to="/routines" className="flex items-center gap-0.5 text-[12px] text-ink-3 hover:text-ink">{t('home.allRoutines')} <ChevronRight size={13} /></Link>
          </div>
          {day.length === 0 ? (
            <p className="text-[13px] text-ink-3">{(routines.data ?? []).length === 0 ? t('home.noRoutines') : t('home.quietDay')}</p>
          ) : (
            <ul className="space-y-1.5">
              {day.slice(0, 8).map((e) => (
                <li key={e.key}>
                  <Link to={`/routines/${e.routine}`} className="flex items-center gap-3 rounded-lg px-1 py-1 text-[13px] hover:bg-sunken/60">
                    <span className="w-12 shrink-0 tabular-nums text-ink-3">{time(e.at)}</span>
                    <span className={cn('size-2 shrink-0 rounded-full', e.state === 'ok' ? 'bg-read' : e.state === 'failed' ? 'bg-danger' : e.state === 'running' ? 'animate-pulse-soft bg-accent' : 'border border-line-strong')} aria-hidden />
                    <span className="flex-1 truncate">{e.name}</span>
                    <span className={cn('text-[11.5px]', e.state === 'failed' ? 'text-danger' : 'text-ink-3')}>{t(`home.state.${e.state}`)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Link to="/cost" className="block">
          <Card className="h-full p-5 transition hover:border-line-strong">
            <div className="flex items-center gap-2 text-[13px] font-medium text-ink-2"><Coins size={16} /> {t('home.spent')}</div>
            <div className="mt-2 text-[28px] font-semibold tabular-nums">{usd(s?.budget.spent ?? 0)}</div>
            {s && s.budget.limit > 0 && <>
              <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-sunken">
                <div className={cn('h-full rounded-full', pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read')} style={{ width: `${pct}%` }} />
              </div>
              <div className="mt-1.5 text-[12px] text-ink-3">{t('cost.perDay', { limit: usd(s.budget.limit) })}</div>
            </>}
          </Card>
        </Link>

        <Card className="p-5 md:col-span-2">
          <div className="mb-3 flex items-center justify-between">
            <div className="flex items-center gap-2 text-[13px] font-medium text-ink-2"><MessageSquare size={16} /> {t('home.recentChats')}</div>
            <Link to="/chat" className="flex items-center gap-0.5 text-[12px] text-ink-3 hover:text-ink">{t('home.allChats')} <ChevronRight size={13} /></Link>
          </div>
          {(chats.data ?? []).length === 0 ? <p className="text-[13px] text-ink-3">{t('home.noChats')}</p> : (
            <ul className="space-y-1">
              {(chats.data ?? []).slice(0, 4).map((c) => (
                <li key={c.id}>
                  <Link to={`/chat/${c.id}`} className="flex items-center gap-3 rounded-lg px-1 py-1 text-[13px] hover:bg-sunken/60">
                    <span className="flex-1 truncate">{c.title}</span>
                    <span className="text-[11.5px] text-ink-3">{relative(c.updated_at)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {s && !s.claude && (
        <p className="mt-6 flex items-center gap-2 rounded-xl border border-change/30 bg-change-soft px-4 py-2.5 text-[13px] text-change">
          <CircleAlert size={15} /> {t('home.noModel')} <Link to="/settings#modelos" className="font-medium underline">{t('home.chooseModel')}</Link>
        </p>
      )}
      {s && (s.budget.limit > 0 && pct >= 100) && (
        <p className="mt-3 flex items-center gap-2 rounded-xl border border-danger/30 bg-danger-soft px-4 py-2.5 text-[13px] text-danger"><Sparkles size={15} /> {t('home.budgetOut')}</p>
      )}
    </div>
  )
}
