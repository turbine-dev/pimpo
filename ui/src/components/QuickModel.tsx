import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ExternalLink, KeyRound, Laptop, Loader2, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, type QuickChoice } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { Button } from './ui'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

type Option = { id: string; title: string; text: string; icon: React.ReactNode; choice?: QuickChoice; models?: string[]; kind: QuickChoice['kind'] }

// QuickModel sets up the models from what this computer already has, or
// from one API key, so the first routine needs nothing else.
export function QuickModel({ onDone }: { onDone?: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const found = useQuery({ queryKey: ['models-detect'], queryFn: api.detectModels })
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const oc = useQuery({ queryKey: ['opencode-models'], queryFn: api.opencodeModels, enabled: !!found.data?.opencode })
  const [picked, setPicked] = useState('')
  const [model, setModel] = useState('')
  const [provider, setProvider] = useState('anthropic')
  const [key, setKey] = useState('')
  const setup = useMutation({
    mutationFn: (c: QuickChoice) => api.quickSetup(c),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['settings'] })
      onDone?.()
    },
  })

  const f = found.data
  const options: Option[] = []
  if (f?.claude_code) options.push({ id: 'claude_code', kind: 'claude_code', title: 'Claude Code', text: t('qs.claudeCode'), icon: <Sparkles size={16} /> })
  if (f?.codex_login) options.push({ id: 'codex', kind: 'codex', title: 'Codex', text: t('qs.codex'), icon: <Sparkles size={16} /> })
  // opencode takes a few seconds to list its models: the option shows at
  // once, so the cards below it do not move under the pointer.
  if (f?.opencode) options.push({ id: 'opencode', kind: 'opencode', title: 'opencode', text: t('qs.opencode'), icon: <Sparkles size={16} />, models: (oc.data ?? []).map((m) => m.id) })
  if (f?.ollama_up && f.ollama.length > 0) options.push({ id: 'ollama', kind: 'ollama', title: 'Ollama', text: t('qs.local'), icon: <Laptop size={16} />, models: f.ollama.map((m) => m.id) })
  if (f?.lmstudio_up && f.lmstudio.length > 0) options.push({ id: 'lmstudio', kind: 'lmstudio', title: 'LM Studio', text: t('qs.local'), icon: <Laptop size={16} />, models: f.lmstudio.map((m) => m.id) })
  options.push({ id: 'provider', kind: 'provider', title: t('qs.apiKey'), text: t('qs.apiKeyText'), icon: <KeyRound size={16} /> })

  const providers = (info.data?.providers ?? []).filter((p) => !p.local && p.id !== 'custom')
  const current = providers.find((p) => p.id === provider)
  const chosen = options.find((o) => o.id === picked)
  const choose = (o: Option) => {
    setPicked(o.id)
    setModel(o.models?.[0] ?? '')
    setup.reset()
  }
  const firstOpencode = oc.data?.[0]?.id ?? ''
  useEffect(() => {
    if (picked === 'opencode' && !model && firstOpencode) setModel(firstOpencode)
  }, [picked, model, firstOpencode])
  const go = () => {
    if (!chosen) return
    if (chosen.kind === 'provider') setup.mutate({ kind: 'provider', provider, key })
    else if (chosen.models) setup.mutate({ kind: chosen.kind, model })
    else setup.mutate({ kind: chosen.kind })
  }
  const ready = chosen && (chosen.kind === 'provider' ? key.trim().length > 8 : !chosen.models || !!model)

  if (found.isPending) return <p className="flex items-center gap-2 text-[13.5px] text-ink-3"><Loader2 size={15} className="animate-spin" /> {t('qs.looking')}</p>

  return (
    <div className="space-y-3 text-[14.5px] text-ink-2">
      <p>{t('qs.intro')}</p>
      <div role="radiogroup" aria-label={t('qs.intro')} className="grid gap-2">
        {options.map((o) => (
          <button key={o.id} type="button" role="radio" aria-checked={picked === o.id} onClick={() => choose(o)}
            className={cn('flex items-start gap-3 rounded-xl border p-3.5 text-left transition-colors', picked === o.id ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
            <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-lg bg-sunken text-ink-2">{o.icon}</span>
            <span className="min-w-0">
              <span className="block text-[14px] font-medium text-ink">{o.title}</span>
              <span className="block text-[12.5px] text-ink-3">{o.text}</span>
            </span>
          </button>
        ))}
      </div>

      {chosen?.kind === 'opencode' && oc.isPending && <p className="flex items-center gap-2 text-[12.5px] text-ink-3"><Loader2 size={14} className="animate-spin" /> {t('qs.looking')}</p>}
      {chosen?.kind === 'opencode' && oc.isSuccess && oc.data.length === 0 && <p className="text-[12.5px] text-ink-3">{t('qs.opencodeNone')}</p>}
      {chosen?.models && chosen.models.length > 0 && (
        <div className="space-y-1">
          <label className="block space-y-1">
            <span className="text-[12.5px]">{t('qs.whichModel')}</span>
            <select className={field} value={model} onChange={(e) => setModel(e.target.value)}>
              {chosen.models.map((m) => <option key={m} value={m}>{m.replace(/^opencode:/, '')}</option>)}
            </select>
          </label>
          {chosen.kind !== 'opencode' && <p className="text-[11.5px] text-ink-3">{t('qs.localHint')}</p>}
        </div>
      )}

      {chosen?.kind === 'provider' && (
        <div className="space-y-2">
          <label className="block space-y-1">
            <span className="text-[12.5px]">{t('qs.provider')}</span>
            <select className={field} value={provider} onChange={(e) => { setProvider(e.target.value); setup.reset() }}>
              {providers.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </label>
          <label className="block space-y-1">
            <span className="text-[12.5px]">{t('qs.key')}</span>
            <input className={field} type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} spellCheck={false} />
          </label>
          {current?.key_url && (
            <a className="inline-flex items-center gap-1 text-[12.5px] text-ink-3 underline" href={current.key_url} target="_blank" rel="noreferrer">
              {t('qs.getKey', { name: current.name })} <ExternalLink size={12} />
            </a>
          )}
          <p className="text-[11.5px] text-ink-3">{t('qs.keySafe')}</p>
        </div>
      )}

      {chosen && !setup.isSuccess && (
        <Button variant="primary" onClick={go} disabled={!ready || setup.isPending}>
          {setup.isPending ? <><Loader2 size={14} className="animate-spin" /> {t('qs.testing')}</> : t('qs.use')}
        </Button>
      )}
      {setup.error && <p className="text-[13px] text-danger">{setup.error.message}</p>}
      {setup.data && (
        <p className="flex items-start gap-2 rounded-lg bg-read-soft px-3 py-2 text-[13px] text-read">
          <Check size={15} className="mt-0.5 shrink-0" />
          <span>{t('qs.ready', { explore: setup.data.explore.replace(/^[a-z]+:/, ''), judge: setup.data.judge.replace(/^[a-z]+:/, '') })}{setup.data.ms ? ` · ${(setup.data.ms / 1000).toFixed(1)} s` : ''}</span>
        </p>
      )}
      <p className="text-[12px] text-ink-3">{t('qs.later')}</p>
    </div>
  )
}
