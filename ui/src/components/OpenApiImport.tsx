import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Loader2, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, type CapRisk, type OpenApiPreview, type OpenApiSource } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { SecretInput } from './SecretInput'
import { Button } from './ui'

const field = 'h-9 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const risks: CapRisk[] = ['read', 'notify', 'reversible', 'irreversible']
const MAX = 100
const SHOWN = 200

const methodTone: Record<string, string> = {
  GET: 'bg-read-soft text-read', POST: 'bg-sunken text-ink-2', PUT: 'bg-sunken text-ink-2', PATCH: 'bg-sunken text-ink-2', DELETE: 'bg-danger-soft text-danger',
}

function suggest(title: string) {
  const n = title.toLowerCase().replace(/[^a-z0-9]/g, '').replace(/(openapi|restapi|api)+$/, '').slice(0, 31)
  return /^[a-z]/.test(n) && n.length >= 2 ? n : 'api' + n.slice(0, 28)
}

// OpenApiImport turns a REST API's OpenAPI description into a JSON
// connector: read it, pick the operations and their risk, install.
export function OpenApiImport() {
  const t = useT()
  const qc = useQueryClient()
  const [mode, setMode] = useState<'url' | 'paste'>('url')
  const [url, setUrl] = useState('')
  const [spec, setSpec] = useState('')
  const [header, setHeader] = useState('')
  const [headerDraft, setHeaderDraft] = useState('Authorization')
  const [name, setName] = useState('')
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [chosen, setChosen] = useState<Record<string, CapRisk>>({})
  const [filter, setFilter] = useState('')
  const source = (h = header): OpenApiSource => (mode === 'url' ? { url: url.trim(), header: h } : { spec, header: h })

  const preview = useMutation({
    mutationFn: (h: string) => api.openapiPreview(source(h)),
    onSuccess: (d: OpenApiPreview, h) => {
      setHeader(h)
      if (!name) setName(suggest(d.title))
      // A small API starts with its reads chosen; a large one with none.
      if (!h) setChosen(d.operations.length <= 30 ? Object.fromEntries(d.operations.filter((o) => o.method === 'GET').map((o) => [o.id, o.risk])) : {})
    },
  })
  const add = useMutation({
    mutationFn: () => api.openapiAdd({ ...source(), name, operations: chosen, keys }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['catalog'] }),
  })
  const d = preview.data
  const words = filter.toLowerCase().split(/\s+/).filter(Boolean)
  const visible = useMemo(() => (d?.operations ?? []).filter((o) => {
    const text = `${o.id} ${o.method} ${o.path} ${o.summary}`.toLowerCase()
    return words.every((w) => text.includes(w))
  }), [d, filter]) // eslint-disable-line react-hooks/exhaustive-deps
  const count = Object.keys(chosen).length
  const toggle = (id: string, risk: CapRisk, on: boolean) => {
    const next = { ...chosen }
    if (on) next[id] = risk
    else delete next[id]
    setChosen(next)
  }
  const reset = () => { preview.reset(); add.reset(); setChosen({}); setKeys({}); setHeader(''); setName('') }

  if (add.isSuccess) {
    return (
      <div className="space-y-3">
        <p className="rounded-lg bg-read-soft px-3 py-2 text-[13px] text-read">{t('mcp.installed', { name })}</p>
        <Button onClick={reset}>{t('oa.another')}</Button>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-[12.5px] text-ink-3">{t('oa.hint')}</p>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); setChosen({}); preview.mutate('') }}>
        <div role="radiogroup" className="flex gap-2">
          {(['url', 'paste'] as const).map((k) => (
            <button key={k} type="button" role="radio" aria-checked={mode === k} onClick={() => { setMode(k); reset() }}
              className={cn('rounded-full border px-3 py-1.5 text-[12.5px]', mode === k ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2')}>
              {t(k === 'url' ? 'oa.fromUrl' : 'oa.fromPaste')}
            </button>
          ))}
        </div>
        {mode === 'url' ? (
          <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('oa.url')}</span>
            <input className={field} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…/openapi.json" inputMode="url" spellCheck={false} /></label>
        ) : (
          <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('oa.paste')}</span>
            <textarea className="min-h-32 w-full rounded-[10px] border border-line bg-bg p-3 font-mono text-[12.5px] outline-none focus:border-accent" value={spec} onChange={(e) => setSpec(e.target.value)} spellCheck={false} /></label>
        )}
        <Button type="submit" variant="primary" disabled={preview.isPending || (mode === 'url' ? !/^https?:\/\//.test(url.trim()) : !spec.trim())}>
          {preview.isPending ? <><Loader2 size={14} className="animate-spin" /> {t('oa.reading')}</> : t('oa.read')}
        </Button>
        {preview.error && <p className="text-[13px] text-danger">{preview.error.message}</p>}
      </form>

      {d && (
        <div className="space-y-4 border-t border-line pt-4">
          <div>
            <div className="text-[15px] font-semibold tracking-tight">{d.title}</div>
            {d.description && <p className="mt-0.5 line-clamp-3 text-[12.5px] text-ink-2">{d.description}</p>}
            <p className="mt-1 text-[12px] text-ink-3">{t('oa.summary', { base: d.base, count: d.operations.length })}</p>
          </div>
          <p className="flex gap-2 rounded-xl bg-sunken px-3 py-2 text-[12.5px] text-ink-2">
            <AlertTriangle size={15} className="mt-0.5 shrink-0 text-ink-3" /> {t('oa.warn', { host: d.base.replace(/^https?:\/\//, '').split('/')[0] })}
          </p>

          <label className="block space-y-1">
            <span className="text-[12.5px] text-ink-2">{t('mcp.name')}</span>
            <input className={cn(field, 'font-mono')} value={name} onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9]/g, ''))} />
            <span className="block text-[11.5px] text-ink-3">{t('mcp.nameHint', { name: name || 'nome' })}</span>
          </label>

          {d.keys.map((k) => (
            <label key={k.name} className="block space-y-1">
              <span className="text-[12.5px] text-ink-2"><code className="font-mono">{k.name}</code> ({t('common.optional')})</span>
              <SecretInput className={field} autoComplete="off" value={keys[k.name] ?? ''} onValue={(v) => setKeys({ ...keys, [k.name]: v })} />
              <span className="block text-[11.5px] text-ink-3">{k.description}</span>
            </label>
          ))}
          {!header && (
            <details className="rounded-xl border border-line px-3 py-2 text-[12.5px]">
              <summary className="cursor-pointer text-ink-2">{t('oa.customKey')}</summary>
              <div className="mt-2 flex gap-2">
                <input className={cn(field, 'font-mono')} value={headerDraft} onChange={(e) => setHeaderDraft(e.target.value)} aria-label={t('oa.headerName')} />
                <Button size="sm" onClick={() => preview.mutate(headerDraft.trim())} disabled={!headerDraft.trim() || preview.isPending}>{t('oa.addHeader')}</Button>
              </div>
              <p className="mt-1 text-[11.5px] text-ink-3">{t('oa.customKeyHint')}</p>
            </details>
          )}

          <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-[14px] font-medium">{t('oa.operations')}</div>
              <div className="flex gap-1.5">
                <Button size="sm" variant="ghost" onClick={() => setChosen({ ...chosen, ...Object.fromEntries(visible.filter((o) => o.method === 'GET').slice(0, MAX).map((o) => [o.id, chosen[o.id] ?? o.risk])) })}>{t('oa.pickReads')}</Button>
                <Button size="sm" variant="ghost" onClick={() => setChosen({})} disabled={count === 0}>{t('oa.clear')}</Button>
              </div>
            </div>
            <p className="text-[12px] text-ink-3">{t('oa.opsHint')}</p>
            {d.operations.length > 8 && (
              <label className="flex h-9 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 focus-within:border-accent">
                <Search size={14} className="text-ink-3" />
                <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('oa.filter')} aria-label={t('oa.filter')} className="flex-1 bg-transparent text-sm outline-none" />
              </label>
            )}
            <ul className="max-h-[52vh] divide-y divide-line overflow-y-auto rounded-xl border border-line">
              {visible.slice(0, SHOWN).map((o) => (
                <li key={o.id} className="flex items-start gap-3 px-3 py-2.5">
                  <input type="checkbox" className="mt-1 size-4 accent-[var(--color-accent)]" checked={!!chosen[o.id]} aria-label={t('mcp.include', { tool: o.id })}
                    onChange={(e) => toggle(o.id, o.risk, e.target.checked)} />
                  <div className="min-w-0 flex-1">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className={cn('shrink-0 rounded px-1.5 py-0.5 font-mono text-[10.5px] font-semibold', methodTone[o.method])}>{o.method}</span>
                      <code className="truncate font-mono text-[12px]" title={o.path}>{o.path}</code>
                    </div>
                    <p className="line-clamp-2 text-[12px] text-ink-3">{o.summary || o.id}</p>
                  </div>
                  <select className="h-8 max-w-[44%] rounded-lg border border-line bg-bg px-2 text-[12px]" value={chosen[o.id] ?? o.risk} disabled={!chosen[o.id]} aria-label={t('mcp.riskOf', { tool: o.id })}
                    onChange={(e) => toggle(o.id, e.target.value as CapRisk, true)}>
                    {risks.map((r) => <option key={r} value={r}>{t(`mcp.risk.${r}` as TKey)}</option>)}
                  </select>
                </li>
              ))}
            </ul>
            {visible.length > SHOWN && <p className="text-[12px] text-ink-3">{t('oa.more', { shown: SHOWN, count: visible.length })}</p>}
            {d.unsupported.length > 0 && (
              <details className="text-[12px] text-ink-3">
                <summary className="cursor-pointer">{t('oa.unsupported', { count: d.unsupported.length })}</summary>
                <ul className="mt-1 space-y-0.5 pl-4">{d.unsupported.map((u) => <li key={u.id}><code className="font-mono">{u.id}</code>: {u.why}</li>)}</ul>
              </details>
            )}
          </div>

          {count > MAX && <p className="text-[13px] text-danger">{t('oa.tooMany', { max: MAX })}</p>}
          {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
          <Button variant="primary" onClick={() => add.mutate()} disabled={count === 0 || count > MAX || name.length < 2 || add.isPending}>
            {add.isPending && <Loader2 size={14} className="animate-spin" />} {t('oa.install', { count })}
          </Button>
        </div>
      )}
    </div>
  )
}
