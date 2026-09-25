import { Archive, Bell, BellOff, CalendarDays, CheckSquare, Clock, FilePen, GitPullRequest, Globe, House, KeyRound, ListTodo, Mail, MailPlus, MessageCircle, MessagesSquare, NotebookPen, Rss, Search, Send, Tag, Trash2 } from 'lucide-react'
import { motion } from 'motion/react'
import { type ReactNode } from 'react'
import type { RoutineSummary } from '../lib/api'
import { cn } from '../lib/cn'
import { cronText, usd, when } from '../lib/format'
import { hasKey, tr, useT } from '../lib/i18n'
import { Card, RunDots } from './ui'

const capIcon: Record<string, ReactNode> = {
  'calendar.events': <CalendarDays size={13} />,
  'gmail.search': <Mail size={13} />,
  'gmail.archive': <Archive size={13} />,
  'gmail.label': <Tag size={13} />,
  'http.getJSON': <Globe size={13} />,
  'telegram.send': <Send size={13} />,
  'gmail.trash': <Trash2 size={13} />,
  'gmail.delete': <Trash2 size={13} />,
  'gmail.draft': <FilePen size={13} />,
  'gmail.send': <MailPlus size={13} />,
  'gmail.unsubscribe': <BellOff size={13} />,
  'whatsapp.send': <MessageCircle size={13} />,
  'whatsapp.send_to': <MessageCircle size={13} />,
  'notify.send': <Bell size={13} />,
  'discord.send': <MessagesSquare size={13} />,
  'slack.send': <MessagesSquare size={13} />,
  'github.issues': <GitPullRequest size={13} />,
  'github.comment': <GitPullRequest size={13} />,
  'ha.states': <House size={13} />,
  'ha.call': <House size={13} />,
  'ha.critical': <KeyRound size={13} />,
  'notion.search': <NotebookPen size={13} />,
  'notion.append': <NotebookPen size={13} />,
  'obsidian.search': <NotebookPen size={13} />,
  'obsidian.append': <NotebookPen size={13} />,
  'rss.read': <Rss size={13} />,
  'todoist.tasks': <ListTodo size={13} />,
  'todoist.add': <ListTodo size={13} />,
  'todoist.close': <CheckSquare size={13} />,
  'web.search': <Search size={13} />,
}

export const capRisk: Record<string, 'read' | 'notify' | 'reversible' | 'irreversible'> = {
  'calendar.events': 'read',
  'gmail.search': 'read',
  'http.getJSON': 'read',
  'telegram.send': 'notify',
  'gmail.archive': 'reversible',
  'gmail.label': 'reversible',
  'gmail.trash': 'reversible',
  'gmail.draft': 'reversible',
  'gmail.delete': 'irreversible',
  'gmail.send': 'irreversible',
  'gmail.unsubscribe': 'irreversible',
  'whatsapp.send': 'notify',
  'whatsapp.send_to': 'irreversible',
  'notify.send': 'notify',
  'discord.send': 'notify',
  'slack.send': 'notify',
  'github.issues': 'read',
  'github.comment': 'irreversible',
  'ha.states': 'read',
  'ha.call': 'reversible',
  'ha.critical': 'irreversible',
  'notion.search': 'read',
  'notion.append': 'reversible',
  'obsidian.search': 'read',
  'obsidian.append': 'reversible',
  'rss.read': 'read',
  'todoist.tasks': 'read',
  'todoist.add': 'reversible',
  'todoist.close': 'reversible',
  'web.search': 'read',
}

export function capabilityLabel(entry: string) {
  const [name, scope] = entry.split(':')
  const key = `cap.${name}`
  const base = hasKey(key) ? tr(key) : name
  return scope ? `${base} ${scope}` : base
}

export function CapabilityChip({ entry }: { entry: string }) {
  const name = entry.split(':')[0]
  const writes = capRisk[name] === 'reversible' || capRisk[name] === 'irreversible'
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11.5px]', writes ? 'border-change/30 text-change' : 'border-line text-ink-2')} title={capabilityLabel(entry)}>
      {capIcon[name] ?? <Globe size={13} />}
      <span className="max-w-[150px] truncate">{capabilityLabel(entry)}</span>
    </span>
  )
}

const stateStyle = {
  active: { label: 'routine.state.active', cls: 'text-read' },
  paused: { label: 'routine.state.paused', cls: 'text-ink-3' },
  broken: { label: 'routine.state.broken', cls: 'text-danger' },
} as const

export function RoutineCard({ r, onOpen }: { r: RoutineSummary; onOpen?: () => void }) {
  const t = useT()
  const st = stateStyle[r.state] ?? stateStyle.active
  const runs = r.runs.slice(-14).map((o) => (o === 'ok' ? 'ok' : o === 'failed' ? 'failed' : 'skipped')) as ('ok' | 'failed' | 'skipped')[]
  return (
    <motion.div layout layoutId={`routine-${r.id}`} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25, ease: 'easeOut' }}>
      <Card
        role="button"
        tabIndex={0}
        onClick={onOpen}
        onKeyDown={(e) => e.key === 'Enter' && onOpen?.()}
        className={cn('group flex h-full cursor-pointer flex-col gap-4 p-5 transition-[border,transform] hover:-translate-y-0.5 hover:border-line-strong', r.state === 'broken' && 'border-danger/40')}
        aria-label={t('routine.aria', { name: r.name })}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate text-[15px] font-semibold tracking-tight">{r.name}</h3>
            <p className="mt-0.5 line-clamp-2 text-[13px] text-ink-2">{r.description}</p>
          </div>
          <span className={cn('flex shrink-0 items-center gap-1.5 text-[12px] font-medium', st.cls)}>
            <span className={cn('size-1.5 rounded-full bg-current', r.state === 'broken' && 'animate-pulse-soft')} />
            {t(st.label)}
          </span>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {r.capabilities.map((c) => (
            <CapabilityChip key={c} entry={c} />
          ))}
        </div>
        <div className="mt-auto flex items-end justify-between gap-3 border-t border-line pt-3.5">
          <div className="space-y-1.5">
            {runs.length ? <RunDots runs={runs} /> : <span className="text-[12px] text-ink-3">{t('routine.noRunsYet')}</span>}
            <div className="flex items-center gap-1 text-[12px] text-ink-3">
              <Clock size={12} /> {r.watch && !r.schedule ? t('routine.onNew', { what: capabilityLabel(r.watch.capability).toLowerCase() }) : r.state === 'active' && r.next_run ? when(r.next_run) : cronText(r.schedule)}
            </div>
          </div>
          <div className="text-right">
            <div className="text-[15px] font-semibold tabular-nums">{usd(r.cost_month_usd)}</div>
            <div className="text-[11px] text-ink-3">{t('routine.thisMonth')}</div>
          </div>
        </div>
      </Card>
    </motion.div>
  )
}
