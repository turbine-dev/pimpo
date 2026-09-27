import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowUp, Check, Loader2, Mic, Repeat, Square, Trash2, Volume2, X } from 'lucide-react'
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { ModelPicker, useRoutedText } from '../components/ModelPicker'
import { Logo } from '../components/Shell'
import { Button, RiskBadge } from '../components/ui'
import { api, type ChatTurn } from '../lib/api'
import { cn } from '../lib/cn'
import { relative, usd } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { canSpeak, speak, useDictation } from '../lib/voice'

const suggestions: TKey[] = ['chat.s1', 'chat.s2', 'chat.s3', 'chat.s4']

// A conversation with Pimpo in the app. Each message is rehearsed like
// any task: reads are real, changes are listed and wait for a confirm.
export function Chat() {
  const t = useT()
  const { id } = useParams()
  const nav = useNavigate()
  const qc = useQueryClient()
  const chats = useQuery({ queryKey: ['chats'], queryFn: api.chats })
  const chat = useQuery({
    queryKey: ['chat', id],
    queryFn: () => api.chat(id!),
    enabled: !!id,
    refetchInterval: (q) => (q.state.data?.turns.some((x) => x.state === 'running' || x.state === 'compiling' || x.done?.state === 'running') ? 1500 : false),
  })
  const assistants = useQuery({ queryKey: ['assistants'], queryFn: api.assistants })
  const [who, setWho] = useState('')
  const [model, setModel] = useState('auto')
  const [picked, setPicked] = useState(false)
  useEffect(() => { if (!picked) setModel(chat.data?.model ?? 'auto') }, [chat.data?.model, picked])
  useEffect(() => { setPicked(false) }, [id])
  const emojiOf = (a?: string) => assistants.data?.find((x) => x.id === a)?.emoji
  const current = assistants.data?.find((x) => x.id === chat.data?.chat.assistant)
  const turns = chat.data?.turns ?? []
  const busy = turns.some((x) => x.state === 'running')
  const refresh = () => { qc.invalidateQueries({ queryKey: ['chat', id] }); qc.invalidateQueries({ queryKey: ['chats'] }) }
  const location = useLocation()
  const [readAloud, setReadAloud] = useState<string>((location.state as { readAloud?: string } | null)?.readAloud ?? '')
  const send = useMutation({
    mutationFn: ({ text }: { text: string; spoken: boolean }) => (id ? api.sendChat(id, text, picked ? model : '') : api.newChat(text, who, model)),
    onSuccess: (r, v) => { if (v.spoken) setReadAloud(r.turn); setPicked(false); if (!id) nav(`/chat/${r.chat}`); refresh() },
  })
  useEffect(() => {
    const done = turns.find((x) => x.id === readAloud && x.state !== 'running')
    if (done) {
      speak(done.summary || done.error || '')
      setReadAloud('')
    }
  }, [turns, readAloud])
  const remove = useMutation({ mutationFn: api.deleteChat, onSuccess: (_, gone) => { if (gone === id) nav('/chat'); qc.invalidateQueries({ queryKey: ['chats'] }) } })
  const end = useRef<HTMLDivElement>(null)
  useEffect(() => { end.current?.scrollIntoView?.({ block: 'end' }) }, [turns.length, turns.at(-1)?.state])

  return (
    <div className="mx-auto flex h-[calc(100dvh-8rem)] max-w-4xl flex-col md:h-[calc(100dvh-7.5rem)]">
      <div className="flex-1 overflow-y-auto pb-4">
        {!id ? (
          (chats.data ?? []).length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center px-4 text-center">
              <Logo size={52} />
              <h1 className="mt-4 text-[22px] font-semibold tracking-tight">{t('chat.hello')}</h1>
              <p className="mt-1 max-w-md text-[13.5px] text-ink-2">{t('chat.helloText')}</p>
              <AssistantPicker value={who} onChange={setWho} className="mt-5 justify-center" />
              <Suggestions onPick={(text) => send.mutate({ text, spoken: false })} disabled={send.isPending} className="mt-6 max-w-xl" />
            </div>
          ) : (
            <section aria-label={t('chat.list')}>
              <div className="mb-4 flex items-end justify-between gap-4">
                <h1 className="text-[22px] font-semibold tracking-tight">{t('chat.list')}</h1>
                <AssistantPicker value={who} onChange={setWho} />
              </div>
              <ul className="divide-y divide-line rounded-2xl border border-line bg-surface">
                {(chats.data ?? []).map((c) => (
                  <li key={c.id} className="group flex items-center gap-3 px-4 py-3">
                    <Link to={`/chat/${c.id}`} className="min-w-0 flex-1">
                      <span className="block truncate text-[14px] font-medium">{emojiOf(c.assistant) && <span className="mr-1.5" aria-hidden>{emojiOf(c.assistant)}</span>}{c.title}</span>
                      <span className="block text-[12px] text-ink-3">{relative(c.updated_at)} · {t('chat.turns', { count: c.turns })}</span>
                    </Link>
                    <button type="button" aria-label={t('chat.delete', { title: c.title })} onClick={() => remove.mutate(c.id)}
                      className="grid size-8 place-items-center rounded-lg text-ink-3 opacity-60 hover:bg-sunken hover:text-ink hover:opacity-100 focus:opacity-100">
                      <Trash2 size={14} />
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          )
        ) : (
          <div className="mx-auto max-w-3xl space-y-6 px-1">
            {current && <p className="text-center text-[12.5px] text-ink-3"><span aria-hidden>{current.emoji}</span> {current.name}</p>}
            {turns.map((x) => <Turn key={x.id} chat={id} turn={x} onChange={refresh} />)}
            <div ref={end} />
          </div>
        )}
      </div>
      {send.error && <p className="mb-2 text-center text-[13px] text-danger">{send.error.message}</p>}
      <Composer disabled={busy || send.isPending} onSend={(text, spoken) => send.mutate({ text, spoken })} model={model} onModel={(m) => { setModel(m); setPicked(true) }} />
    </div>
  )
}

// AssistantPicker chooses who a new conversation is with.
export function AssistantPicker({ value, onChange, className }: { value: string; onChange: (id: string) => void; className?: string }) {
  const t = useT()
  const assistants = useQuery({ queryKey: ['assistants'], queryFn: api.assistants })
  if ((assistants.data?.length ?? 0) === 0) return null
  return (
    <div role="radiogroup" aria-label={t('as.pick')} className={cn('flex flex-wrap gap-1.5', className)}>
      {[{ id: '', name: t('as.default'), emoji: '✨' }, ...assistants.data!].map((a) => (
        <button key={a.id} type="button" role="radio" aria-checked={value === a.id} onClick={() => onChange(a.id)}
          className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', value === a.id ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
          <span aria-hidden className="mr-1">{a.emoji}</span>{a.name}
        </button>
      ))}
    </div>
  )
}

export function Suggestions({ onPick, disabled, className }: { onPick: (text: string) => void; disabled?: boolean; className?: string }) {
  const t = useT()
  return (
    <div className={cn('grid w-full gap-2 sm:grid-cols-2', className)}>
      {suggestions.map((s) => (
        <button key={s} type="button" onClick={() => onPick(t(s))} disabled={disabled}
          className="rounded-xl border border-line bg-surface px-4 py-3 text-left text-[13px] text-ink-2 transition hover:border-line-strong hover:text-ink">
          {t(s)}
        </button>
      ))}
    </div>
  )
}

export function Composer({ disabled, onSend, model, onModel }: { disabled: boolean; onSend: (text: string, spoken: boolean) => void; model?: string; onModel?: (model: string) => void }) {
  const t = useT()
  const [text, setText] = useState('')
  const [spoken, setSpoken] = useState(false)
  const dictation = useDictation((heard, final) => {
    setText(heard)
    setSpoken(true)
    if (final && heard.trim() && !disabled) {
      onSend(heard.trim(), true)
      setText('')
      setSpoken(false)
    }
  })
  const go = () => {
    if (!text.trim() || disabled) return
    onSend(text.trim(), spoken)
    setText('')
    setSpoken(false)
  }
  const key = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      go()
    }
  }
  return (
    <form className="mx-auto w-full max-w-3xl rounded-2xl border border-line bg-surface p-2 shadow-[var(--shadow-card)] focus-within:border-accent"
      onSubmit={(e) => { e.preventDefault(); go() }}>
      <textarea value={text} onChange={(e) => { setText(e.target.value); setSpoken(false) }} onKeyDown={key} rows={1} placeholder={t('chat.placeholder')} aria-label={t('chat.placeholder')}
        className="block max-h-40 min-h-10 w-full resize-none bg-transparent px-2 py-2 text-[14px] outline-none [field-sizing:content]" />
      <div className="flex items-center gap-1">
        {dictation.supported && (
          <button type="button" onClick={() => (dictation.listening ? dictation.stop() : dictation.start())} aria-label={t(dictation.listening ? 'voice.stop' : 'voice.speak')}
            title={dictation.error || undefined}
            className={cn('grid size-8 shrink-0 place-items-center rounded-xl transition', dictation.listening ? 'animate-pulse-soft bg-danger text-white' : dictation.error ? 'text-danger hover:bg-sunken' : 'text-ink-3 hover:bg-sunken hover:text-ink')}>
            {dictation.listening ? <Square size={14} /> : <Mic size={16} />}
          </button>
        )}
        {onModel && <ModelPicker value={model ?? 'auto'} onChange={onModel} />}
        <div className="flex-1" />
        <button type="submit" aria-label={t('chat.send')} disabled={!text.trim() || disabled}
          className="grid size-8 shrink-0 place-items-center rounded-xl bg-ink text-bg transition disabled:opacity-30">
          {disabled ? <Loader2 size={16} className="animate-spin" /> : <ArrowUp size={16} />}
        </button>
      </div>
    </form>
  )
}

