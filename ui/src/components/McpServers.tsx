import * as Dialog from '@radix-ui/react-dialog'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Cloud, Laptop, Loader2, Search, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, type CapRisk, type McpListing, type McpTool } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { SecretInput } from './SecretInput'
import { Button } from './ui'

const field = 'h-9 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const risks: CapRisk[] = ['read', 'notify', 'reversible', 'irreversible']
const categories: [TKey, string][] = [
  ['mcp.cat.files', 'file'], ['mcp.cat.dev', 'github'], ['mcp.cat.data', 'database'], ['mcp.cat.web', 'search'],
  ['mcp.cat.notes', 'notes'], ['mcp.cat.chat', 'slack'], ['mcp.cat.calendar', 'calendar'], ['mcp.cat.finance', 'finance'],
  ['mcp.cat.maps', 'maps'], ['mcp.cat.home', 'home'],
]

// Explore searches the official MCP registry.
export function McpExplore() {
  const t = useT()
  const [text, setText] = useState('')
  const [q, setQ] = useState('')
  useEffect(() => {
    const id = setTimeout(() => setQ(text.trim()), 350)
    return () => clearTimeout(id)
  }, [text])
  const pages = useInfiniteQuery({
    queryKey: ['mcp-search', q],
    queryFn: ({ pageParam }) => api.mcpSearch(q, pageParam),
    initialPageParam: '',
    getNextPageParam: (last) => last.next || undefined,
  })
  const list = pages.data?.pages.flatMap((p) => p.servers) ?? []
  const [picked, setPicked] = useState<McpListing | null>(null)
  return (
    <div className="space-y-3">
      <p className="text-[12.5px] text-ink-3">{t('mcp.searchHint')}</p>
      <label className="flex h-10 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 focus-within:border-accent">
        <Search size={15} className="text-ink-3" />
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder={t('mcp.search')} aria-label={t('mcp.search')} className="flex-1 bg-transparent text-sm outline-none" />
      </label>
      <div className="flex flex-wrap gap-1.5">
        {categories.map(([label, term]) => (
          <button key={term} type="button" aria-pressed={q === term} onClick={() => setText(q === term ? '' : term)}
            className={cn('rounded-full border px-2.5 py-1 text-[12px] transition', q === term ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
            {t(label)}
          </button>
        ))}
      </div>
      {pages.error && <p className="text-[13px] text-danger">{pages.error.message}</p>}
      {pages.isPending ? <p className="flex items-center gap-2 text-[12.5px] text-ink-3"><Loader2 size={15} className="animate-spin" /> {t('mcp.searching')}</p> : list.length === 0 ? <p className="text-[13px] text-ink-3">{t('mcp.none')}</p> : (
        <ul className="grid gap-2 lg:grid-cols-2">
          {list.map((s) => (
            <li key={s.id} className="flex items-start gap-3 rounded-xl border border-line bg-surface p-3">
              <div className="grid size-8 shrink-0 place-items-center rounded-lg bg-sunken text-ink-3">{s.kind === 'remote' ? <Cloud size={15} /> : <Laptop size={15} />}</div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-[13.5px] font-medium" title={s.id}>{s.title}</div>
                <div className="line-clamp-2 text-[12px] text-ink-3">{s.description}</div>
                <div className="mt-1 text-[11px] text-ink-3">{s.kind === 'unsupported' ? t('mcp.unsupported') : s.kind === 'remote' ? t('mcp.remote') : `${t('mcp.local')} · ${s.kind}`} · v{s.version}</div>
              </div>
              <Button size="sm" disabled={s.kind === 'unsupported'} onClick={() => setPicked(s)} aria-label={`${t('mcp.add')} ${s.title}`}>{t('mcp.add')}</Button>
            </li>
          ))}
        </ul>
      )}
      {pages.hasNextPage && <Button size="sm" variant="ghost" onClick={() => pages.fetchNextPage()} disabled={pages.isFetchingNextPage}>{t('mcp.more')}</Button>}
      {picked && <AddServer listing={picked} onClose={() => setPicked(null)} />}
    </div>
  )
}

// splitCommand splits a command line on spaces, keeping quoted parts.
function splitCommand(s: string) {
  return (s.match(/"[^"]*"|'[^']*'|\S+/g) ?? []).map((p) => p.replace(/^["']|["']$/g, ''))
}

function lines(s: string, sep: string) {
  const out: Record<string, string> = {}
  for (const l of s.split('\n')) {
    const i = l.indexOf(sep)
    if (i > 0) out[l.slice(0, i).trim()] = l.slice(i + 1).trim()
  }
  return out
}

