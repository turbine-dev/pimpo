import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Download, HardDriveDownload, Loader2, Play, Trash2, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { api, type LocalItem, type LocalJob } from '../lib/api'
import { cn } from '../lib/cn'
import { size } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card } from './ui'

const field = 'h-9 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const active = (j: LocalJob) => j.state === 'downloading' || j.state === 'verifying' || j.state === 'unpacking'

type Ask = { title: string; bytes: number; go: () => void }

// LocalModels downloads models that run on this computer (voices to read
// aloud, and language models through Ollama) and shows each download as it
// happens.
export function LocalModels() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['local'], queryFn: api.local, refetchInterval: (s) => (s.state.data?.jobs.some(active) ? 1000 : 15_000) })
  const refresh = () => qc.invalidateQueries({ queryKey: ['local'] })
  const install = useMutation({ mutationFn: api.installLocal, onSuccess: refresh })
  const remove = useMutation({ mutationFn: api.removeLocal, onSuccess: refresh })
  const pull = useMutation({ mutationFn: api.pullOllama, onSuccess: () => { refresh(); qc.invalidateQueries({ queryKey: ['models', 'detect'] }) } })
  const drop = useMutation({ mutationFn: api.removeOllama, onSuccess: () => { refresh(); qc.invalidateQueries({ queryKey: ['models', 'detect'] }) } })
  const cancel = useMutation({ mutationFn: api.cancelDownload, onSuccess: refresh })
  const [ask, setAsk] = useState<Ask | null>(null)
  const [other, setOther] = useState('')
  const d = q.data
  if (!d) return null
  const jobs = d.jobs
  const jobFor = (item: string) => jobs.find((j) => j.item === item && active(j))
  const engineNeeded = d.engine && !d.engine.installed ? d.engine.size : 0
  const multi = d.voices.filter((v) => (v.languages?.length ?? 0) > 1)
  const single = d.voices.filter((v) => (v.languages?.length ?? 0) === 1)
  const groups = [...(multi.length ? [{ key: 'multi', voices: multi }] : []),
    ...[...new Set(single.map((v) => v.languages![0]))].map((l) => ({ key: l, voices: single.filter((v) => v.languages![0] === l) }))]
  const installedModels = d.ollama.models ?? []
  const fits = d.suggestions.filter((s) => !d.memory || s.min_ram <= d.memory)
  const error = install.error || pull.error || remove.error || drop.error

  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><HardDriveDownload size={16} /> {t('lm.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('lm.text', { free: d.free >= 0 ? size(d.free) : '?' })}</p>

      {jobs.length > 0 && (
        <ul className="mb-5 space-y-2" aria-label={t('lm.downloads')}>
          {jobs.slice(0, 6).map((j) => <JobRow key={j.id} j={j} onCancel={() => cancel.mutate(j.id)} />)}
        </ul>
      )}

      <div className="mb-2 text-[13.5px] font-medium">{t('lm.voices')}</div>
      <p className="mb-2 text-[12.5px] text-ink-3">{t('lm.voicesText')}{engineNeeded > 0 && <> {t('lm.engineFirst', { size: size(engineNeeded) })}</>}</p>
      {!d.engine ? <p className="mb-4 text-[12.5px] text-ink-3">{t('lm.noEngine')}</p> : (
        <div className="mb-5 space-y-3">
          {groups.map((g) => (
            <div key={g.key}>
              <div className="mb-1 flex items-center justify-between gap-2">
                <span className="text-[12px] font-medium uppercase tracking-wide text-ink-3">{g.key === 'multi' ? t('lm.multi') : g.key}</span>
                {g.key !== 'multi' && <Sample language={g.key} />}
              </div>
              <ul className="divide-y divide-line rounded-xl border border-line">
                {g.voices.map((v) => (
                  <VoiceRow key={v.id} v={v} job={jobFor(v.id)} busy={install.isPending}
                    onGet={() => setAsk({ title: v.name, bytes: v.size + engineNeeded, go: () => install.mutate(v.id) })}
                    onRemove={() => remove.mutate(v.id)} />
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      {d.engine && (d.transcribers?.length ?? 0) > 0 && (
        <>
          <div className="mb-2 text-[13.5px] font-medium">{t('lm.transcribers')}</div>
          <p className="mb-2 text-[12.5px] text-ink-3">{t('lm.transcribersText')}{engineNeeded > 0 && <> {t('lm.engineFirst', { size: size(engineNeeded) })}</>}</p>
          <ul className="mb-5 divide-y divide-line rounded-xl border border-line">
            {d.transcribers!.map((v) => (
              <VoiceRow key={v.id} v={v} job={jobFor(v.id)} busy={install.isPending}
                onGet={() => setAsk({ title: v.name, bytes: v.size + engineNeeded, go: () => install.mutate(v.id) })}
                onRemove={() => remove.mutate(v.id)} />
            ))}
          </ul>
        </>
      )}

      <div className="mb-2 text-[13.5px] font-medium">{t('lm.llms')}</div>
      {!d.ollama.up ? (
        <p className="text-[12.5px] text-ink-3">{t('lm.noOllama', { url: d.ollama.url })} <a className="underline" href="https://ollama.com/download" target="_blank" rel="noreferrer">ollama.com</a></p>
      ) : (
        <>
          <p className="mb-2 text-[12.5px] text-ink-3">{t('lm.llmsText', { memory: d.memory ? size(d.memory) : '?' })}</p>
          {installedModels.length > 0 && (
            <ul className="mb-3 divide-y divide-line rounded-xl border border-line">
              {installedModels.map((m) => (
                <li key={m.id} className="flex items-center gap-3 px-3 py-2 text-[13px]">
                  <Check size={14} className="text-read" /><code className="min-w-0 flex-1 truncate font-mono text-[12.5px]">{m.id}</code>
                  <Button size="sm" variant="ghost" aria-label={t('lm.remove', { name: m.id })} onClick={() => drop.mutate(m.id)}><Trash2 size={13} /></Button>
                </li>
              ))}
            </ul>
          )}
          <ul className="divide-y divide-line rounded-xl border border-line">
            {fits.filter((s) => !installedModels.some((m) => m.id === s.model)).map((s) => {
              const job = jobFor('ollama:' + s.model)
              return (
                <li key={s.model} className="px-3 py-2 text-[13px]">
                  <div className="flex items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <code className="font-mono text-[12.5px]">{s.model}</code>
                      <span className="ml-2 text-[12px] text-ink-3">{s.about}</span>
                    </div>
                    <span className="text-[12px] tabular-nums text-ink-3">{size(s.size)}</span>
                    {!job && <Button size="sm" onClick={() => setAsk({ title: s.model, bytes: s.size, go: () => pull.mutate(s.model) })}><Download size={13} /> {t('lm.get')}</Button>}
                  </div>
                  {job && <Progress j={job} />}
                </li>
              )
            })}
          </ul>
          <form className="mt-3 flex gap-2" onSubmit={(e) => { e.preventDefault(); if (other.trim()) setAsk({ title: other.trim(), bytes: 0, go: () => { pull.mutate(other.trim()); setOther('') } }) }}>
            <input className={cn(field, 'min-w-0 flex-1')} value={other} onChange={(e) => setOther(e.target.value)} placeholder={t('lm.otherPlaceholder')} aria-label={t('lm.other')} />
            <Button type="submit" size="sm" disabled={!other.trim()}><Download size={13} /> {t('lm.get')}</Button>
          </form>
          <p className="mt-2 text-[12px] text-ink-3">{t('lm.useIt')}</p>
        </>
      )}
      {error && <p className="mt-3 text-[12.5px] text-danger">{error.message}</p>}
      {ask && <Confirm ask={ask} free={d.free} onClose={() => setAsk(null)} />}
    </Card>
  )
}

function VoiceRow({ v, job, busy, onGet, onRemove }: { v: LocalItem; job?: LocalJob; busy: boolean; onGet: () => void; onRemove: () => void }) {
  const t = useT()
  return (
    <li className="px-3 py-2 text-[13px]">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <span className="font-medium">{v.name}</span>
          <span className="ml-2 text-[12px] text-ink-3">{v.about}{(v.languages?.length ?? 0) > 1 && ` · ${v.languages!.join(', ')}`}</span>
        </div>
        <span className="text-[12px] tabular-nums text-ink-3">{size(v.size)}</span>
        {v.installed ? (
          <>
            <span className="flex items-center gap-1 text-[12px] text-read"><Check size={13} /> {t('lm.installed')}</span>
            <Button size="sm" variant="ghost" aria-label={t('lm.remove', { name: v.name })} onClick={onRemove}><Trash2 size={13} /></Button>
          </>
        ) : !job && <Button size="sm" disabled={busy} onClick={onGet}><Download size={13} /> {t('lm.get')}</Button>}
      </div>
      {job && <Progress j={job} />}
    </li>
  )
}

// Sample reads a sentence in a language with the voice Pimpo would use.
export function Sample({ language, forWhat = 'routines' }: { language: string; forWhat?: 'chat' | 'routines' }) {
  const t = useT()
  const audio = useRef<HTMLAudioElement>(null)
  const [voice, setVoice] = useState('')
  const play = useMutation({
    mutationFn: () => api.voiceSample(language, forWhat),
    onSuccess: (r) => { setVoice(r.voice); if (audio.current) { audio.current.src = `/api/media/${r.id}`; audio.current.play().catch(() => {}) } },
  })
  return (
    <span className="flex items-center gap-2 text-[12px] text-ink-3">
      {voice && <span>{voice === 'system' ? t('lm.systemVoice') : voice}</span>}
      <button type="button" className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 hover:bg-sunken hover:text-ink" onClick={() => play.mutate()} disabled={play.isPending}>
        {play.isPending ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />} {t('lm.listen')}
      </button>
      {play.error && <span className="text-danger">{play.error.message}</span>}
      <audio ref={audio} className="hidden" />
    </span>
  )
}

function Progress({ j }: { j: LocalJob }) {
  const t = useT()
  const pct = j.total > 0 ? Math.min(100, (j.done / j.total) * 100) : 0
  return (
    <div className="mt-2">
      <div className="h-1.5 overflow-hidden rounded-full bg-sunken" role="progressbar" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label={j.name}>
        <div className={cn('h-full rounded-full bg-accent transition-[width]', j.total === 0 && 'w-1/3 animate-pulse')} style={j.total > 0 ? { width: `${pct}%` } : undefined} />
      </div>
      <div className="mt-1 flex justify-between text-[11.5px] tabular-nums text-ink-3">
        <span>{t(`lm.state.${j.state}` as TKey)}{j.detail && j.state === 'downloading' ? ` · ${j.detail}` : ''}</span>
        <span>{j.total > 0 ? `${size(j.done)} / ${size(j.total)} · ${Math.round(pct)}%` : ''}</span>
      </div>
    </div>
  )
}

function JobRow({ j, onCancel }: { j: LocalJob; onCancel: () => void }) {
  const t = useT()
  return (
    <li className="rounded-xl border border-line px-3 py-2 text-[13px]">
      <div className="flex items-center gap-2">
        {active(j) ? <Loader2 size={14} className="animate-spin text-ink-3" /> : j.state === 'done' ? <Check size={14} className="text-read" /> : <X size={14} className="text-danger" />}
        <span className="min-w-0 flex-1 truncate font-medium">{j.name}</span>
        {active(j) && <Button size="sm" variant="ghost" onClick={onCancel}>{t('lm.cancel')}</Button>}
      </div>
      {active(j) ? <Progress j={j} /> : (
        <div className={cn('mt-0.5 text-[12px]', j.state === 'failed' ? 'text-danger' : 'text-ink-3')}>{t(`lm.state.${j.state}` as TKey)}{j.error && `: ${j.error}`}</div>
      )}
    </li>
  )
}

function Confirm({ ask, free, onClose }: { ask: Ask; free: number; onClose: () => void }) {
  const t = useT()
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-1/3 z-50 w-[min(420px,calc(100vw-24px))] -translate-x-1/2 rounded-2xl border border-line bg-surface p-5 shadow-[var(--shadow-pop)] focus:outline-none">
          <Dialog.Title className="text-[16px] font-semibold">{t('lm.confirmTitle', { name: ask.title })}</Dialog.Title>
          <Dialog.Description className="mt-1 text-[13px] text-ink-2">
            {ask.bytes > 0 ? t('lm.confirmText', { size: size(ask.bytes), free: free >= 0 ? size(free) : '?' }) : t('lm.confirmUnknown')}
          </Dialog.Description>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={() => { ask.go(); onClose() }}><Download size={14} /> {t('lm.get')}</Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
