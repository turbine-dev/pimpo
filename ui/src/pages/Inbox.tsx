import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, BellOff, GraduationCap, Headphones, Lightbulb, MessageCircleQuestion, ShieldQuestion, Sparkles } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api } from '../lib/api'
import { useLessonCount } from './Lessons'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'

export function Inbox() {
  const t = useT()
  const nav = useNavigate()
  const qc = useQueryClient()
  const ready = useQuery({ queryKey: ['explorations', 'ready'], queryFn: () => api.explorations('ready') })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const repair = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'repair'), onSuccess: (r) => r.exploration && nav(`/explorations/${r.exploration}`) })
  const run = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'run'), onSettled: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  const approvals = useQuery({ queryKey: ['approvals'], queryFn: api.approvals, refetchInterval: 10_000 })
  const answer = useMutation({ mutationFn: ({ id, a }: { id: string; a: 'once' | 'run' | 'always' | 'deny' }) => api.answer(id, a), onSettled: () => qc.invalidateQueries({ queryKey: ['approvals'] }) })
  const media = useQuery({ queryKey: ['media'], queryFn: api.media, refetchInterval: 60_000 })
  const questions = useQuery({ queryKey: ['questions'], queryFn: api.questions, refetchInterval: 15_000 })
  const reply = useMutation({ mutationFn: ({ id, i }: { id: string; i: number }) => api.answerQuestion(id, i), onSettled: () => qc.invalidateQueries({ queryKey: ['questions'] }) })
  const suggestions = useQuery({ queryKey: ['suggestions'], queryFn: api.suggestions, refetchInterval: 60_000 })
  const suggest = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'accept' | 'dismiss' }) => api.suggestion(id, action),
    onSuccess: (r) => r.exploration && nav(`/explorations/${r.exploration}`),
    onSettled: () => qc.invalidateQueries({ queryKey: ['suggestions'] }),
  })
  const lessons = useLessonCount(true)
  const broken = (routines.data ?? []).filter((r) => r.state === 'broken')
  const items = (ready.data?.length ?? 0) + broken.length + (approvals.data?.length ?? 0) + (media.data?.length ?? 0) + (questions.data?.length ?? 0) + (suggestions.data?.length ?? 0) + lessons

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('inbox.title')}</h1>
      <p className="mb-6 text-sm text-ink-2">{t('inbox.subtitle')}</p>
      {items === 0 && (
        <EmptyState icon={<BellOff size={22} />} title={t('inbox.emptyTitle')}>
          {t('inbox.emptyText')}
        </EmptyState>
      )}
      <div className="space-y-3">
        {(questions.data ?? []).map((q) => (
          <Card key={q.id} className="flex flex-wrap items-center gap-4 border-explore/40 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-explore-soft text-explore"><MessageCircleQuestion size={18} /></div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{q.question}</div>
              <div className="text-[12.5px] text-ink-3">{t('inbox.asked', { when: relative(q.asked) })}</div>
            </div>
            <div className="flex flex-wrap gap-2">
              {q.options.map((o, i) => <Button key={o} size="sm" variant={i === 0 ? 'primary' : 'secondary'} disabled={reply.isPending} onClick={() => reply.mutate({ id: q.id, i })}>{o}</Button>)}
            </div>
          </Card>
        ))}
        {lessons > 0 && (
          <Card className="flex flex-wrap items-center gap-4 p-4">
            <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-sunken text-ink-2"><GraduationCap size={18} /></div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{t('lessons.waiting', { count: lessons })}</div>
              <div className="text-[12.5px] text-ink-3">{t('lessons.inboxText')}</div>
            </div>
            <Button size="sm" variant="primary" onClick={() => nav('/lessons')}>{t('lessons.open')}</Button>
          </Card>
        )}
        {(suggestions.data ?? []).map((s) => (
          <Card key={s.id} className="flex flex-wrap items-start gap-4 p-4">
            <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-sunken text-ink-2"><Lightbulb size={18} /></div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{s.title}</div>
              <div className="text-[13px] text-ink-2">{s.why}</div>
              <div className="mt-1 text-[12.5px] text-ink-3">{t('inbox.suggestionWould', { request: s.request })}</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="primary" disabled={suggest.isPending} onClick={() => suggest.mutate({ id: s.id, action: 'accept' })}>{t('inbox.suggestionYes')}</Button>
              <Button size="sm" variant="ghost" disabled={suggest.isPending} onClick={() => suggest.mutate({ id: s.id, action: 'dismiss' })}>{t('inbox.suggestionNo')}</Button>
            </div>
          </Card>
        ))}
        {(media.data ?? []).length > 0 && (
          <Card className="p-4">
            <div className="mb-2 flex items-center gap-2 text-[14px] font-medium"><Headphones size={15} /> {t('inbox.audio')}</div>
            <ul className="space-y-3">
              {media.data!.map((m) => (
                <li key={m.id}>
                  <div className="mb-1 flex items-baseline justify-between gap-3 text-[13px]"><span className="min-w-0 truncate">{m.title}</span><span className="shrink-0 text-[12px] text-ink-3">{relative(m.at)}</span></div>
                  <audio controls preload="none" src={`/api/media/${m.id}`} className="w-full" aria-label={m.title} />
                </li>
              ))}
            </ul>
          </Card>
        )}
        {(approvals.data ?? []).map((ap) => (
          <Card key={ap.id} className={`flex flex-wrap items-center gap-4 p-4 ${ap.action.risk >= 3 ? 'border-danger/40' : 'border-change/40'}`}>
            <div className={`grid size-10 place-items-center rounded-xl ${ap.action.risk >= 3 ? 'bg-danger-soft text-danger' : 'bg-change-soft text-change'}`}>
              <ShieldQuestion size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{ap.text}</div>
              <div className="text-[12.5px] text-ink-3">{t('inbox.rule', { reason: ap.reason })}</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="ghost" onClick={() => answer.mutate({ id: ap.id, a: 'deny' })}>{t('inbox.deny')}</Button>
              <Button size="sm" onClick={() => answer.mutate({ id: ap.id, a: 'run' })} title={t('inbox.allRunHint')}>{t('inbox.allRun')}</Button>
              <Button size="sm" onClick={() => answer.mutate({ id: ap.id, a: 'always' })}>{t('inbox.always')}</Button>
              <Button size="sm" variant="primary" onClick={() => answer.mutate({ id: ap.id, a: 'once' })}>{t('inbox.allow')}</Button>
            </div>
          </Card>
        ))}
        {broken.map((r) => (
          <Card key={r.id} className="flex flex-wrap items-center gap-4 border-danger/40 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-danger-soft text-danger">
              <AlertTriangle size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{t('inbox.didntRun', { name: r.name })}</div>
              <div className="text-[12.5px] text-ink-3">{t('inbox.paused')}</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => run.mutate(r.id)}>{t('inbox.runAgain')}</Button>
              <Button size="sm" variant="primary" onClick={() => repair.mutate(r.id)}>{t('inbox.redo')}</Button>
            </div>
          </Card>
        ))}
        {(ready.data ?? []).map((e) => (
          <Card key={e.id} className="flex flex-wrap items-center gap-4 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-accent/12 text-accent">
              <Sparkles size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px] font-medium">{e.request}</div>
              <div className="text-[12.5px] text-ink-3">{t('routines.ready', { when: relative(e.updated_at) })}</div>
            </div>
            <Button size="sm" variant="primary" onClick={() => nav(`/explorations/${e.id}`)}>{t('inbox.review')}</Button>
          </Card>
        ))}
      </div>
    </div>
  )
}
