import { CirclePause, KeyRound, User } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import type { Member, Org } from '../lib/api'
import { below, deptColor } from '../lib/org'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

function Avatar({ member }: { member: Member }) {
  if (member.kind === 'person') return <span className="grid size-9 shrink-0 place-items-center rounded-full bg-ink text-bg" aria-hidden><User size={16} /></span>
  return <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-sunken text-[18px]" aria-hidden>{member.avatar || member.name.slice(0, 1).toUpperCase()}</span>
}

function MemberCard({ org, member, onOpen, drag }: { org: Org; member: Member; onOpen: (m: Member) => void; drag?: DragProps }) {
  const t = useT()
  const role = org.roles.find((r) => r.id === member.role)
  const color = deptColor(org, member.department)
  const act = org.activity?.[member.id]
  return (
    <button type="button" onClick={() => onOpen(member)} draggable={!!drag && member.id !== 'ceo'}
      onDragStart={(e) => { e.dataTransfer.setData('text/plain', member.id); drag?.start(member.id) }} onDragEnd={() => drag?.end()}
      onDragOver={(e) => { if (drag?.canDrop(member.id)) { e.preventDefault(); drag.over(member.id) } }}
      onDragLeave={() => drag?.over('')} onDrop={(e) => { e.preventDefault(); drag?.drop(member.id) }}
      aria-label={t('co.open', { name: member.name })}
      className={cn('flex w-48 items-center gap-2.5 rounded-[var(--radius-card)] border border-line bg-surface p-2.5 text-left shadow-[var(--shadow-card)] hover:border-line-strong',
        drag?.target === member.id && 'border-ink ring-2 ring-ink/20', drag?.dragging === member.id && 'opacity-50')}
      style={color ? { borderLeft: `3px solid ${color}` } : undefined}>
      <Avatar member={member} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13.5px] font-medium">{member.name}</span>
        <span className="block truncate text-[12px] text-ink-3">{member.kind === 'person' ? (member.title || t('co.person')) : role?.title}</span>
      </span>
      {member.state === 'paused' && <CirclePause size={14} className="shrink-0 text-ink-3" aria-label={t('co.paused')} />}
      {act?.state === 'working' && <span className="size-2 shrink-0 animate-pulse-soft rounded-full bg-explore" title={act.task} aria-label={t('co.workingOn', { task: act.task ?? '' })} />}
      {act?.state === 'account_missing' && <KeyRound size={14} className="shrink-0 text-change" aria-label={t('co.missingAccounts', { list: (act.missing ?? []).join(', ') })} />}
      {act?.state === 'queued' && <span className="shrink-0 rounded-full bg-sunken px-1.5 text-[11px] tabular-nums text-ink-2" aria-label={t('co.inQueue', { count: act.queue ?? 0 })}>{act.queue}</span>}
    </button>
  )
}

type DragProps = {
  dragging: string
  target: string
  start: (id: string) => void
  end: () => void
  over: (id: string) => void
  canDrop: (id: string) => boolean
  drop: (id: string) => void
}

// OrgChart draws the company as a tree; with onMove, dragging a card onto
// another makes it report there. The member dialog has the same choice for
// keyboards.
export function OrgChart({ org, onOpen, onMove }: { org: Org; onOpen: (m: Member) => void; onMove?: (member: string, boss: string) => void }) {
  const t = useT()
  const [dragging, setDragging] = useState('')
  const [target, setTarget] = useState('')
  const drag: DragProps | undefined = onMove && {
    dragging, target,
    start: setDragging,
    end: () => { setDragging(''); setTarget('') },
    over: setTarget,
    canDrop: (id) => !!dragging && id !== dragging && !below(org, dragging).has(id) && org.members.find((m) => m.id === dragging)?.reports_to !== id,
    drop: (id) => {
      if (drag?.canDrop(id)) onMove(dragging, id)
      setDragging('')
      setTarget('')
    },
  }
  const reports = (id: string) => org.members.filter((m) => m.reports_to === id)
  const branch = (m: Member) => (
    <li key={m.id}>
      <MemberCard org={org} member={m} onOpen={onOpen} drag={drag} />
      {reports(m.id).length > 0 && <ul>{reports(m.id).map(branch)}</ul>}
    </li>
  )
  const list = (m: Member, depth: number): ReactNode[] => [
    <li key={m.id} style={{ paddingLeft: depth * 16 }}><MemberCard org={org} member={m} onOpen={onOpen} /></li>,
    ...reports(m.id).flatMap((r) => list(r, depth + 1)),
  ]
  const top = org.members.find((m) => m.id === 'ceo')
  if (!top) return null
  return (
    <>
      <div className="org hidden overflow-x-auto pb-4 md:block" aria-label={t('co.chart')}><ul>{branch(top)}</ul></div>
      <ul className="space-y-2 md:hidden" aria-label={t('co.chart')}>{list(top, 0)}</ul>
    </>
  )
}
