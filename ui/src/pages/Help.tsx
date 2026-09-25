import { useMutation } from '@tanstack/react-query'
import { ArrowUp, BookOpen, CircleHelp } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useT, type TKey } from '../lib/i18n'

const questions: TKey[] = ['help.q1', 'help.q2', 'help.q3', 'help.q4', 'help.q5', 'help.q6']

// Help asks Zodim about itself: the question starts a chat, and the agent
// answers from the user guide it carries.
export function Help() {
  const t = useT()
  const nav = useNavigate()
  const [text, setText] = useState('')
  const ask = useMutation({ mutationFn: (q: string) => api.newChat(q), onSuccess: (r) => nav(`/chat/${r.chat}`) })
  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-center gap-3">
        <div className="grid size-10 place-items-center rounded-xl bg-explore-soft text-explore"><CircleHelp size={19} /></div>
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">{t('help.title')}</h1>
          <p className="text-sm text-ink-2">{t('help.text')}</p>
        </div>
      </div>
      <form className="mb-5 flex items-center gap-2 rounded-2xl border border-line bg-surface p-2 focus-within:border-accent" onSubmit={(e) => { e.preventDefault(); if (text.trim()) ask.mutate(text.trim()) }}>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder={t('help.ask')} aria-label={t('help.ask')} className="h-10 flex-1 bg-transparent px-2 text-sm outline-none" />
        <button type="submit" aria-label={t('chat.send')} disabled={!text.trim() || ask.isPending} className="grid size-9 place-items-center rounded-xl bg-ink text-bg disabled:opacity-30"><ArrowUp size={16} /></button>
      </form>
      <div className="grid gap-2 sm:grid-cols-2">
        {questions.map((q) => (
          <button key={q} type="button" onClick={() => ask.mutate(t(q))} disabled={ask.isPending}
            className="rounded-xl border border-line bg-surface px-4 py-3 text-left text-[13px] text-ink-2 transition hover:border-line-strong hover:text-ink">
            {t(q)}
          </button>
        ))}
      </div>
      {ask.error && <p className="mt-3 text-[13px] text-danger">{ask.error.message}</p>}
      <a href="https://github.com/denerFernandes/zodim/blob/main/docs/USER_GUIDE.md" target="_blank" rel="noreferrer" className="mt-6 inline-flex items-center gap-1.5 text-[12.5px] text-ink-3 underline">
        <BookOpen size={13} /> {t('help.guide')}
      </a>
    </div>
  )
}