// Manual takes a command or a URL typed by the owner.
export function McpManual() {
  const t = useT()
  const [kind, setKind] = useState<'command' | 'url'>('command')
  const [cmd, setCmd] = useState('')
  const [url, setUrl] = useState('')
  const [extra, setExtra] = useState('')
  const [picked, setPicked] = useState<{ listing: McpListing; env: Record<string, string>; headers: Record<string, string> } | null>(null)
  const go = () => {
    const [command, ...args] = splitCommand(cmd)
    const listing: McpListing = kind === 'command'
      ? { id: cmd, name: '', title: (args.find((a) => !a.startsWith('-')) ?? command).replace(/^@[^/]+\//, '').replace(/(@|==).*$/, ''), description: '', version: '', kind: 'npm', command, args, inputs: [] }
      : { id: url, name: '', title: url.replace(/^https:\/\//, '').split('/')[0], description: '', version: '', kind: 'remote', url, inputs: [] }
    setPicked({ listing, env: kind === 'command' ? lines(extra, '=') : {}, headers: kind === 'url' ? lines(extra, ':') : {} })
  }
  return (
    <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); go() }}>
      <div role="radiogroup" className="flex gap-2">
        {(['command', 'url'] as const).map((k) => (
          <button key={k} type="button" role="radio" aria-checked={kind === k} onClick={() => setKind(k)}
            className={cn('rounded-full border px-3 py-1.5 text-[12.5px]', kind === k ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2')}>
            {t(k === 'command' ? 'mcp.kindCommand' : 'mcp.kindUrl')}
          </button>
        ))}
      </div>
      {kind === 'command' ? (
        <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('mcp.command')}</span>
          <input className={cn(field, 'font-mono')} value={cmd} onChange={(e) => setCmd(e.target.value)} placeholder={t('mcp.commandHint')} spellCheck={false} /></label>
      ) : (
        <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('mcp.url')}</span>
          <input className={field} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://" inputMode="url" /></label>
      )}
      <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t(kind === 'command' ? 'mcp.envLines' : 'mcp.headerLines')}</span>
        <textarea className="min-h-20 w-full rounded-[10px] border border-line bg-bg p-3 font-mono text-[13px] outline-none focus:border-accent" value={extra} onChange={(e) => setExtra(e.target.value)} spellCheck={false} /></label>
      <Button type="submit" variant="primary" disabled={kind === 'command' ? !cmd.trim() : !url.startsWith('https://')}>{t('mcp.check')}</Button>
      {picked && <AddServer listing={picked.listing} env={picked.env} headers={picked.headers} onClose={() => setPicked(null)} />}
    </form>
  )
}

function suggest(title: string) {
  const n = title.toLowerCase().replace(/[^a-z0-9]/g, '').replace(/(mcpserver|servermcp|mcp|server)$/, '').slice(0, 31)
  return /^[a-z]/.test(n) && n.length >= 2 ? n : 'x' + n
}

