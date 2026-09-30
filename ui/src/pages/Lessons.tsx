import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Brain, GraduationCap, Heart, Repeat, Wrench } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Lesson } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'

const icons: Record<Lesson['kind'], ReactNode> = {
  preference: <Heart size={18} />,
  fact: <Brain size={18} />,
  routine: <Repeat size={18} />,
  fix: <Wrench size={18} />,
}

// The Lessons page: what Pimpo noticed and would keep, each accepted,
// edited or rejected here. Nothing applies until the person accepts it.
export function Lessons() {
  const t = useT()
  const lessons = useQuery({ queryKey: ['lessons'], queryFn: api.lessons, refetchInterval: 60_000 })
  const proposed = lessons.data?.proposed ?? []
  const decided = (lessons.data?.decided ?? []).slice(0, 10)
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('lessons.title')}</h1>
      <p className="mb-6 text-sm text-ink-2">{t('lessons.subtitle')}</p>
      {lessons.isSuccess && proposed.length === 0 && (
        <EmptyState icon={<GraduationCap size={22} />} title={t('lessons.emptyTitle')}>{t('lessons.emptyText')}</EmptyState>
      )}
      <ul className="space-y-3" aria-label={t('lessons.title')}>
        {proposed.map((l) => <li key={l.id}><LessonCard lesson={l} /></li>)}
      </ul>
      {decided.length > 0 && (
        <section className="mt-8" aria-labelledby="lessons-decided">
          <h2 id="lessons-decided" className="mb-2 text-[13px] font-semibold uppercase tracking-wide text-ink-3">{t('lessons.decided')}</h2>
          <ul className="divide-y divide-line rounded-[var(--radius-card)] border border-line bg-surface">
            {decided.map((l) => (
              <li key={l.id + (l.decided ?? '')} className="flex items-baseline gap-3 px-4 py-2.5 text-[13px]">
                <span className="shrink-0 text-ink-3">{t(`lessons.kind.${l.kind}`)}</span>
                <span className="min-w-0 flex-1 truncate">{l.edited || l.title}</span>
                <span className="shrink-0 text-[12px] text-ink-3">{t(`lessons.state.${l.state === 'proposed' ? 'accepted' : l.state}`)}{l.decided ? ` · ${relative(l.decided)}` : ''}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

function LessonCard({ lesson: l }: { lesson: Lesson }) {
  const t = useT()
  const nav = useNavigate()
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(l.kind === 'fix' ? '' : l.change)
  const decide = useMutation({
    mutationFn: ({ action, text }: { action: 'accept' | 'edit' | 'reject'; text?: string }) => api.lesson(l.id, action, text),
    onSuccess: (done) => {
      qc.invalidateQueries({ queryKey: ['lessons'] })
      qc.invalidateQueries({ queryKey: ['memory'] })
      if (!done.result || done.state === 'rejected') return
      // A routine made goes to its page; a task started goes to where it
      // can be watched and approved.
      if (done.kind === 'fix' || done.kind === 'routine') {
        const compiled = done.state === 'accepted' && (done.kind === 'fix' || done.from === 'repeated')
        nav(compiled ? `/routines/${done.result}` : `/explorations/${done.result}`)
      }
    },
  })
  const keeps = l.kind === 'routine' ? 'lessons.keepsRoutine' : l.kind === 'fix' ? 'lessons.keepsFix' : 'lessons.keeps'
  const field = `lesson-edit-${l.id}`
  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start gap-4">
        <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-sunken text-ink-2" aria-hidden>{icons[l.kind]}</div>
        <div className="min-w-0 flex-1">
          <div className="text-[12px] text-ink-3">{t(`lessons.kind.${l.kind}`)} · {t(`lessons.from.${l.from}`)} · {relative(l.created)}</div>
          <div className="text-[14px] font-medium">{l.title}</div>
          {l.detail && <div className="mt-0.5 text-[13px] text-ink-2">{l.kind === 'fix' ? t('lessons.failed', { error: l.detail }) : l.detail}</div>}
          {l.change && l.change !== l.title && (
            <div className="mt-2 rounded-lg bg-sunken px-3 py-2 text-[13px]">
              <span className="text-ink-3">{t(keeps)} </span>{l.change}
            </div>
          )}
          {l.evidence.length > 0 && (
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[12.5px]">
              <span className="text-ink-3">{t('lessons.evidence')}:</span>
              {l.evidence.map((e, i) => <Link key={e.to + i} to={e.to} className="text-accent underline-offset-2 hover:underline">{t(`lessons.ev.${e.kind}`)}</Link>)}
            </div>
          )}
        </div>
      </div>
      {editing ? (
        <form className="mt-3 space-y-2" onSubmit={(e) => { e.preventDefault(); decide.mutate({ action: 'edit', text }) }}>
          <label htmlFor={field} className="block text-[12.5px] text-ink-2">{t(l.kind === 'fix' ? 'lessons.editFixLabel' : 'lessons.editLabel')}</label>
          <textarea id={field} value={text} onChange={(e) => setText(e.target.value)} rows={3}
            className="w-full rounded-[10px] border border-line bg-bg px-3 py-2 text-sm outline-none focus:border-accent" />
          <div className="flex justify-end gap-2">
            <Button type="button" size="sm" variant="ghost" onClick={() => setEditing(false)}>{t('lessons.cancel')}</Button>
            <Button type="submit" size="sm" variant="primary" disabled={decide.isPending || !text.trim()}>{t('lessons.saveEdit')}</Button>
          </div>
        </form>
      ) : (
        <div className="mt-3 flex flex-wrap justify-end gap-2">
          <Button size="sm" variant="ghost" disabled={decide.isPending} onClick={() => decide.mutate({ action: 'reject' })}>{t('lessons.reject')}</Button>
          <Button size="sm" disabled={decide.isPending} onClick={() => setEditing(true)}>{t('lessons.edit')}</Button>
          <Button size="sm" variant="primary" disabled={decide.isPending} onClick={() => decide.mutate({ action: 'accept' })}>{t('lessons.accept')}</Button>
        </div>
      )}
      {decide.error && <p role="alert" className="mt-2 text-[12.5px] text-danger">{decide.error.message}</p>}
    </Card>
  )
}

// useLessonCount is how many lessons wait, for the menu's badge.
export function useLessonCount(enabled: boolean): number {
  const lessons = useQuery({ queryKey: ['lessons'], queryFn: api.lessons, enabled, refetchInterval: 60_000 })
  return lessons.data?.proposed?.length ?? 0
}
