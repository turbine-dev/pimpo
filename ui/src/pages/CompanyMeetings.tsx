import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Plus, Send } from 'lucide-react'
import { useState } from 'react'
import { Field, Modal } from '../components/Modal'
import { Button, Card } from '../components/ui'
import { api, type Meeting, type Org } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { area, field } from './Companies'

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

const stateCls: Record<Meeting['state'], string> = { open: 'bg-explore-soft text-explore', running: 'bg-explore-soft text-explore', done: 'bg-read-soft text-read', failed: 'bg-danger-soft text-danger' }

export function MeetingsTab({ org, can, room, onRoom }: { org: Org; can: boolean; room: string | null; onRoom: (id: string | null) => void }) {
  const t = useT()
  const meetings = useQuery({ queryKey: ['company-meetings', org.id], queryFn: () => api.meetings(org.id), refetchInterval: (q) => (q.state.data?.some((m) => m.state === 'running') ? 3000 : false) })
  const [creating, setCreating] = useState(false)
  if (room) return <Room org={org} id={room} can={can} onBack={() => onRoom(null)} />
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[12.5px] text-ink-3">{t('co.meetingsHint')}</p>
        {can && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} /> {t('co.newMeeting')}</Button>}
      </div>
      {(meetings.data ?? []).length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noMeetings')}</p>}
      {(meetings.data ?? []).map((m) => (
        <Card key={m.id} className="p-0">
          <button type="button" onClick={() => onRoom(m.id)} className="flex w-full items-center gap-2 px-4 py-3 text-left hover:bg-sunken/50">
            <span className={cn('rounded-md px-1.5 py-0.5 text-[11.5px] font-medium', stateCls[m.state])}>{t(`co.meeting.${m.state}` as 'co.meeting.done')}</span>
            <span className="min-w-0 flex-1 truncate text-[14px] font-medium">{m.title}</span>
            <span className="text-[12px] text-ink-3">{m.participants.map((p) => nameOf(org, p)).join(', ')} · {relative(m.created)}</span>
          </button>
        </Card>
      ))}
      {creating && <NewMeeting org={org} onClose={() => setCreating(false)} onOpen={onRoom} />}
    </div>
  )
}

function Room({ org, id, can, onBack }: { org: Org; id: string; can: boolean; onBack: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const meeting = useQuery({ queryKey: ['company-meeting', org.id, id], queryFn: () => api.meeting(org.id, id), refetchInterval: 3000 })
  const [text, setText] = useState('')
  const [to, setTo] = useState<string[]>([])
  const [task, setTask] = useState<{ member: string; request: string } | null>(null)
  const refresh = () => qc.invalidateQueries({ queryKey: ['company-meeting', org.id, id] })
  const say = useMutation({ mutationFn: () => api.say(org.id, id, text, to), onSuccess: () => { setText(''); refresh() } })
  const end = useMutation({ mutationFn: () => api.endMeeting(org.id, id), onSuccess: () => { refresh(); qc.invalidateQueries({ queryKey: ['company-meetings', org.id] }) } })
  const give = useMutation({ mutationFn: () => api.meetingTask(org.id, id, task!.member, task!.request), onSuccess: () => setTask(null) })
  const m = meeting.data
  if (!m) return null
  const live = m.state === 'open'
  const waiting = live && m.transcript.length > 0 && m.transcript[m.transcript.length - 1].member === 'ceo'
  return (
    <div className="mx-auto max-w-3xl space-y-3">
      <button type="button" onClick={onBack} className="inline-flex items-center gap-1 text-[13px] text-ink-3 hover:text-ink"><ArrowLeft size={14} /> {t('co.meetings')}</button>
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-[17px] font-semibold">{m.title}</h2>
          {m.agenda && <p className="text-[13px] text-ink-2">{m.agenda}</p>}
        </div>
        {live && m.with_ceo && can && <Button onClick={() => end.mutate()} disabled={end.isPending}>{t('co.endMeeting')}</Button>}
      </div>
      <div className="space-y-2" aria-live="polite">
        {m.transcript.map((turn, i) => {
          const ceo = turn.member === 'ceo'
          return (
            <div key={i} className={cn('flex', ceo && 'justify-end')}>
              <div className={cn('max-w-[85%] rounded-2xl px-3.5 py-2.5 text-[13.5px]', ceo ? 'bg-ink text-bg' : 'border border-line bg-surface')}>
                <p className={cn('mb-0.5 text-[11.5px] font-medium', ceo ? 'text-bg/70' : 'text-ink-3')}>{nameOf(org, turn.member)}</p>
                <p className="whitespace-pre-line">{turn.text}</p>
                {!ceo && can && <button type="button" className="mt-1 text-[11.5px] text-ink-3 underline-offset-2 hover:underline" onClick={() => setTask({ member: turn.member, request: '' })}>{t('co.makeTask')}</button>}
              </div>
            </div>
          )
        })}
        {waiting && <p className="text-[12.5px] text-ink-3">{t('co.answering')}</p>}
      </div>
      {m.minutes && (
        <Card className="space-y-1 p-4 text-[13px]">
          <p className="font-medium">{t('co.minutes')}</p>
          <p className="whitespace-pre-line text-ink-2">{m.minutes}</p>
          {!!m.decisions?.length && <ul className="list-disc pl-5">{m.decisions.map((d) => <li key={d}>{d}</li>)}</ul>}
        </Card>
      )}
      {m.error && <p className="text-[13px] text-danger">{m.error}</p>}
      {live && m.with_ceo && can && (
        <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); say.mutate() }}>
          <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('co.speakTo')}>
            <span className="text-[12px] text-ink-3">{t('co.speakTo')}</span>
            <button type="button" aria-pressed={to.length === 0} onClick={() => setTo([])} className={cn('rounded-full border px-2.5 py-0.5 text-[12px]', to.length === 0 ? 'border-ink bg-ink text-bg' : 'border-line')}>{t('co.everyone')}</button>
            {m.participants.map((p) => (
              <button key={p} type="button" aria-pressed={to.includes(p)} onClick={() => setTo(to.includes(p) ? to.filter((x) => x !== p) : [...to, p])}
                className={cn('rounded-full border px-2.5 py-0.5 text-[12px]', to.includes(p) ? 'border-ink bg-ink text-bg' : 'border-line')}>{nameOf(org, p)}</button>
            ))}
          </div>
          <div className="flex gap-2">
            <textarea className={area + ' min-h-12 flex-1'} aria-label={t('co.yourMessage')} placeholder={t('co.yourMessage')} value={text} onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (text.trim() && !waiting) say.mutate() } }} />
            <Button type="submit" variant="primary" aria-label={t('co.send')} disabled={!text.trim() || waiting || say.isPending}><Send size={15} /></Button>
          </div>
          {say.error && <p className="text-[13px] text-danger">{say.error.message}</p>}
        </form>
      )}
      {task && (
        <Modal title={t('co.makeTaskFor', { name: nameOf(org, task.member) })} onClose={() => setTask(null)}>
          <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); give.mutate() }}>
            <Field label={t('co.whatToDo')}><textarea className={area} value={task.request} onChange={(e) => setTask({ ...task, request: e.target.value })} /></Field>
            <p className="text-[12px] text-ink-3">{t('co.makeTaskHint')}</p>
            {give.error && <p className="text-[13px] text-danger">{give.error.message}</p>}
            <Button type="submit" variant="primary" disabled={!task.request.trim() || give.isPending}>{t('co.giveWorkNow')}</Button>
          </form>
        </Modal>
      )}
    </div>
  )
}

