import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, CircleDot, GraduationCap, KeyRound, Layers, Lightbulb, MessageCircleQuestion, ShieldQuestion, Sparkles, Unplug } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Need, type NeedKind, type Needs } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button } from './ui'

// One list of what needs the person signed in: approvals, keys asked for
// privately, questions, routines that stopped, jobs that went wrong, what
// is ready to look at and lessons to review, from /api/needs, answered in
// place. A kind the server adds later shows its title and an Open link
// until it gets its own buttons here: add its icon, its title and its
// actions.

export const needKinds: NeedKind[] = ['approval', 'credential_request', 'question', 'failed_routine', 'job_error', 'job_planned', 'exploration_ready', 'suggestion', 'lesson', 'system']

// useNeeds is the list; the event stream refreshes it as things change.
export function useNeeds(enabled = true) {
  return useQuery<Needs>({ queryKey: ['needs'], queryFn: api.needs, refetchInterval: 30_000, enabled })
}

type Act = { need: Need; action: string; index?: number; limit?: number }

// useNeedAction does what a button in the list says, by kind.
export function useNeedAction(onGo?: (to: string) => void) {
  const qc = useQueryClient()
  const nav = useNavigate()
  const go = (to: string) => (onGo ? onGo(to) : nav(to))
  return useMutation({
    mutationFn: async ({ need, action, index, limit }: Act) => {
      if (action === 'open') return need.link && go(need.link)
      switch (need.kind) {
        case 'approval':
          return api.answer(need.id, action as 'once' | 'run' | 'routine' | 'always' | 'deny', limit)
        case 'question':
          return api.answerQuestion(need.id, index ?? 0)
        case 'failed_routine': {
          const r = await api.routineAction(need.id, action as 'run' | 'repair')
          if (r.exploration) go(`/explorations/${r.exploration}`)
          return
        }
        case 'suggestion': {
          const r = await api.suggestion(need.id, action as 'accept' | 'dismiss')
          if (r.exploration) go(`/explorations/${r.exploration}`)
          return
        }
      }
    },
    onSettled: () => {
      for (const key of ['needs', 'state', 'approvals', 'grants', 'questions', 'routines', 'suggestions']) qc.invalidateQueries({ queryKey: [key] })
    },
  })
}

const kindLabel = (k: string) => `needs.kind.${k}` as TKey

type Tone = 'danger' | 'change' | 'explore' | 'accent' | 'plain'
const tones: Record<Tone, string> = { danger: 'bg-danger-soft text-danger', change: 'bg-change-soft text-change', explore: 'bg-explore-soft text-explore', accent: 'bg-accent/12 text-accent', plain: 'bg-sunken text-ink-2' }

function look(n: Need): { icon: ReactNode; tone: Tone } {
  switch (n.kind) {
    case 'approval': return { icon: <ShieldQuestion size={16} />, tone: (n.risk ?? 0) >= 3 ? 'danger' : 'change' }
    case 'credential_request': return { icon: <KeyRound size={16} />, tone: 'change' }
    case 'question': return { icon: <MessageCircleQuestion size={16} />, tone: 'explore' }
    case 'failed_routine': return { icon: <AlertTriangle size={16} />, tone: 'danger' }
    case 'job_error': return { icon: <Layers size={16} />, tone: 'danger' }
    case 'job_planned': return { icon: <Layers size={16} />, tone: 'plain' }
    case 'exploration_ready': return { icon: <Sparkles size={16} />, tone: 'accent' }
    case 'suggestion': return { icon: <Lightbulb size={16} />, tone: 'plain' }
    case 'lesson': return { icon: <GraduationCap size={16} />, tone: 'plain' }
    case 'system': return { icon: <Unplug size={16} />, tone: 'danger' }
  }
  return { icon: <CircleDot size={16} />, tone: 'plain' }
}

