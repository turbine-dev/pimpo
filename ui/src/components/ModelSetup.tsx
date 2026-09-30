import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, AudioLines, Brain, Check, ChevronRight, Cpu, ExternalLink, HardDrive, Layers, Loader2, Plus, RefreshCw, Search, Sparkles, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, EFFORTS, type CatalogModel, type Effort, type Job, type ModelOption, type ModelTest, type Provider, type Settings } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { fill, useT, type TKey } from '../lib/i18n'
import { LocalModels, Sample } from './LocalModels'
import { Button, Card, Switch } from './ui'

const field = 'h-9 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const claudeCode = ['sonnet', 'opus', 'haiku']
const jobs: { job: Job; key: 'explore_model' | 'compile_model' | 'judge_model'; label: TKey; text: TKey }[] = [
  { job: 'explore', key: 'explore_model', label: 'models.explore', text: 'ms.exploreText' },
  { job: 'compile', key: 'compile_model', label: 'models.compile', text: 'ms.compileText' },
  { job: 'judge', key: 'judge_model', label: 'ms.judge', text: 'ms.judgeText' },
]

// EffortSelect picks how hard a model thinks; "" keeps the default, which
// is the job's level when there is one and the model's own otherwise.
export function EffortSelect({ value, onChange, fallback, className }: { value: string; onChange: (e: string) => void; fallback?: string; className?: string }) {
  const t = useT()
  const name = (e: string) => { const n = t(`effort.${e}` as TKey); return n.charAt(0).toUpperCase() + n.slice(1) }
  return (
    <label className={cn('relative block', className)} title={t('mp.effort')}>
      <Brain size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-3" />
      <select className={cn(field, 'w-full pl-8')} value={value} aria-label={t('mp.effort')} onChange={(e) => onChange(e.target.value)}>
        <option value="">{fallback ? `${t('ms.effortDefault')} (${t(`effort.${fallback}` as TKey)})` : t('ms.effortDefault')}</option>
        {EFFORTS.map((e) => <option key={e} value={e}>{name(e)}</option>)}
      </select>
    </label>
  )
}

export const label = (id: string) => (claudeCode.includes(id) ? `Claude Code · ${id}` : id === 'codex' ? 'Codex · ChatGPT' : id.startsWith('opencode:') ? `opencode · ${id.slice(9)}` : id)
const price = (t: ReturnType<typeof useT>, m: { price_in: number; price_out: number; free?: boolean }) =>
  m.free || (m.price_in === 0 && m.price_out === 0) ? t('models.free') : t('models.price', { in: m.price_in, out: m.price_out })

// useModelOptions lists the models the owner can choose from: Claude Code
// when installed, Codex when signed in, and their own tested models.
export function useModelOptions() {
  const s = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const found = useQuery({ queryKey: ['models', 'detect'], queryFn: api.detectModels, staleTime: 60_000 })
  const options = [...(found.data?.claude_code || info.data?.claude_code ? claudeCode : []), ...(found.data?.codex_login ? ['codex'] : []), ...(s.data?.models ?? []).map((m) => m.id)]
  return { options, auto: info.data?.auto, settings: s.data }
}

// useRetired is the set of the house's models their providers stopped
// offering.
export function useRetired() {
  const q = useQuery({ queryKey: ['models', 'retired'], queryFn: api.retiredModels, staleTime: 60_000 })
  return new Set(q.data?.retired ?? [])
}

// RetiredHint suggests switching when the chosen model is gone from its
// provider; it shows nothing otherwise.
export function RetiredHint({ model, className }: { model?: string; className?: string }) {
  const t = useT()
  const retired = useRetired()
  if (!model || !retired.has(model)) return null
  return (
    <span role="status" className={cn('flex items-start gap-1.5 text-[12px] text-danger', className)}>
      <AlertTriangle size={13} className="mt-0.5 shrink-0" /> {t('ms.retiredHint', { model: label(model) })}
    </span>
  )
}

// Marks says whether a catalog model is new or retired, and when new
// without a price, that the owner must give one.
function Marks({ m }: { m: CatalogModel }) {
  const t = useT()
  if (m.retired) return <span className="rounded-full bg-danger-soft px-1.5 text-[10.5px] font-medium text-danger">{t('ms.retired')}</span>
  if (m.new) return <span className="rounded-full bg-explore-soft px-1.5 text-[10.5px] font-medium text-explore">{t(m.priced || m.free ? 'ms.new' : 'ms.newUnpriced')}</span>
  return null
}

// useSettings saves each change at once, on top of the latest saved settings.
function useSettings() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const save = useMutation({
    mutationFn: (change: (s: Settings) => Settings) => api.saveSettings(change(qc.getQueryData<Settings>(['settings']) ?? q.data!)),
    onSuccess: (s) => qc.setQueryData(['settings'], s),
  })
  return { s: q.data, save }
}