// AddServer asks for what the server needs, lists its tools and lets the
// owner set each one's risk before installing.
function AddServer({ listing, env = {}, headers = {}, onClose }: { listing: McpListing; env?: Record<string, string>; headers?: Record<string, string>; onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [name, setName] = useState(listing.name || suggest(listing.title))
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(listing.inputs.map((i) => [`${i.kind}:${i.name}`, i.default ?? ''])))
  const [chosen, setChosen] = useState<Record<string, CapRisk | null>>({})
  const source = () => {
    const pick = (kind: string) => Object.fromEntries(listing.inputs.filter((i) => i.kind === kind).map((i) => [i.name, values[`${kind}:${i.name}`] ?? '']))
    return { name, command: listing.command, args: listing.args, url: listing.url, env: { ...env, ...pick('env') }, headers: { ...headers, ...pick('header') }, arg_values: pick('arg') }
  }
  const probe = useMutation({
    mutationFn: () => api.mcpProbe(source()),
    onSuccess: (d) => setChosen(Object.fromEntries(d.tools.map((x) => [x.tool, x.risk]))),
  })
  const add = useMutation({
    mutationFn: () => api.mcpAdd({ ...source(), description: listing.description, source: listing.version ? `${listing.id}@${listing.version}` : listing.id,
      tools: Object.fromEntries(Object.entries(chosen).filter(([, r]) => r)) as Record<string, CapRisk> }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['catalog'] }),
  })
  const count = Object.values(chosen).filter(Boolean).length
  const missing = listing.inputs.some((i) => i.required && !values[`${i.kind}:${i.name}`]?.trim())
  const tools: McpTool[] = probe.data?.tools ?? []

  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[8vh] z-50 max-h-[84vh] w-[min(600px,calc(100vw-32px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-3 flex items-start justify-between gap-4">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-[17px] font-semibold tracking-tight">{t('mcp.title', { title: listing.title })}</Dialog.Title>
              <Dialog.Description className="mt-1 text-[13px] text-ink-2">{listing.description}</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <p className="mb-4 flex gap-2 rounded-xl bg-sunken px-3 py-2 text-[12.5px] text-ink-2">
            <AlertTriangle size={15} className="mt-0.5 shrink-0 text-ink-3" />
            {t('mcp.warn', { where: t(listing.kind === 'remote' ? 'mcp.whereRemote' : 'mcp.whereLocal') })}
          </p>
          {add.isSuccess ? (
            <div className="space-y-3">
              <p className="rounded-lg bg-read-soft px-3 py-2 text-[13px] text-read">{t('mcp.installed', { name })}</p>
              <Button onClick={onClose}>{t('common.close')}</Button>
            </div>
          ) : (
            <div className="space-y-3">
              <label className="block space-y-1">
                <span className="text-[12.5px] text-ink-2">{t('mcp.name')}</span>
                <input className={cn(field, 'font-mono')} value={name} onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9]/g, ''))} />
                <span className="block text-[11.5px] text-ink-3">{t('mcp.nameHint', { name: name || 'nome' })}</span>
              </label>
              {listing.inputs.map((i) => (
                <label key={`${i.kind}:${i.name}`} className="block space-y-1">
                  <span className="text-[12.5px] text-ink-2"><code className="font-mono">{i.name}</code>{!i.required && ` (${t('common.optional')})`}</span>
                  {i.secret
                    ? <SecretInput className={field} value={values[`${i.kind}:${i.name}`] ?? ''} autoComplete="off" onValue={(v) => setValues({ ...values, [`${i.kind}:${i.name}`]: v })} />
                    : <input className={field} type="text" value={values[`${i.kind}:${i.name}`] ?? ''} autoComplete="off"
                      onChange={(e) => setValues({ ...values, [`${i.kind}:${i.name}`]: e.target.value })} />}
                  {i.description && <span className="block text-[11.5px] text-ink-3">{i.description}</span>}
                </label>
              ))}
              {!probe.data && (
                <Button variant="primary" onClick={() => probe.mutate()} disabled={missing || name.length < 2 || probe.isPending}>
                  {probe.isPending ? <><Loader2 size={14} className="animate-spin" /> {t('mcp.checking')}</> : t('mcp.check')}
                </Button>
              )}
              {probe.error && <p className="text-[13px] text-danger">{probe.error.message}</p>}
              {tools.length > 0 && (
                <div className="space-y-2 border-t border-line pt-3">
                  <div className="text-[14px] font-medium">{t('mcp.tools')}</div>
                  <p className="text-[12px] text-ink-3">{t('mcp.toolsHint')}</p>
                  <ul className="divide-y divide-line rounded-xl border border-line">
                    {tools.map((x) => (
                      <li key={x.tool} className="flex items-start gap-3 px-3 py-2.5">
                        <input type="checkbox" className="mt-1 size-4 accent-[var(--color-accent)]" checked={!!chosen[x.tool]} aria-label={t('mcp.include', { tool: x.tool })}
                          onChange={(e) => setChosen({ ...chosen, [x.tool]: e.target.checked ? x.risk : null })} />
                        <div className="min-w-0 flex-1">
                          <code className="font-mono text-[12.5px]">{x.tool}</code>
                          {x.description && <p className="line-clamp-2 text-[12px] text-ink-3">{x.description}</p>}
                          {x.claimed && <p className="text-[11.5px] text-ink-3">{t('mcp.claim', { risk: t(`mcp.risk.${x.claimed}` as TKey) })}</p>}
                        </div>
                        <select className="h-8 rounded-lg border border-line bg-bg px-2 text-[12px]" value={chosen[x.tool] ?? x.risk} disabled={!chosen[x.tool]} aria-label={t('mcp.riskOf', { tool: x.tool })}
                          onChange={(e) => setChosen({ ...chosen, [x.tool]: e.target.value as CapRisk })}>
                          {risks.map((r) => <option key={r} value={r}>{t(`mcp.risk.${r}` as TKey)}</option>)}
                        </select>
                      </li>
                    ))}
                  </ul>
                  {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
                  <Button variant="primary" onClick={() => add.mutate()} disabled={count === 0 || add.isPending}>
                    {add.isPending && <Loader2 size={14} className="animate-spin" />} {t('mcp.install', { count })}
                  </Button>
                </div>
              )}
            </div>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