export function NewMeeting({ org, onClose, onOpen, question, with: start }: { org: Org; onClose: () => void; onOpen: (id: string) => void; question?: string; with?: string[] }) {
  const t = useT()
  const qc = useQueryClient()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const [m, setM] = useState({ title: '', agenda: '', participants: start ?? agents.map((a) => a.id), withCEO: true, chair: agents[0]?.id ?? '', rounds: 2, max_usd: 1 })
  const save = useMutation({
    mutationFn: () => api.meet(org.id, m.withCEO
      ? { title: m.title, agenda: m.agenda, participants: m.participants, max_usd: m.max_usd, with_ceo: true, question }
      : { title: m.title, agenda: m.agenda, chair: m.chair, participants: m.participants, rounds: m.rounds, max_usd: m.max_usd }),
    onSuccess: (x) => { qc.invalidateQueries({ queryKey: ['company-meetings', org.id] }); onClose(); onOpen(x.id) },
  })
  const toggle = (id: string) => setM({ ...m, participants: m.participants.includes(id) ? m.participants.filter((x) => x !== id) : [...m.participants, id] })
  const enough = m.participants.length >= (m.withCEO ? 1 : 2)
  return (
    <Modal title={t('co.newMeeting')} onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.meetingTitle')}><input className={field} value={m.title} onChange={(e) => setM({ ...m, title: e.target.value })} /></Field>
        <Field label={t('co.agenda')}><textarea className={area} value={m.agenda} onChange={(e) => setM({ ...m, agenda: e.target.value })} /></Field>
        <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={m.withCEO} onChange={(e) => setM({ ...m, withCEO: e.target.checked })} /> {t('co.iTakePart')}</label>
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
        {!m.withCEO && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('co.chair')}>
              <select className={field} value={m.chair} onChange={(e) => setM({ ...m, chair: e.target.value })}>{agents.filter((a) => m.participants.includes(a.id)).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}</select>
            </Field>
            <Field label={t('co.rounds')}><input type="number" min={1} max={4} className={field} value={m.rounds} onChange={(e) => setM({ ...m, rounds: Number(e.target.value) })} /></Field>
          </div>
        )}
        <Field label={t('co.maxUsd')}><input type="number" min={0.1} max={2} step={0.1} className={field + ' w-32'} value={m.max_usd} onChange={(e) => setM({ ...m, max_usd: Number(e.target.value) })} /></Field>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" disabled={!m.title.trim() || (!m.withCEO && !m.agenda.trim()) || !enough || (!m.withCEO && !m.participants.includes(m.chair)) || save.isPending}>{t('co.startMeeting')}</Button>
      </form>
    </Modal>
  )
}