// ModelSetup is where the owner picks which model does each job: Claude
// Code with their subscription, a model on this computer, or a provider's
// model with its price, tested before it is used.
export function ModelSetup() {
  const t = useT()
  const { s, save } = useSettings()
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const found = useQuery({ queryKey: ['models', 'detect'], queryFn: api.detectModels })
  const [open, setOpen] = useState<{ provider: Provider; local?: CatalogModel[] } | null>(null)
  const [oc, setOc] = useState(false)
  if (!s || !info.data) return <Card className="p-5 text-sm text-ink-3">{t('ui.loading')}</Card>
  const mine = s.models ?? []
  const options = [...(found.data?.claude_code || info.data.claude_code ? claudeCode : []), ...(found.data?.codex_login ? ['codex'] : []), ...mine.map((m) => m.id)]
  const providers = info.data.providers.filter((p) => !p.local)
  const local = (id: 'ollama' | 'lmstudio') => info.data.providers.find((p) => p.id === id)!

  return (
    <div className="space-y-4">
      <Card className="p-5">
        <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Cpu size={16} /> {t('ms.inUse')}</div>
        <p className="mb-4 text-[13px] text-ink-3">{t('ms.inUseText')}</p>
        <div className="space-y-4">
          {jobs.map(({ job, key, label: l, text }) => {
            const fallbacks = s.fallbacks?.[job] ?? []
            const spare = options.filter((o) => o !== s[key] && !fallbacks.includes(o))
            return (
              <div key={job} className="grid gap-2 sm:grid-cols-[1fr_1.4fr] sm:items-start">
                <div>
                  <div className="text-[13.5px] font-medium">{t(l)}</div>
                  <div className="text-[12px] text-ink-3">{t(text)}</div>
                </div>
                <div className="space-y-2">
                  <div className="flex gap-2">
                    <select className={cn(field, 'min-w-0 flex-1')} value={s[key]} aria-label={t(l)} onChange={(e) => save.mutate((x) => ({ ...x, [key]: e.target.value }))}>
                      {!options.includes(s[key]) && <option value={s[key]}>{label(s[key])}</option>}
                      {options.map((o) => <option key={o} value={o}>{label(o)}</option>)}
                    </select>
                    <EffortSelect value={s.efforts?.[job] ?? ''} className="w-[124px] shrink-0"
                      onChange={(e) => save.mutate((x) => { const ef = { ...x.efforts }; if (e) ef[job] = e as Effort; else delete ef[job]; return { ...x, efforts: ef } })} />
                  </div>
                  <RetiredHint model={s[key]} />
                  <div className="flex flex-wrap items-center gap-1.5 text-[12px]">
                    <span className="text-ink-3">{t('ms.fallbacks')}</span>
                    {fallbacks.length === 0 && <span className="text-ink-3">{t('ms.noFallback')}</span>}
                    {fallbacks.map((f, i) => (
                      <span key={f} className="inline-flex items-center gap-1 rounded-full bg-sunken px-2 py-0.5">
                        {i + 1}. {label(f)}
                        <button type="button" aria-label={t('ms.removeFallback', { model: f })} className="text-ink-3 hover:text-ink"
                          onClick={() => save.mutate((x) => ({ ...x, fallbacks: { ...x.fallbacks, [job]: fallbacks.filter((y) => y !== f) } }))}><X size={11} /></button>
                      </span>
                    ))}
                    {spare.length > 0 && fallbacks.length < 4 && (
                      <select className="h-7 rounded-full border border-dashed border-line-strong bg-transparent px-2 text-[12px]" value="" aria-label={t('ms.addFallback')}
                        onChange={(e) => e.target.value && save.mutate((x) => ({ ...x, fallbacks: { ...x.fallbacks, [job]: [...fallbacks, e.target.value] } }))}>
                        <option value="">+ {t('ms.addFallback')}</option>
                        {spare.map((o) => <option key={o} value={o}>{label(o)}</option>)}
                      </select>
                    )}
                  </div>
                </div>
              </div>
            )
          })}
        </div>
        {save.error && <p className="mt-3 text-[12.5px] text-danger">{save.error.message}</p>}
      </Card>

      <AutoChoice s={s} options={options} save={save.mutate} />

      <CompactChoice s={s} save={save.mutate} />

      <Card className="p-5">
        <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><HardDrive size={16} /> {t('ms.here')}</div>
        <p className="mb-3 text-[13px] text-ink-3">{t('ms.hereText')}</p>
        <ul className="divide-y divide-line rounded-xl border border-line">
          <li className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">Claude Code</div>
              <div className="text-[12.5px] text-ink-3">{found.data?.claude_code ? t('ms.claudeFound', { version: found.data.claude_code }) : found.isLoading ? t('ui.loading') : t('ms.claudeMissing')}</div>
            </div>
            {found.data?.claude_code ? <Check size={16} className="text-read" aria-label={t('ms.detected')} />
              : <a className="text-[12.5px] underline" href="https://docs.anthropic.com/en/docs/claude-code/setup" target="_blank" rel="noreferrer">{t('ms.install')}</a>}
          </li>
          <li className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">Codex · ChatGPT</div>
              <div className="text-[12.5px] text-ink-3">{found.isLoading ? t('ui.loading') : found.data?.codex_login ? t('ms.codexFound') : found.data?.codex ? t('ms.codexNoLogin') : t('ms.codexMissing')}</div>
            </div>
            {found.data?.codex_login && <Button size="sm" onClick={() => save.mutate((x) => ({ ...x, explore_model: 'codex', compile_model: 'codex' }))}>{t('ms.useTasks')}</Button>}
          </li>
          {(['ollama', 'lmstudio'] as const).map((id) => {
            const list = found.data?.[id] ?? []
            const url = id === 'ollama' ? found.data?.ollama_url : found.data?.lmstudio_url
            const up = id === 'ollama' ? found.data?.ollama_up : found.data?.lmstudio_up
            return (
              <li key={id} className="flex items-center gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <div className="text-[14px] font-medium">{local(id).name}</div>
                  <div className="text-[12.5px] text-ink-3">
                    {found.isLoading ? t('ui.loading') : list.length > 0 ? t('ms.localModels', { count: list.length, url: url ?? '' })
                      : up ? (id === 'ollama' ? fill(t('ms.ollamaEmpty', { url: url ?? '' }), { cmd: <code className="rounded bg-sunken px-1">ollama pull qwen3:4b</code> }) : t('ms.lmEmpty', { url: url ?? '' }))
                      : t('ms.localMissing', { url: url ?? '' })}
                  </div>
                </div>
                {list.length > 0 && <Button size="sm" onClick={() => setOpen({ provider: local(id), local: list })}>{t('ms.choose')} <ChevronRight size={14} /></Button>}
              </li>
            )
          })}
          {found.data?.opencode && (
            <li className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="text-[14px] font-medium">opencode</div>
                <div className="text-[12.5px] text-ink-3">{t('ms.ocText')}</div>
              </div>
              <Button size="sm" onClick={() => setOc(true)}>{t('ms.choose')} <ChevronRight size={14} /></Button>
            </li>
          )}
          {found.data?.qwen_code && (
            <li className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="text-[14px] font-medium">Qwen Code</div>
                <div className="text-[12.5px] text-ink-3">{t('ms.qwenCodeText')}</div>
              </div>
              {info.data.providers.some((p) => p.id === 'dashscope') && (
                <Button size="sm" onClick={() => setOpen({ provider: info.data.providers.find((p) => p.id === 'dashscope')! })}>{t('ms.connectQwen')}</Button>
              )}
            </li>
          )}
        </ul>
        {(found.data?.apps?.length ?? 0) > 0 && <p className="mt-3 text-[12.5px] text-ink-3">{t('ms.apps', { list: found.data!.apps!.join(', ') })}</p>}
        <LocalAddresses s={s} save={save.mutate} />
      </Card>

      <VoiceChoice s={s} save={save.mutate} />

      <LocalModels />

      <Card className="p-5">
        <div className="mb-1 text-[15px] font-medium">{t('ms.providers')}</div>
        <p className="mb-3 text-[13px] text-ink-3">{t('ms.providersText')}</p>
        <div className="grid gap-2 sm:grid-cols-2">
          {providers.map((p) => {
            const connected = p.id === 'custom' ? !!s.custom_url : info.data.keys[p.id]
            const count = mine.filter((m) => m.id.startsWith(p.id + ':')).length
            return (
              <button key={p.id} type="button" onClick={() => setOpen({ provider: p })}
                className="flex items-center gap-3 rounded-xl border border-line px-4 py-3 text-left transition hover:border-line-strong hover:bg-sunken/50">
                <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-sunken text-[13px] font-semibold">{p.name.slice(0, 2)}</span>
                <span className="min-w-0 flex-1">
                  <span className="block text-[14px] font-medium">{p.id === 'custom' ? t('ms.custom') : p.name}</span>
                  <span className="block text-[12px] text-ink-3">{connected ? (count ? t('ms.inList', { count }) : t('ms.connected')) : t('ms.connect')}</span>
                </span>
                {connected ? <Check size={15} className="text-read" /> : <ChevronRight size={15} className="text-ink-3" />}
              </button>
            )
          })}
        </div>
      </Card>

      {mine.length > 0 && <Mine s={s} save={save.mutate} />}
      {oc && <OpencodePicker s={s} save={save.mutate} onClose={() => setOc(false)} />}
      {open && <Picker provider={open.provider} local={open.local} onClose={() => setOpen(null)} save={save.mutate} s={s} />}
    </div>
  )
}