// NeedRow is one item with the buttons its kind offers; compact is the
// bell's shorter version.
export function NeedRow({ need: n, compact, busy, onAct }: { need: Need; compact?: boolean; busy?: boolean; onAct: (a: Act) => void }) {
  const t = useT()
  const { icon, tone } = look(n)
  const [limit, setLimit] = useState(n.amount === undefined ? '' : String(n.amount))
  const has = (a: string) => n.actions.includes(a)
  const btn = (action: string, label: string, variant: 'primary' | 'secondary' | 'ghost' = 'secondary', title?: string) =>
    has(action) && <Button key={action} size="sm" variant={variant} disabled={busy} title={title} onClick={() => onAct({ need: n, action })}>{label}</Button>

  let title = n.title
  let hint = n.detail ?? ''
  let buttons: ReactNode = null
  let extra: ReactNode = null
  switch (n.kind) {
    case 'approval':
      hint = n.detail ? t('inbox.rule', { reason: n.detail }) : ''
      buttons = <>
        {btn('deny', t('inbox.deny'), 'ghost')}
        {!compact && btn('run', t('inbox.allRun'), 'secondary', t('inbox.allRunHint'))}
        {!compact && has('routine') && (
          <Button size="sm" disabled={busy} title={t('inbox.forRoutineHint')}
            onClick={() => onAct({ need: n, action: 'routine', limit: n.amount !== undefined && limit ? Number(limit) : undefined })}>{t('inbox.forRoutine')}</Button>
        )}
        {!compact && btn('always', t('inbox.always'))}
        {btn('once', t('inbox.allow'), 'primary')}
      </>
      // A grant allows up to the amount this moves, or the higher limit set here.
      if (!compact && has('routine') && n.amount !== undefined) extra = (
        <label className="mt-2 flex items-center gap-2 text-[12.5px] text-ink-3">
          {t('inbox.routineLimit')}
          <input type="number" min={n.amount} step="any" inputMode="decimal" value={limit} onChange={(e) => setLimit(e.target.value)}
            className="h-7 w-24 rounded-lg border border-line bg-bg px-2 text-[12.5px] text-ink outline-none focus:border-accent" />
        </label>
      )
      break
    case 'credential_request':
      hint = n.created ? t('inbox.asked', { when: relative(n.created) }) : ''
      buttons = btn('open', t('cred.open'), 'primary')
      break
    case 'question':
      hint = n.created ? t('inbox.asked', { when: relative(n.created) }) : ''
      buttons = (
        <div role="group" aria-label={n.title} className="flex flex-wrap gap-1.5">
          {(n.options ?? []).map((o, i) => <Button key={o} size="sm" variant={i === 0 ? 'primary' : 'secondary'} disabled={busy} onClick={() => onAct({ need: n, action: 'answer', index: i })}>{o}</Button>)}
        </div>
      )
      if (!compact && has('type')) extra = <TypedAnswer need={n} busy={busy} />
      break
    case 'lesson':
      title = t('lessons.waiting', { count: n.count ?? 1 })
      hint = t('lessons.inboxText')
      buttons = btn('open', t('lessons.open'), 'primary')
      break
    case 'failed_routine':
      title = t('inbox.didntRun', { name: n.title })
      hint = n.detail || t('inbox.paused')
      buttons = <>{btn('run', t('inbox.runAgain'))}{!compact && btn('repair', t('inbox.redo'), 'primary')}{btn('open', t('needs.open'), 'ghost')}</>
      break
    case 'job_error':
      title = t('needs.jobError', { request: n.title })
      buttons = btn('open', t('needs.open'), 'primary')
      break
    case 'job_planned':
      hint = t('needs.jobPlanned')
      buttons = btn('open', t('needs.open'), 'primary')
      break
    case 'exploration_ready':
      hint = t('inbox.readyHint')
      buttons = btn('open', t('inbox.review'), 'primary')
      break
    case 'suggestion':
      buttons = <>{btn('accept', t('inbox.suggestionYes'), 'primary')}{btn('dismiss', t('inbox.suggestionNo'), 'ghost')}</>
      break
    default:
      buttons = n.link && <Button size="sm" variant="primary" disabled={busy} onClick={() => onAct({ need: n, action: 'open' })}>{t('needs.open')}</Button>
  }
  const soon = n.urgency >= 4 && n.expires
  const when = !['question', 'credential_request'].includes(n.kind) && n.created ? relative(n.created) : ''

  return (
    <div className={cn('flex items-start gap-3', compact ? 'rounded-xl px-2 py-2.5 hover:bg-sunken/50' : 'p-4')}>
      <div className={cn('grid shrink-0 place-items-center', compact ? 'size-8 rounded-lg' : 'size-10 rounded-xl', tones[tone])}>{icon}</div>
      <div className="min-w-0 flex-1">
        <div className={cn('font-medium', compact ? 'line-clamp-2 text-[13px]' : 'text-[14px] [overflow-wrap:anywhere]')}>{title}</div>
        {hint && <div className={cn('text-ink-3', compact ? 'truncate text-[11.5px]' : 'text-[12.5px] [overflow-wrap:anywhere]')}>{hint}</div>}
        {n.proposal && !compact && <div className="mt-1 text-[12.5px] text-ink-3">{t('inbox.suggestionWould', { request: n.proposal })}</div>}
        {(soon || when) && (
          <div className={cn('text-[11.5px]', soon ? 'font-medium text-danger' : 'text-ink-3')}>
            {soon ? t('needs.expires', { when: relative(n.expires) }) : when}
          </div>
        )}
        {extra}
        {buttons && <div className="mt-2 flex flex-wrap gap-1.5">{buttons}</div>}
      </div>
    </div>
  )
}