function Turn({ chat, turn: x, onChange }: { chat: string; turn: ChatTurn; onChange: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const act = useMutation({ mutationFn: () => api.chatDo(chat, x.id), onSuccess: onChange })
  const compile = useMutation({ mutationFn: () => api.compile(x.id), onSuccess: () => { onChange(); qc.invalidateQueries({ queryKey: ['routines'] }) } })
  const failed = x.done?.results.filter((r) => !r.ok) ?? []
  const routed = useRoutedText()(x.model)
  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-md bg-ink px-4 py-2.5 text-[14px] text-bg">{x.request}</div>
      </div>
      <div className="flex gap-3">
        <div className="mt-0.5 shrink-0"><Logo size={26} /></div>
        <div className="min-w-0 flex-1 space-y-3">
          {x.state === 'running' ? (
            <p className="flex items-center gap-2 text-[13.5px] text-ink-2">
              <Loader2 size={15} className="animate-spin" /> {t('chat.working')}
              <Link to={`/explorations/${x.id}`} className="text-[12.5px] text-ink-3 underline">{t('chat.steps')}</Link>
            </p>
          ) : x.state === 'failed' ? (
            <p className="text-[14px] text-danger">{t('chat.failed')} {x.error}</p>
          ) : (
            <div className="whitespace-pre-wrap text-[14px] leading-relaxed text-ink">{x.summary}</div>
          )}

          {x.state !== 'running' && x.actions.length > 0 && (
            <div className="rounded-xl border border-line bg-surface p-3">
              <div className="mb-2 text-[12.5px] font-medium text-ink-2">{t('chat.wouldDo')}</div>
              <ul className="space-y-1.5">
                {x.actions.map((a, i) => (
                  <li key={i} className="flex items-start justify-between gap-3 text-[13px]">
                    <span className="min-w-0">{a.text}</span>
                    <RiskBadge risk={a.risk} />
                  </li>
                ))}
              </ul>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                {!x.done && <Button size="sm" variant="primary" onClick={() => act.mutate()} disabled={act.isPending}><Check size={14} /> {t('chat.confirm')}</Button>}
                {x.done?.state === 'running' && <span className="flex items-center gap-1.5 text-[12.5px] text-ink-2"><Loader2 size={13} className="animate-spin" /> {t('chat.doing')}</span>}
                {x.done?.state === 'done' && <span className="flex items-center gap-1 text-[12.5px] text-read"><Check size={13} /> {t('chat.didIt')}</span>}
                {x.done?.state === 'failed' && (
                  <span className="text-[12.5px] text-danger"><X size={13} className="mr-1 inline" />{t('chat.didFail')} {failed.map((f) => f.error).join('; ')}</span>
                )}
              </div>
              {act.error && <p className="mt-2 text-[12.5px] text-danger">{act.error.message}</p>}
            </div>
          )}

          {x.state !== 'running' && x.state !== 'failed' && (
            <div className="flex flex-wrap items-center gap-2 text-[12px] text-ink-3">
              {x.routine ? (
                <span className="text-read">{t('chat.routineMade', { name: x.routine })} <Link to={`/routines/${x.routine}`} className="underline">{t('chat.openRoutine')}</Link></span>
              ) : x.state === 'ready' && (
                <Button size="sm" variant="ghost" onClick={() => compile.mutate()} disabled={compile.isPending}>
                  {compile.isPending ? <Loader2 size={13} className="animate-spin" /> : <Repeat size={13} />} {t('chat.routine')}
                </Button>
              )}
              {x.state === 'compiling' && <Loader2 size={13} className="animate-spin" />}
              {canSpeak() && x.summary && (
                <Button size="sm" variant="ghost" onClick={() => speak(x.summary!)} aria-label={t('voice.listen')}><Volume2 size={13} /> {t('voice.listen')}</Button>
              )}
              <span>{t('chat.cost', { cost: usd(x.cost_usd) })}</span>
              {routed && <span title={x.model?.by === 'jev' ? t('mp.weighedJev') : x.model?.by === 'rules' ? t('mp.weighedRules') : undefined}>· {routed}</span>}
              {compile.error && <span className="text-danger">{compile.error.message}</span>}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
