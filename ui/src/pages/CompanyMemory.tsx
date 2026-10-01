import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Field, Modal } from '../components/Modal'
import { Button, Card } from '../components/ui'
import { api, type Meeting, type Org } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { area, field } from './Companies'

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

// author is who wrote a note: a person, or a member of the company.
function author(org: Org, by: string, you: string) {
  if (by.startsWith('human:')) return you
  return nameOf(org, by.split('/').pop() ?? by)
}

export function Digest({ org }: { org: Org }) {
  const t = useT()
  const digest = useQuery({ queryKey: ['company-digest', org.id], queryFn: () => api.companyDigest(org.id) })
  if (!digest.data) return null
  return (
    <Card className="p-4">
      <h2 className="mb-1 text-[13.5px] font-semibold">{t('co.thisWeek')}</h2>
      <p className="whitespace-pre-line text-[12.5px] text-ink-2">{digest.data.text}</p>
    </Card>
  )
}

export function MemoryTab({ org, can }: { org: Org; can: boolean }) {
  const t = useT()
  const qc = useQueryClient()
  const notes = useQuery({ queryKey: ['company-notes', org.id], queryFn: () => api.companyNotes(org.id) })
  const meetings = useQuery({ queryKey: ['company-meetings', org.id], queryFn: () => api.meetings(org.id), refetchInterval: (q) => (q.state.data?.some((m) => m.state === 'running') ? 3000 : false) })
  const [note, setNote] = useState({ kind: 'decision', title: '', body: '' })
  const [meeting, setMeeting] = useState(false)
  const refresh = () => qc.invalidateQueries({ queryKey: ['company-notes', org.id] })
  const add = useMutation({ mutationFn: () => api.saveCompanyNote(org.id, note), onSuccess: () => { setNote({ ...note, title: '', body: '' }); refresh() } })
  const remove = useMutation({ mutationFn: (id: string) => api.deleteCompanyNote(org.id, id), onSuccess: refresh })
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.memory')}</h2>
        <p className="text-[12.5px] text-ink-3">{t('co.memoryHint')}</p>
        {can && (
          <form className="space-y-2 rounded-[var(--radius-card)] border border-line p-3" onSubmit={(e) => { e.preventDefault(); add.mutate() }}>
            <div className="flex gap-2">
              <select className={field + ' w-36'} aria-label={t('co.noteKind')} value={note.kind} onChange={(e) => setNote({ ...note, kind: e.target.value })}>
                {['decision', 'fact', 'lesson'].map((k) => <option key={k} value={k}>{t(`co.note.${k}` as 'co.note.fact')}</option>)}
              </select>
              <input className={field} aria-label={t('co.noteTitle')} placeholder={t('co.noteTitle')} value={note.title} maxLength={120} onChange={(e) => setNote({ ...note, title: e.target.value })} />
            </div>
            <textarea className={area} aria-label={t('co.noteText')} placeholder={t('co.noteText')} value={note.body} onChange={(e) => setNote({ ...note, body: e.target.value })} />
            {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
            <Button type="submit" size="sm" disabled={!note.title.trim() || !note.body.trim() || add.isPending}><Plus size={13} /> {t('co.keep')}</Button>
          </form>
        )}
        {(notes.data ?? []).map((n) => (
          <Card key={n.id} className="flex items-start gap-3 p-4">
            <div className="min-w-0 flex-1">
              <p className="text-[12px] text-ink-3">{t(`co.note.${n.kind}` as 'co.note.fact')} · {author(org, n.by, t('co.you'))} · {relative(n.created)}</p>
              <p className="text-[14px] font-medium">{n.title}</p>
              <p className="line-clamp-4 whitespace-pre-line text-[12.5px] text-ink-2">{n.body}</p>
            </div>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.forget', { name: n.title })} onClick={() => remove.mutate(n.id)}><Trash2 size={14} /></Button>}
          </Card>
        ))}
      </section>
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-[15px] font-semibold">{t('co.meetings')}</h2>
          {can && org.members.filter((m) => m.kind === 'agent').length > 1 && <Button size="sm" onClick={() => setMeeting(true)}><Plus size={14} /> {t('co.newMeeting')}</Button>}
        </div>
        {(meetings.data ?? []).length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noMeetings')}</p>}
        {(meetings.data ?? []).map((m) => <MeetingCard key={m.id} org={org} m={m} />)}
      </section>
      {meeting && <NewMeeting org={org} onClose={() => setMeeting(false)} />}
    </div>
  )
}

function MeetingCard({ org, m }: { org: Org; m: Meeting }) {
  const t = useT()
  return (
    <Card className="p-4">
      <details>
        <summary className="flex cursor-pointer list-none items-center gap-2">
          <span className={cn('rounded-md px-1.5 py-0.5 text-[11.5px] font-medium', m.state === 'done' ? 'bg-read-soft text-read' : m.state === 'failed' ? 'bg-danger-soft text-danger' : 'bg-explore-soft text-explore')}>{t(`co.meeting.${m.state}` as 'co.meeting.done')}</span>
          <span className="min-w-0 flex-1 truncate text-[14px] font-medium">{m.title}</span>
          <span className="text-[12px] tabular-nums text-ink-3">${m.cost_usd.toFixed(2)}</span>
        </summary>
        <div className="mt-3 space-y-2 text-[12.5px]">
          <p className="text-ink-2">{m.agenda}</p>
          {m.minutes && <p className="whitespace-pre-line">{m.minutes}</p>}
          {!!m.decisions?.length && <ul className="list-disc pl-5">{m.decisions.map((d) => <li key={d}>{d}</li>)}</ul>}
          {m.error && <p className="text-danger">{m.error}</p>}
          <div className="space-y-1 border-t border-line pt-2">
            {m.transcript.map((turn, i) => <p key={i}><b className="font-medium">{nameOf(org, turn.member)}:</b> <span className="text-ink-2">{turn.text}</span></p>)}
          </div>
        </div>
      </details>
    </Card>
  )
}

function NewMeeting({ org, onClose }: { org: Org; onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const [m, setM] = useState({ title: '', agenda: '', chair: agents[0]?.id ?? '', participants: agents.slice(0, 2).map((a) => a.id), rounds: 2, max_usd: 1 })
  const save = useMutation({ mutationFn: () => api.meet(org.id, m), onSuccess: () => { qc.invalidateQueries({ queryKey: ['company-meetings', org.id] }); onClose() } })
  const toggle = (id: string) => setM({ ...m, participants: m.participants.includes(id) ? m.participants.filter((x) => x !== id) : [...m.participants, id] })
  return (
    <Modal title={t('co.newMeeting')} onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.meetingTitle')}><input className={field} value={m.title} onChange={(e) => setM({ ...m, title: e.target.value })} /></Field>
        <Field label={t('co.agenda')}><textarea className={area} value={m.agenda} onChange={(e) => setM({ ...m, agenda: e.target.value })} /></Field>
        <fieldset>
          <legend className="mb-1 text-[12.5px] text-ink-2">{t('co.participants')}</legend>
          <div className="flex flex-wrap gap-3">
            {agents.map((a) => (
              <label key={a.id} className="flex items-center gap-1.5 text-[13px]">
                <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={m.participants.includes(a.id)} onChange={() => toggle(a.id)} /> {a.name}
              </label>
            ))}
          </div>
        </fieldset>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label={t('co.chair')}>
            <select className={field} value={m.chair} onChange={(e) => setM({ ...m, chair: e.target.value })}>{agents.filter((a) => m.participants.includes(a.id)).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}</select>
          </Field>
          <Field label={t('co.rounds')}><input type="number" min={1} max={4} className={field} value={m.rounds} onChange={(e) => setM({ ...m, rounds: Number(e.target.value) })} /></Field>
          <Field label={t('co.maxUsd')}><input type="number" min={0.1} max={2} step={0.1} className={field} value={m.max_usd} onChange={(e) => setM({ ...m, max_usd: Number(e.target.value) })} /></Field>
        </div>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" disabled={!m.title.trim() || !m.agenda.trim() || m.participants.length < 2 || !m.participants.includes(m.chair) || save.isPending}>{t('co.startMeeting')}</Button>
      </form>
    </Modal>
  )
}