// AutoChoice is how Pimpo picks a model for each chat request: quick ones
// go to a light model, heavy ones to a strong one, the rest stay on the
// tasks model.
function AutoChoice({ s, options, save }: { s: Settings; options: string[]; save: (c: (s: Settings) => Settings) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const auto = info.data?.auto
  const on = !s.auto_off
  const change = (c: (s: Settings) => Settings) => { save(c); setTimeout(() => qc.invalidateQueries({ queryKey: ['models'] }), 300) }
  const pick = (key: 'auto_light' | 'auto_strong', derived: string | undefined, l: TKey, text: TKey) => (
    <div className="grid gap-2 sm:grid-cols-[1fr_1.4fr] sm:items-start">
      <div>
        <div className="text-[13.5px] font-medium">{t(l)}</div>
        <div className="text-[12px] text-ink-3">{t(text)}</div>
      </div>
      <select className={cn(field, 'w-full')} value={s[key] ?? ''} aria-label={t(l)} onChange={(e) => change((x) => ({ ...x, [key]: e.target.value }))}>
        <option value="">{derived ? t('ms.autoDerived', { model: label(derived) }) : t('ms.autoNone')}</option>
        {options.filter((o) => o !== s.explore_model).map((o) => <option key={o} value={o}>{label(o)}</option>)}
      </select>
    </div>
  )
  return (
    <Card className="p-5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Sparkles size={16} /> {t('ms.auto')}</div>
          <p className="text-[13px] text-ink-3">{t('ms.autoText', { model: label(s.explore_model) })}</p>
        </div>
        <Switch on={on} label={t('ms.auto')} onChange={() => change((x) => ({ ...x, auto_off: on }))} className="mt-0.5" />
      </div>
      {on && (
        <div className="mt-4 space-y-4">
          {pick('auto_light', s.auto_light ? undefined : auto?.light, 'ms.autoLight', 'ms.autoLightText')}
          {pick('auto_strong', s.auto_strong ? undefined : auto?.strong, 'ms.autoStrong', 'ms.autoStrongText')}
          <p className="text-[12px] text-ink-3">{t('ms.autoEffort')} {t(auto?.weigher === 'jev' ? 'ms.autoByJev' : 'ms.autoByRules')} {t('ms.autoBudget')}</p>
        </div>
      )}
    </Card>
  )
}

// CompactChoice is Anthropic's server-side summary of long conversations:
// once a chat or job reaches the size set, the older turns are summarized
// by Anthropic so the conversation keeps going; the summary is counted in
// the cost.
function CompactChoice({ s, save }: { s: Settings; save: (c: (s: Settings) => Settings) => void }) {
  const t = useT()
  const on = !s.compact_off
  const [at, setAt] = useState(String(s.compact_at || 150000))
  return (
    <Card className="p-5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Layers size={16} /> {t('ms.compact')}</div>
          <p className="text-[13px] text-ink-3">{t('ms.compactText')}</p>
        </div>
        <Switch on={on} label={t('ms.compact')} onChange={() => save((x) => ({ ...x, compact_off: on }))} className="mt-0.5" />
      </div>
      {on && (
        <form className="mt-3 flex flex-wrap items-center gap-2 text-[13px]" onSubmit={(e) => { e.preventDefault(); const n = Math.round(Number(at)); if (n >= 50000) save((x) => ({ ...x, compact_at: n === 150000 ? 0 : n })) }}>
          <label htmlFor="ms-compact-at" className="text-ink-2">{t('ms.compactAt')}</label>
          <input id="ms-compact-at" type="number" min={50000} max={1000000} step={10000} className={cn(field, 'w-32')} value={at} onChange={(e) => setAt(e.target.value)} />
          <Button size="sm" type="submit" disabled={Number(at) < 50000 || Number(at) > 1000000}>{t('common.save')}</Button>
        </form>
      )}
    </Card>
  )
}

// VoiceChoice picks the default voices: for routines' audio (like the
// podcast) and for listening to answers in the chat.
function VoiceChoice({ s, save }: { s: Settings; save: (c: (s: Settings) => Settings) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const v = useQuery({ queryKey: ['voice', s.voice, s.chat_voice], queryFn: api.voice })
  const [key, setKey] = useState('')
  const setEleven = useMutation({ mutationFn: api.setElevenLabsKey, onSuccess: () => { setKey(''); qc.invalidateQueries({ queryKey: ['voice'] }) } })
  const lang = s.locale || 'pt-BR'
  const set = (patch: Partial<Settings>) => save((x) => ({ ...x, ...patch }))
  const uses = (e?: string) => (e || 'auto')
  const eleven = uses(s.voice) === 'elevenlabs' || s.chat_voice === 'elevenlabs'
  const openai = uses(s.voice) === 'openai' || s.chat_voice === 'openai'
  const row = (who: 'routines' | 'chat') => {
    const engine = who === 'routines' ? uses(s.voice) : (s.chat_voice ?? '')
    const model = who === 'routines' ? s.voice_model : s.chat_voice_model
    const name = who === 'routines' ? s.voice_name : s.chat_voice_name
    const patch = (e: string, m = '', n = '') => set(who === 'routines' ? { voice: e as Settings['voice'], voice_model: m, voice_name: n } : { chat_voice: e, chat_voice_model: m, chat_voice_name: n })
    const engines = who === 'routines' ? ['auto', 'local', 'system', 'openai', 'elevenlabs'] : ['', 'browser', 'auto', 'local', 'system', 'openai', 'elevenlabs']
    return (
      <div className="grid gap-2 sm:grid-cols-[1fr_2fr] sm:items-center">
        <div>
          <div className="text-[13.5px] font-medium">{t(who === 'routines' ? 'vc.routines' : 'vc.chat')}</div>
          <div className="text-[12px] text-ink-3">{t(who === 'routines' ? 'vc.routinesText' : 'vc.chatText')}</div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <select className={cn(field, 'min-w-[200px]')} value={engine} aria-label={t(who === 'routines' ? 'vc.routines' : 'vc.chat')} onChange={(e) => patch(e.target.value)}>
            {engines.map((k) => <option key={k} value={k}>{t(`vc.engine.${k || 'same'}` as TKey)}</option>)}
          </select>
          {engine === 'openai' && (
            <>
              <select className={field} value={model || 'tts-1'} aria-label={t('vc.model')} onChange={(e) => patch(engine, e.target.value, name)}>
                {Object.entries(v.data?.openai_prices ?? { 'tts-1': 15, 'tts-1-hd': 30 }).map(([m, p]) => <option key={m} value={m}>{t('vc.openaiModel', { model: m, price: (p / 1000).toFixed(3) })}</option>)}
              </select>
              <select className={field} value={name || 'nova'} aria-label={t('vc.voice')} onChange={(e) => patch(engine, model, e.target.value)}>
                {(v.data?.openai_voices ?? ['nova']).map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
            </>
          )}
          {engine === 'elevenlabs' && (v.data?.elevenlabs_voices?.length ?? 0) > 0 && (
            <select className={field} value={name || ''} aria-label={t('vc.voice')} onChange={(e) => patch(engine, model, e.target.value)}>
              <option value="">{t('vc.pickVoice')}</option>
              {v.data!.elevenlabs_voices!.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
            </select>
          )}
          {engine !== 'browser' && <Sample language={lang} forWhat={who} />}
        </div>
        <p className="text-[12px] text-ink-3 sm:col-start-2">{t(`vc.about.${engine || 'same'}` as TKey)}</p>
      </div>
    )
  }
  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><AudioLines size={16} /> {t('vc.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('vc.text')}</p>
      <div className="space-y-4">
        {row('routines')}
        {row('chat')}
      </div>
      {openai && v.data && !v.data.openai_key && <p className="mt-3 text-[12.5px] text-danger">{t('vc.needOpenAI')}</p>}
      {eleven && (
        <form className="mt-3 flex flex-wrap items-center gap-2" onSubmit={(e) => { e.preventDefault(); setEleven.mutate(key.trim()) }}>
          <input type="password" autoComplete="off" className={cn(field, 'min-w-[240px] flex-1')} value={key} onChange={(e) => setKey(e.target.value)}
            placeholder={v.data?.elevenlabs_key ? t('models.keySaved') : t('vc.elevenKey')} aria-label={t('vc.elevenKey')} />
          <Button type="submit" size="sm" disabled={!key.trim() || setEleven.isPending}>{setEleven.isPending ? <Loader2 size={13} className="animate-spin" /> : t('ms.connectSee')}</Button>
          <a className="text-[12.5px] underline" href="https://elevenlabs.io/app/settings/api-keys" target="_blank" rel="noreferrer">{t('ms.getKey')}</a>
          {(setEleven.error || v.data?.elevenlabs_error) && <p className="w-full text-[12.5px] text-danger">{setEleven.error?.message ?? v.data?.elevenlabs_error}</p>}
        </form>
      )}
    </Card>
  )
}

function LocalAddresses({ s, save }: { s: Settings; save: (c: (s: Settings) => Settings) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [ollama, setOllama] = useState(s.ollama_url ?? '')
  const [lm, setLm] = useState(s.lmstudio_url ?? '')
  return (
    <details className="mt-3 text-[13px]">
      <summary className="cursor-pointer text-ink-2">{t('ms.addresses')}</summary>
      <form className="mt-2 grid gap-2 sm:grid-cols-[auto_1fr]" onSubmit={(e) => { e.preventDefault(); save((x) => ({ ...x, ollama_url: ollama.trim(), lmstudio_url: lm.trim() })); setTimeout(() => qc.invalidateQueries({ queryKey: ['models', 'detect'] }), 300) }}>
        <label className="self-center text-ink-3" htmlFor="ms-ollama">Ollama</label>
        <input id="ms-ollama" className={field} value={ollama} onChange={(e) => setOllama(e.target.value)} placeholder="http://127.0.0.1:11434" />
        <label className="self-center text-ink-3" htmlFor="ms-lm">LM Studio</label>
        <input id="ms-lm" className={field} value={lm} onChange={(e) => setLm(e.target.value)} placeholder="http://127.0.0.1:1234" />
        <div />
        <div><Button size="sm" type="submit">{t('ms.findAgain')}</Button></div>
      </form>
    </details>
  )
}

function Mine({ s, save }: { s: Settings; save: (c: (s: Settings) => Settings) => void }) {
  const t = useT()
  const mine = s.models ?? []
  const retired = useRetired()
  const test = useMutation({ mutationFn: (m: ModelOption) => api.tryModel(m.id, m.price_in, m.price_out) })
  const remove = (id: string) => save((x) => {
    const back = (v: string, d: string) => (v === id ? d : v)
    const fb = Object.fromEntries(Object.entries(x.fallbacks ?? {}).map(([k, v]) => [k, (v ?? []).filter((y) => y !== id)]))
    return { ...x, models: (x.models ?? []).filter((m) => m.id !== id), fallbacks: fb, explore_model: back(x.explore_model, 'sonnet'), compile_model: back(x.compile_model, 'sonnet'), judge_model: back(x.judge_model, 'haiku') }
  })
  return (
    <Card className="p-5">
      <div className="mb-3 text-[15px] font-medium">{t('models.yours')}</div>
      <ul className="divide-y divide-line rounded-xl border border-line">
        {mine.map((m) => (
          <li key={m.id} className="flex flex-wrap items-center gap-3 px-3 py-2 text-[13px]">
            <code className="min-w-0 flex-1 truncate font-mono text-[12.5px]">{m.id}</code>
            {retired.has(m.id) && <span className="rounded-full bg-danger-soft px-1.5 text-[10.5px] font-medium text-danger" title={t('ms.retiredText')}>{t('ms.retired')}</span>}
            <span className="text-[12px] tabular-nums text-ink-3">{m.id.startsWith('opencode:') ? t('ms.ocPrice') : price(t, m)}</span>
            <Button size="sm" variant="ghost" onClick={() => test.mutate(m)} disabled={test.isPending}>{test.isPending && test.variables?.id === m.id ? <Loader2 size={13} className="animate-spin" /> : t('common.test')}</Button>
            <Button size="sm" variant="ghost" aria-label={t('models.remove', { id: m.id })} onClick={() => remove(m.id)}><Trash2 size={13} /></Button>
          </li>
        ))}
      </ul>
      {mine.some((m) => retired.has(m.id)) && <p className="mt-2 text-[12.5px] text-ink-3">{t('ms.retiredText')}</p>}
      {test.data && <TestResult r={test.data} />}
    </Card>
  )
}

function TestResult({ r }: { r: ModelTest }) {
  const t = useT()
  if (r.ok) return <p className="mt-2 text-[12.5px] text-read">✓ {t('ms.worked', { ms: r.ms ?? 0, cost: usd(r.cost_usd ?? 0) })}</p>
  return (
    <div className="mt-2 rounded-lg bg-danger-soft px-3 py-2 text-[12.5px] text-danger">
      <div className="font-medium">{t(`ms.problem.${r.problem ?? 'other'}` as TKey)}</div>
      {r.error && <div className="mt-0.5 break-words opacity-80">{r.error}</div>}
    </div>
  )
}

// OpencodePicker lists the models of the providers the owner signed in to
// in opencode; a plan's models (Copilot, OpenCode Go) cost no money.
function OpencodePicker({ s, save, onClose }: { s: Settings; save: (c: (s: Settings) => Settings) => void; onClose: () => void }) {
  const t = useT()
  const q = useQuery({ queryKey: ['models', 'opencode'], queryFn: api.opencodeModels, retry: false })
  const [search, setSearch] = useState('')
  const mine = new Set((s.models ?? []).map((m) => m.id))
  const list = (q.data ?? []).filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
  const groups = [...new Set(list.map((m) => m.provider))]
  const add = (id: string) => save((x) => ({ ...x, models: [...(x.models ?? []).filter((y) => y.id !== id), { id, price_in: 0, price_out: 0 }] }))
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(680px,calc(100vw-24px))] -translate-x-1/2 flex-col rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="flex items-start justify-between gap-4 border-b border-line p-5">
            <div>
              <Dialog.Title className="text-[17px] font-semibold tracking-tight">opencode</Dialog.Title>
              <Dialog.Description className="text-[13px] text-ink-3">{t('ms.ocPick')}</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            {q.isLoading ? <p className="flex items-center gap-2 text-[13px] text-ink-3"><Loader2 size={14} className="animate-spin" /> {t('ms.loadingModels')}</p>
              : q.error ? <p className="text-[13px] text-danger">{q.error.message}</p> : (
              <>
                <label className="relative mb-3 block">
                  <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" />
                  <input className={cn(field, 'w-full pl-8')} value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('ms.search')} aria-label={t('ms.search')} />
                </label>
                {groups.map((g) => {
                  const items = list.filter((m) => m.provider === g)
                  return (
                    <div key={g} className="mb-4">
                      <div className="mb-1.5 flex items-center gap-2 text-[12px] font-medium uppercase tracking-wide text-ink-3">
                        {g}{items[0]?.subscription && <span className="rounded-full bg-read/15 px-2 py-0.5 normal-case tracking-normal text-read">{t('ms.ocPlan')}</span>}
                      </div>
                      <ul className="divide-y divide-line rounded-xl border border-line">
                        {items.slice(0, 60).map((m) => (
                          <li key={m.id} className="flex items-center gap-3 px-3 py-2 text-[13px]">
                            <code className="min-w-0 flex-1 truncate font-mono text-[12.5px]">{m.name}</code>
                            {mine.has(m.id) ? <span className="flex items-center gap-1 text-[12px] text-read"><Check size={13} /> {t('ms.added')}</span>
                              : <Button size="sm" onClick={() => add(m.id)}><Plus size={13} /> {t('ms.ocAdd')}</Button>}
                          </li>
                        ))}
                      </ul>
                      {items.length > 60 && <p className="mt-1 text-[12px] text-ink-3">{t('ms.ocMore', { count: items.length - 60 })}</p>}
                    </div>
                  )
                })}
                {list.length === 0 && <p className="text-[13px] text-ink-3">{t('mcp.none')}</p>}
              </>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// Picker connects a provider and lists its models: each can be tested with
// a real one-word answer and then used.
function Picker({ provider, local, onClose, save, s }: { provider: Provider; local?: CatalogModel[]; onClose: () => void; save: (c: (s: Settings) => Settings) => void; s: Settings }) {
  const t = useT()
  const qc = useQueryClient()
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const custom = provider.id === 'custom'
  const hasKey = custom ? !!s.custom_url : !provider.needs_key || !!info.data?.keys[provider.id]
  const [key, setKey] = useState('')
  const [url, setUrl] = useState(s.custom_url ?? '')
  const [q, setQ] = useState('')
  const [prices, setPrices] = useState<Record<string, { in: string; out: string }>>({})
  const [tried, setTried] = useState<Record<string, ModelTest>>({})
  const catalog = useQuery({ queryKey: ['models', 'catalog', provider.id], queryFn: () => api.modelCatalog(provider.id), enabled: !local && hasKey, retry: false })
  // lookAgain asks the provider for its list now instead of the day's copy.
  const lookAgain = useMutation({
    mutationFn: () => api.modelCatalog(provider.id, true),
    onSuccess: (list) => { qc.setQueryData(['models', 'catalog', provider.id], list); qc.invalidateQueries({ queryKey: ['models', 'retired'] }) },
  })
  const connect = useMutation({
    mutationFn: async () => {
      if (custom) save((x) => ({ ...x, custom_url: url.trim() }))
      if (key.trim()) await api.setModelKey(provider.id, key.trim())
    },
    onSuccess: () => { setKey(''); qc.invalidateQueries({ queryKey: ['models'] }) },
  })
  const tryIt = useMutation({
    mutationFn: async (m: CatalogModel) => {
      const p = priceOf(m)
      return { id: m.id, r: await api.tryModel(`${provider.id}:${m.id}`, p!.in, p!.out) }
    },
    onSuccess: ({ id, r }) => {
      setTried((x) => ({ ...x, [id]: r }))
      if (!r.ok) return
      const m = (local ?? catalog.data ?? []).find((y) => y.id === id)!
      const p = priceOf(m)!
      const full = `${provider.id}:${id}`
      save((x) => ({ ...x, models: [...(x.models ?? []).filter((y) => y.id !== full), { id: full, price_in: p.in, price_out: p.out }] }))
    },
  })
  const priceOf = (m: CatalogModel) => {
    if (m.free || provider.local) return { in: 0, out: 0 }
    if (m.priced) return { in: m.price_in, out: m.price_out }
    const typed = prices[m.id]
    if (!typed || typed.in === '' || typed.out === '' || Number(typed.in) < 0 || Number(typed.out) < 0) return null
    return { in: Number(typed.in), out: Number(typed.out) }
  }
  const list = useMemo(() => (local ?? catalog.data ?? []).filter((m) => (m.id + m.name).toLowerCase().includes(q.toLowerCase())), [local, catalog.data, q])
  const inUse = (id: string) => (s.models ?? []).some((m) => m.id === `${provider.id}:${id}`)
  const use = (id: string, jobKeys: ('explore_model' | 'compile_model' | 'judge_model')[]) =>
    save((x) => ({ ...x, ...Object.fromEntries(jobKeys.map((k) => [k, `${provider.id}:${id}`])) }))

  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(680px,calc(100vw-24px))] -translate-x-1/2 flex-col rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="flex items-start justify-between gap-4 border-b border-line p-5">
            <div>
              <Dialog.Title className="text-[17px] font-semibold tracking-tight">{custom ? t('ms.custom') : provider.name}</Dialog.Title>
              <Dialog.Description className="text-[13px] text-ink-3">{t(custom ? 'ms.customText' : provider.local ? 'ms.localText' : 'ms.pickText')}</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>

          {!local && (provider.needs_key || custom) && (
            <form className="space-y-2 border-b border-line p-5" onSubmit={(e) => { e.preventDefault(); connect.mutate() }}>
              {custom && <input className={cn(field, 'w-full')} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://meu-servidor.exemplo/v1" aria-label={t('ms.customUrl')} />}
              <div className="flex flex-wrap items-center gap-2">
                <input type="password" autoComplete="off" className={cn(field, 'min-w-[220px] flex-1')} value={key} onChange={(e) => setKey(e.target.value)}
                  placeholder={info.data?.keys[provider.id] ? t('models.keySaved') : custom ? t('ms.keyOptional') : t('models.keyNew')} aria-label={t('models.keyOf', { provider: provider.name })} />
                <Button type="submit" size="sm" variant={hasKey ? 'secondary' : 'primary'} disabled={(!key.trim() && !(custom && url.trim() !== (s.custom_url ?? ''))) || connect.isPending}>
                  {connect.isPending ? <Loader2 size={14} className="animate-spin" /> : hasKey ? t('ms.changeKey') : t('ms.connectSee')}
                </Button>
              </div>
              {provider.key_url && <a className="inline-flex items-center gap-1 text-[12.5px] text-ink-2 underline" href={provider.key_url} target="_blank" rel="noreferrer">{t('ms.getKey')} <ExternalLink size={12} /></a>}
              {connect.error && <p className="text-[12.5px] text-danger">{connect.error.message}</p>}
            </form>
          )}

          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            {!hasKey && !local ? <p className="text-[13px] text-ink-3">{t('ms.keyFirst')}</p> : catalog.isLoading ? (
              <p className="flex items-center gap-2 text-[13px] text-ink-3"><Loader2 size={14} className="animate-spin" /> {t('ms.loadingModels')}</p>
            ) : catalog.error ? (
              <p className="text-[13px] text-danger">{catalog.error.message}</p>
            ) : (
              <>
                <div className="mb-3 flex items-center gap-2">
                  <label className="relative block min-w-0 flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" />
                    <input className={cn(field, 'w-full pl-8')} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('ms.search')} aria-label={t('ms.search')} />
                  </label>
                  {!local && (
                    <Button size="sm" variant="ghost" onClick={() => lookAgain.mutate()} disabled={lookAgain.isPending} title={t('ms.lookAgainText')}>
                      <RefreshCw size={13} className={cn(lookAgain.isPending && 'animate-spin')} /> {t('ms.lookAgain')}
                    </Button>
                  )}
                </div>
                {lookAgain.error && <p className="mb-2 text-[12.5px] text-danger">{lookAgain.error.message}</p>}
                <ul className="divide-y divide-line rounded-xl border border-line">
                  {list.slice(0, 200).map((m) => {
                    const p = priceOf(m)
                    const r = tried[m.id]
                    const added = inUse(m.id)
                    return (
                      <li key={m.id} className="px-3 py-2.5 text-[13px]">
                        <div className="flex flex-wrap items-center gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-1.5"><span className="truncate font-medium">{m.name}</span><Marks m={m} /></div>
                            {m.name !== m.id && <code className="block truncate text-[11.5px] text-ink-3">{m.id}</code>}
                          </div>
                          {m.priced || m.free || provider.local ? (
                            <span className="text-[12px] tabular-nums text-ink-3">{price(t, { ...m, free: m.free || provider.local })}</span>
                          ) : (
                            <span className="flex items-center gap-1 text-[12px]">
                              <input className={cn(field, 'h-8 w-20')} type="number" min={0} step="0.01" placeholder={t('models.in')} aria-label={t('models.in')}
                                value={prices[m.id]?.in ?? ''} onChange={(e) => setPrices({ ...prices, [m.id]: { in: e.target.value, out: prices[m.id]?.out ?? '' } })} />
                              <input className={cn(field, 'h-8 w-20')} type="number" min={0} step="0.01" placeholder={t('models.out')} aria-label={t('models.out')}
                                value={prices[m.id]?.out ?? ''} onChange={(e) => setPrices({ ...prices, [m.id]: { in: prices[m.id]?.in ?? '', out: e.target.value } })} />
                            </span>
                          )}
                          {m.retired ? null : added && !r ? <span className="flex items-center gap-1 text-[12px] text-read"><Check size={13} /> {t('ms.added')}</span> : (
                            <Button size="sm" disabled={!p || tryIt.isPending} onClick={() => tryIt.mutate(m)} title={!p ? t('ms.needPrice') : undefined}>
                              {tryIt.isPending && tryIt.variables?.id === m.id ? <Loader2 size={13} className="animate-spin" /> : <><Plus size={13} /> {t('ms.testUse')}</>}
                            </Button>
                          )}
                        </div>
                        {r && <TestResult r={r} />}
                        {r?.ok && (
                          <div className="mt-2 flex flex-wrap items-center gap-2 text-[12px]">
                            <span className="text-ink-3">{t('ms.useFor')}</span>
                            <Button size="sm" variant="ghost" onClick={() => use(m.id, ['explore_model', 'compile_model'])}>{t('ms.useTasks')}</Button>
                            <Button size="sm" variant="ghost" onClick={() => use(m.id, ['judge_model'])}>{t('ms.useJudge')}</Button>
                            <Button size="sm" variant="ghost" onClick={() => use(m.id, ['explore_model', 'compile_model', 'judge_model'])}>{t('ms.useAll')}</Button>
                          </div>
                        )}
                      </li>
                    )
                  })}
                  {list.length === 0 && <li className="px-3 py-4 text-[13px] text-ink-3">{t('mcp.none')}</li>}
                </ul>
                {!provider.local && <p className="mt-3 text-[12px] text-ink-3">{t('ms.priceSource')}</p>}
              </>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
