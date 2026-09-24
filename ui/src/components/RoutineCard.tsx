import { Archive, CalendarDays, Clock, Globe, Mail, Send, Tag } from 'lucide-react'
import { motion } from 'motion/react'
import { type ReactNode } from 'react'
import type { RoutineSummary } from '../lib/api'
import { cn } from '../lib/cn'
import { cronText, usd, when } from '../lib/format'
import { Card, RunDots } from './ui'

const capIcon: Record<string, ReactNode> = {
  'calendar.events': <CalendarDays size={13} />,
  'gmail.search': <Mail size={13} />,
  'gmail.archive': <Archive size={13} />,
  'gmail.label': <Tag size={13} />,
  'http.getJSON': <Globe size={13} />,
  'telegram.send': <Send size={13} />,
}

const capLabel: Record<string, string> = {
  'calendar.events': 'Lê a agenda',
  'gmail.search': 'Lê e-mails',
  'gmail.archive': 'Arquiva e-mails',
  'gmail.label': 'Marca e-mails',
  'http.getJSON': 'Consulta',
  'telegram.send': 'Avisa você',
}

export const capRisk: Record<string, 'read' | 'notify' | 'reversible' | 'irreversible'> = {
  'calendar.events': 'read',
  'gmail.search': 'read',
  'http.getJSON': 'read',
  'telegram.send': 'notify',
  'gmail.archive': 'reversible',
  'gmail.label': 'reversible',
}

export function capabilityLabel(entry: string) {
  const [name, scope] = entry.split(':')
  const base = capLabel[name] ?? name
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
  active: { label: 'Ativa', cls: 'text-read' },
  paused: { label: 'Pausada', cls: 'text-ink-3' },
  broken: { label: 'Precisa de atenção', cls: 'text-danger' },
}

export function RoutineCard({ r, onOpen }: { r: RoutineSummary; onOpen?: () => void }) {
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
        aria-label={`Rotina ${r.name}`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate text-[15px] font-semibold tracking-tight">{r.name}</h3>
            <p className="mt-0.5 line-clamp-2 text-[13px] text-ink-2">{r.description}</p>
          </div>
          <span className={cn('flex shrink-0 items-center gap-1.5 text-[12px] font-medium', st.cls)}>
            <span className={cn('size-1.5 rounded-full bg-current', r.state === 'broken' && 'animate-pulse-soft')} />
            {st.label}
          </span>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {r.capabilities.map((c) => (
            <CapabilityChip key={c} entry={c} />
          ))}
        </div>
        <div className="mt-auto flex items-end justify-between gap-3 border-t border-line pt-3.5">
          <div className="space-y-1.5">
            <RunDots runs={runs.length ? runs : ['skipped']} />
            <div className="flex items-center gap-1 text-[12px] text-ink-3">
              <Clock size={12} /> {r.state === 'active' && r.next_run ? when(r.next_run) : cronText(r.schedule)}
            </div>
          </div>
          <div className="text-right">
            <div className="text-[15px] font-semibold tabular-nums">{usd(r.cost_month_usd)}</div>
            <div className="text-[11px] text-ink-3">este mês</div>
          </div>
        </div>
      </Card>
    </motion.div>
  )
}
