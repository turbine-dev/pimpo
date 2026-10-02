import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ListChecks, Plus, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { Field, Modal } from '../components/Modal'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type CompanyQuestion, type CompanyTask, type Org } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { relative } from '../lib/format'
import { area, field } from './Companies'

const columns: { key: string; states: CompanyTask['state'][] }[] = [
  { key: 'todo', states: ['todo'] },
  { key: 'doing', states: ['doing', 'review'] },
  { key: 'waiting', states: ['waiting', 'blocked'] },
  { key: 'done', states: ['done'] },
]

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

// Questions lists what members asked the CEO, to answer in place.
export function Questions({ org, onDiscuss }: { org: Org; onDiscuss?: (q: CompanyQuestion) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['company-questions', org.id], queryFn: () => api.companyQuestions(org.id) })
  const answer = useMutation({
    mutationFn: ({ q, choice }: { q: CompanyQuestion; choice: string }) => api.answerCompanyQuestion(org.id, q.id, choice),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['company-questions', org.id] }); qc.invalidateQueries({ queryKey: ['needs'] }) },
  })
  const mine = (list.data ?? []).filter((q) => org.members.find((m) => m.id === q.to)?.kind === 'person')
  const others = (list.data ?? []).filter((q) => !mine.includes(q))
  if (!list.data?.length) return null
  return (
    <section className="space-y-2" aria-label={t('co.questions')}>
      <h2 className="text-[15px] font-semibold">{t('co.questions')}</h2>
      {mine.map((q) => <QuestionCard key={q.id} org={org} q={q} onAnswer={org.grant !== 'view' ? (choice) => answer.mutate({ q, choice }) : undefined} onDiscuss={org.grant !== 'view' ? onDiscuss : undefined} busy={answer.isPending} />)}
      {others.map((q) => <QuestionCard key={q.id} org={org} q={q} />)}
      {answer.error && <p className="text-[13px] text-danger">{answer.error.message}</p>}
    </section>
  )
}

function QuestionCard({ org, q, onAnswer, onDiscuss, busy }: { org: Org; q: CompanyQuestion; onAnswer?: (choice: string) => void; onDiscuss?: (q: CompanyQuestion) => void; busy?: boolean }) {
  const t = useT()
  const [typed, setTyped] = useState('')
  return (
    <Card className="space-y-2 p-4">
      <p className="text-[12px] text-ink-3">{t('co.asks', { from: nameOf(org, q.from), to: nameOf(org, q.to), when: relative(q.asked) })}</p>
      <p className="text-[14px]">{q.text}</p>
      {q.recommendation && <p className="text-[12.5px] text-ink-2">{t('co.recommends', { text: q.recommendation })}</p>}
      {onAnswer ? (
        q.options?.length ? (
          <div role="group" aria-label={q.text} className="flex flex-wrap gap-1.5">
            {q.options.map((o, i) => <Button key={o} size="sm" variant={i === 0 ? 'primary' : 'secondary'} disabled={busy} onClick={() => onAnswer(o)}>{o}</Button>)}
          </div>
        ) : (
          <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (typed.trim()) onAnswer(typed.trim()) }}>
            <input className={field + ' h-8'} value={typed} aria-label={t('inbox.answerType')} placeholder={t('inbox.answerType')} onChange={(e) => setTyped(e.target.value)} />
            <Button type="submit" size="sm" disabled={busy || !typed.trim()}>{t('inbox.answerSend')}</Button>
          </form>
        )
      ) : <p className="text-[12.5px] text-ink-3">{t('co.waitingFor', { name: nameOf(org, q.to) })}</p>}
      {onDiscuss && <Button size="sm" variant="ghost" onClick={() => onDiscuss(q)}>{t('co.discuss', { name: nameOf(org, q.from) })}</Button>}
    </Card>
  )
}

