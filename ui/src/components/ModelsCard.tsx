import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Cpu, Loader2, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api, type ModelOption, type Settings } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { useT } from '../lib/i18n'
import { Button, Card } from './ui'

const field = 'h-9 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const providers = ['anthropic', 'openai', 'openrouter', 'ollama'] as const
const claudeCode = ['sonnet', 'opus', 'haiku']
const roles = [['explore_model', 'models.explore'], ['compile_model', 'models.compile'], ['judge_model', 'models.judge']] as const

// Which model does each job: Claude Code with the owner's subscription,
// or an API model with its price, which the budget needs.
export function ModelsCard({ s, set }: { s: Settings; set: (s: Settings) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const info = useQuery({ queryKey: ['models'], queryFn: api.models })
  const [keys, setKeys] = useState<Record<string, string>>({})
  const saveKey = useMutation({ mutationFn: (p: string) => api.setModelKey(p, keys[p] ?? ''), onSuccess: (_, p) => { setKeys({ ...keys, [p]: '' }); qc.invalidateQueries({ queryKey: ['models'] }) } })
  const [draft, setDraft] = useState({ provider: 'anthropic', name: '', price_in: '', price_out: '' })
  const test = useMutation({ mutationFn: api.testModel })
  const models = s.models ?? []
  const add = () => {
    const id = `${draft.provider}:${draft.name.trim()}`
    const free = draft.provider === 'ollama'
    set({ ...s, models: [...models.filter((m) => m.id !== id), { id, price_in: free ? 0 : Number(draft.price_in), price_out: free ? 0 : Number(draft.price_out) }] })
    setDraft({ ...draft, name: '', price_in: '', price_out: '' })
  }
  const canAdd = draft.name.trim() !== '' && (draft.provider === 'ollama' || (draft.price_in !== '' && draft.price_out !== '' && Number(draft.price_in) >= 0 && Number(draft.price_out) >= 0))
  const remove = (m: ModelOption) => set({ ...s, models: models.filter((x) => x.id !== m.id),
    explore_model: s.explore_model === m.id ? 'sonnet' : s.explore_model, compile_model: s.compile_model === m.id ? 'sonnet' : s.compile_model, judge_model: s.judge_model === m.id ? 'haiku' : s.judge_model })

  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Cpu size={16} /> {t('models.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('models.text')}</p>

      <div className="grid gap-3 sm:grid-cols-3">
        {roles.map(([key, label]) => (
          <label key={key} className="block space-y-1">
            <span className="text-[12.5px] text-ink-2">{t(label)}</span>
            <select className={cn(field, 'w-full')} value={s[key]} onChange={(e) => set({ ...s, [key]: e.target.value })}>
              <optgroup label="Claude Code">
                {claudeCode.map((m) => <option key={m} value={m}>Claude Code · {m}</option>)}
              </optgroup>
              {models.length > 0 && (
                <optgroup label={t('models.yours')}>
                  {models.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
                </optgroup>
              )}
            </select>
          </label>
        ))}
      </div>
      {info.data && !info.data.claude_code && <p className="mt-2 text-[12.5px] text-ink-3">{t('models.noClaude')}</p>}

      <div className="mt-5 space-y-2 border-t border-line pt-4">
        <div className="text-[13px] font-medium text-ink-2">{t('models.keys')}</div>
        {providers.filter((p) => p !== 'ollama').map((p) => (
          <form key={p} className="flex flex-wrap items-center gap-2" onSubmit={(e) => { e.preventDefault(); saveKey.mutate(p) }}>
            <span className="w-24 text-[13px] capitalize">{p}</span>
            <input type="password" className={cn(field, 'min-w-[200px] flex-1')} value={keys[p] ?? ''} onChange={(e) => setKeys({ ...keys, [p]: e.target.value })}
              placeholder={info.data?.keys[p] ? t('models.keySaved') : t('models.keyNew')} aria-label={t('models.keyOf', { provider: p })} autoComplete="off" />
            <Button size="sm" type="submit" disabled={!keys[p] || saveKey.isPending}>{t('common.save')}</Button>
            {info.data?.keys[p] && <Check size={14} className="text-read" aria-label={t('models.keySaved')} />}
          </form>
        ))}
        <label className="flex flex-wrap items-center gap-2">
          <span className="w-24 text-[13px]">Ollama</span>
          <input className={cn(field, 'min-w-[200px] flex-1')} value={s.ollama_url ?? ''} onChange={(e) => set({ ...s, ollama_url: e.target.value })} placeholder="http://127.0.0.1:11434" aria-label={t('models.ollama')} />
        </label>
      </div>

      <div className="mt-5 space-y-2 border-t border-line pt-4">
        <div className="text-[13px] font-medium text-ink-2">{t('models.yours')}</div>
        {models.length > 0 && (
          <ul className="divide-y divide-line rounded-xl border border-line">
            {models.map((m) => (
              <li key={m.id} className="flex flex-wrap items-center gap-3 px-3 py-2 text-[13px]">
                <code className="flex-1 font-mono text-[12.5px]">{m.id}</code>
                <span className="text-[12px] text-ink-3 tabular-nums">{m.id.startsWith('ollama:') ? t('models.free') : t('models.price', { in: m.price_in, out: m.price_out })}</span>
                <Button size="sm" variant="ghost" onClick={() => test.mutate(m.id)} disabled={test.isPending}>{test.isPending && test.variables === m.id ? <Loader2 size={13} className="animate-spin" /> : t('common.test')}</Button>
                <Button size="sm" variant="ghost" aria-label={t('models.remove', { id: m.id })} onClick={() => remove(m)}><Trash2 size={13} /></Button>
              </li>
            ))}
          </ul>
        )}
        {test.data && <p className="text-[12.5px] text-read">✓ {t('models.worked', { cost: usd(test.data.cost_usd) })}</p>}
        {test.error && <p className="text-[12.5px] text-danger">{test.error.message}</p>}
        <div className="flex flex-wrap items-end gap-2">
          <select className={field} value={draft.provider} onChange={(e) => setDraft({ ...draft, provider: e.target.value })} aria-label={t('models.provider')}>
            {providers.map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
          <input className={cn(field, 'min-w-[180px] flex-1 font-mono')} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder={draft.provider === 'ollama' ? 'qwen3:8b' : draft.provider === 'anthropic' ? 'claude-sonnet-5' : 'nome-do-modelo'} aria-label={t('models.name')} />
          {draft.provider !== 'ollama' && <>
            <input className={cn(field, 'w-28')} type="number" min={0} step="0.01" value={draft.price_in} onChange={(e) => setDraft({ ...draft, price_in: e.target.value })} placeholder={t('models.in')} aria-label={t('models.in')} />
            <input className={cn(field, 'w-28')} type="number" min={0} step="0.01" value={draft.price_out} onChange={(e) => setDraft({ ...draft, price_out: e.target.value })} placeholder={t('models.out')} aria-label={t('models.out')} />
          </>}
          <Button size="sm" onClick={add} disabled={!canAdd}><Plus size={14} /> {t('models.add')}</Button>
        </div>
        <p className="text-[12px] text-ink-3">{t('models.priceHint')}</p>
      </div>
    </Card>
  )
}