// TypedAnswer answers a question in words; the server checks them against
// the options and says which it takes when they match none.
function TypedAnswer({ need: n, busy }: { need: Need; busy?: boolean }) {
  const t = useT()
  const qc = useQueryClient()
  const [typed, setTyped] = useState('')
  const write = useMutation({
    mutationFn: (text: string) => api.answerQuestionText(n.id, text),
    onSuccess: () => { for (const key of ['needs', 'questions']) qc.invalidateQueries({ queryKey: [key] }) },
  })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (typed.trim()) write.mutate(typed.trim())
  }
  return (
    <>
      <form onSubmit={submit} className="mt-2 flex gap-2">
        <input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder={t('inbox.answerType')} aria-label={t('inbox.answerType')}
          className="h-8 min-w-0 flex-1 rounded-[10px] border border-line bg-bg px-3 text-[13px] outline-none focus:border-accent" />
        <Button type="submit" size="sm" disabled={busy || write.isPending || !typed.trim()}>{t('inbox.answerSend')}</Button>
      </form>
      {write.error && <p role="alert" className="mt-1 text-[12.5px] text-danger">{write.error.message}</p>}
    </>
  )
}

// KindFilter picks one kind or all; kinds with nothing waiting are left out.
export function KindFilter({ counts, total, value, onChange, className }: { counts: Needs['counts']; total: number; value: string; onChange: (k: string) => void; className?: string }) {
  const t = useT()
  const kinds = Object.entries(counts).filter(([, n]) => (n ?? 0) > 0).map(([k]) => k)
    .sort((a, b) => (needKinds.indexOf(a as NeedKind) + 1 || 99) - (needKinds.indexOf(b as NeedKind) + 1 || 99))
  const tab = (k: string, label: string, n: number) => (
    <button key={k} type="button" role="tab" aria-selected={value === k} onClick={() => onChange(k)}
      className={cn('-mb-px shrink-0 border-b-2 px-2 py-2 text-[12.5px]', value === k ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-3 hover:text-ink')}>
      {label}{n > 0 && <span className="ml-1 text-ink-3 tabular-nums">{n}</span>}
    </button>
  )
  return (
    <div role="tablist" aria-label={t('needs.filters')} className={cn('flex gap-1 overflow-x-auto border-b border-line [scrollbar-width:none]', className)}>
      {tab('all', t('inbox.tabAll'), total)}
      {kinds.map((k) => tab(k, needKinds.includes(k as NeedKind) ? t(kindLabel(k)) : k, counts[k as NeedKind] ?? 0))}
    </div>
  )
}