export function TaskBoard({ org, can, onDiscuss }: { org: Org; can: boolean; onDiscuss?: (q: CompanyQuestion) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const tasks = useQuery({ queryKey: ['company-tasks', org.id], queryFn: () => api.companyTasks(org.id) })
  const [open, setOpen] = useState<CompanyTask | null>(null)
  const [creating, setCreating] = useState(false)
  const drop = useMutation({ mutationFn: (id: string) => api.dropTask(org.id, id), onSuccess: () => { qc.invalidateQueries({ queryKey: ['company-tasks', org.id] }); setOpen(null) } })
  const list = (tasks.data ?? []).filter((x) => x.state !== 'dropped')
  const add = can && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} /> {t('co.newTask')}</Button>
  return (
    <div className="space-y-4">
      <Questions org={org} onDiscuss={onDiscuss} />
      {tasks.isSuccess && list.length === 0 ? (
        <EmptyState icon={<ListChecks />} title={t('co.empty.tasks')} action={add}>{t('co.empty.tasksText')}</EmptyState>
      ) : (<>
      {add && <div className="flex justify-end">{add}</div>}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        {columns.map((c) => {
          const items = list.filter((x) => c.states.includes(x.state))
          return (
            <section key={c.key} aria-label={t(`co.col.${c.key}` as 'co.col.todo')} className="space-y-2 rounded-[var(--radius-card)] bg-sunken/60 p-2">
              <h3 className="px-1 text-[12.5px] font-medium text-ink-2">{t(`co.col.${c.key}` as 'co.col.todo')} <span className="tabular-nums text-ink-3">{items.length}</span></h3>
              {items.map((x) => (
                <button key={x.id} type="button" onClick={() => setOpen(x)} className="block w-full rounded-xl border border-line bg-surface p-3 text-left hover:border-line-strong">
                  <span className="block break-words text-[13px] font-medium">{x.title}</span>
                  <span className="mt-0.5 block truncate text-[12px] text-ink-3">{nameOf(org, x.requester)} → {nameOf(org, x.assignee)}</span>
                  {x.drift && <span className="mt-1 inline-flex items-center gap-1 text-[11.5px] text-change"><TriangleAlert size={12} /> {t('co.drift')}</span>}
                </button>
              ))}
            </section>
          )
        })}
      </div>
      </>)}
      {open && (
        <Modal title={open.title} onClose={() => setOpen(null)}>
          <dl className="space-y-3 text-[13px]">
            <div><dt className="text-ink-3">{t('co.objective')}</dt><dd className="whitespace-pre-line">{open.objective}</dd></div>
            <div><dt className="text-ink-3">{t('co.acceptance')}</dt><dd className="whitespace-pre-line">{open.acceptance}</dd></div>
            {open.constraints && <div><dt className="text-ink-3">{t('co.constraints')}</dt><dd className="whitespace-pre-line">{open.constraints}</dd></div>}
            {open.out_of_scope && <div><dt className="text-ink-3">{t('co.outOfScope')}</dt><dd className="whitespace-pre-line">{open.out_of_scope}</dd></div>}
            {open.report && <div><dt className="text-ink-3">{t('co.report')}</dt><dd className="whitespace-pre-line">{open.report}</dd></div>}
            {!!open.dossier?.length && (
              <div><dt className="text-ink-3">{t('co.dossier')}</dt>
                <dd><ul className="space-y-0.5">{open.dossier.map((l, i) => <li key={i} className="font-mono text-[12px]">{l.kind} {l.ref} {l.title}</li>)}</ul></dd></div>
            )}
            {open.low_trust && <p className="text-[12.5px] text-change">{t('co.lowTrust')}</p>}
          </dl>
          {can && open.state !== 'done' && <Button className={cn('mt-4 text-danger hover:bg-danger-soft hover:text-danger')} variant="ghost" onClick={() => drop.mutate(open.id)}>{t('co.dropTask')}</Button>}
        </Modal>
      )}
      {creating && <NewTask org={org} onClose={() => setCreating(false)} />}
    </div>
  )
}

function NewTask({ org, onClose }: { org: Org; onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const [task, setTask] = useState({ assignee: agents[0]?.id ?? '', title: '', objective: '', acceptance: '', constraints: '', out_of_scope: '', due: '' })
  const save = useMutation({
    mutationFn: () => api.assignTask(org.id, task),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['company-tasks', org.id] }); onClose() },
  })
  const set = (k: keyof typeof task) => (e: { target: { value: string } }) => setTask({ ...task, [k]: e.target.value })
  return (
    <Modal title={t('co.newTask')} onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.assignee')}>
          <select className={field} value={task.assignee} onChange={set('assignee')}>{agents.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}</select>
        </Field>
        <Field label={t('co.taskTitle')}><input className={field} value={task.title} maxLength={120} onChange={set('title')} /></Field>
        <Field label={t('co.objective')}><textarea className={area} value={task.objective} onChange={set('objective')} /></Field>
        <Field label={t('co.acceptance')}><textarea className={area} value={task.acceptance} placeholder={t('co.acceptanceHint')} onChange={set('acceptance')} /></Field>
        <Field label={t('co.constraints')}><input className={field} value={task.constraints} onChange={set('constraints')} /></Field>
        <Field label={t('co.outOfScope')}><input className={field} value={task.out_of_scope} onChange={set('out_of_scope')} /></Field>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" disabled={!task.assignee || !task.title.trim() || !task.objective.trim() || !task.acceptance.trim() || save.isPending}>{t('co.assign')}</Button>
      </form>
    </Modal>
  )
}
