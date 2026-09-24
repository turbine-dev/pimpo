import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, ChevronDown, Plug, Puzzle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { api, type CatalogKind } from '../lib/api'
import { cn } from '../lib/cn'
import { Button, Card, RiskBadge } from './ui'

export function Catalog() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['catalog'], queryFn: api.catalog })
  const list = q.data?.connectors ?? []
  const refresh = () => qc.invalidateQueries({ queryKey: ['catalog'] })
  const reload = useMutation({ mutationFn: () => fetch('/api/connectors/reload', { method: 'POST', credentials: 'same-origin' }), onSuccess: refresh })
  const install = useMutation({
    mutationFn: async (f: File) => {
      const form = new FormData()
      form.append('file', f)
      const res = await fetch('/api/connectors/install', { method: 'POST', body: form, credentials: 'same-origin' })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? res.statusText)
    },
    onSuccess: refresh,
  })
  return (
    <section aria-label="Mais conexões" className="mt-8">
      <h2 className="mb-1 text-[17px] font-semibold tracking-tight">Mais conexões</h2>
      <p className="mb-3 text-[13px] text-ink-2">Cada uma diz exatamente o que permite fazer. O que não dá para desfazer sempre pede sua aprovação.</p>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-[10px] border border-line bg-surface px-3 py-1.5 text-[13px] hover:border-line-strong">
          <Puzzle size={14} /> Instalar conector (.zip)
          <input type="file" accept=".zip" className="sr-only" onChange={(e) => e.target.files?.[0] && install.mutate(e.target.files[0])} />
        </label>
        <Button size="sm" variant="ghost" onClick={() => reload.mutate()} disabled={reload.isPending}>Recarregar a pasta de conectores</Button>
        <a className="text-[12.5px] text-ink-3 underline" href="https://github.com/denerFernandes/vigia/blob/main/docs/CONNECTORS.md" target="_blank" rel="noreferrer">Como criar um conector</a>
        {install.error && <span className="text-[12.5px] text-danger">{install.error.message}</span>}
      </div>
      {(q.data?.broken ?? []).map((b) => (
        <p key={b} className="mb-2 flex gap-2 rounded-xl bg-danger-soft px-3 py-2 text-[13px] text-danger"><AlertTriangle size={15} className="mt-0.5 shrink-0" /> Conector não carregado: {b}</p>
      ))}
      <div className="grid gap-3 lg:grid-cols-2">
        {list.map((k) => <KindCard key={k.id} k={k} />)}
      </div>
    </section>
  )
}

function KindCard({ k }: { k: CatalogKind }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [values, setValues] = useState<Record<string, string>>({})
  const refresh = () => qc.invalidateQueries({ queryKey: ['catalog'] })
  const save = useMutation({ mutationFn: () => api.setCatalog(k.id, values), onSuccess: () => { setValues({}); refresh(); check.mutate() } })
  const remove = useMutation({ mutationFn: () => api.removeCatalog(k.id), onSuccess: refresh })
  const check = useMutation({ mutationFn: () => api.checkCatalog(k.id) })
  const needsSetup = k.fields.length > 0

  return (
    <Card className="p-4">
      <button type="button" className="flex w-full items-center gap-3 text-left" onClick={() => setOpen(!open)} aria-expanded={open}>
        <div className={cn('grid size-9 shrink-0 place-items-center rounded-xl', k.configured ? 'bg-read-soft text-read' : 'bg-sunken text-ink-3')}>
          {k.external ? <Puzzle size={17} /> : <Plug size={17} />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[14.5px] font-medium">
            {k.title}
            {k.external && <span className="rounded-full bg-explore-soft px-1.5 text-[10.5px] font-medium text-explore">externo</span>}
          </div>
          <div className="truncate text-[12.5px] text-ink-3">{k.description}</div>
        </div>
        {k.configured && needsSetup ? <span className="flex items-center gap-1 text-[12px] text-read"><Check size={13} /> Pronto</span> : !needsSetup ? <span className="text-[12px] text-ink-3">Sem configuração</span> : null}
        <ChevronDown size={16} className={cn('shrink-0 text-ink-3 transition', open && 'rotate-180')} />
      </button>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: 'auto', opacity: 1 }} exit={{ height: 0, opacity: 0 }} className="overflow-hidden">
            <div className="mt-4 space-y-3 border-t border-line pt-4">
              <ul className="space-y-1.5">
                {k.capabilities.map((c) => (
                  <li key={c.name} className="flex items-start justify-between gap-3 text-[12.5px]">
                    <span><code className="font-mono text-ink">{c.signature}</code><span className="block text-ink-3">{c.returns}</span></span>
                    <RiskBadge risk={c.risk} />
                  </li>
                ))}
              </ul>
              <p className="text-[12.5px] text-ink-2">{k.help}</p>
              {needsSetup && (
                <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
                  {k.fields.map((f) => (
                    <label key={f.name} className="block">
                      <span className="mb-1 block text-[12px] font-medium text-ink-2">{f.label}{f.optional && ' (opcional)'}</span>
                      <input type={f.secret ? 'password' : 'text'} value={values[f.name] ?? ''} placeholder={f.secret && k.configured ? '••••••• (guardado)' : (k.values[f.name] ?? f.placeholder)}
                        onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
                        className="h-9 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
                    </label>
                  ))}
                  {save.error && <p className="text-[12.5px] text-danger">{save.error.message}</p>}
                  <div className="flex flex-wrap items-center gap-2 pt-1">
                    <Button size="sm" variant="primary" type="submit" disabled={save.isPending}>Salvar</Button>
                    {k.configured && <Button size="sm" type="button" onClick={() => check.mutate()} disabled={check.isPending}>Testar</Button>}
                    {k.configured && !k.external && <Button size="sm" variant="ghost" type="button" onClick={() => remove.mutate()}>Remover</Button>}
                    {check.data && (
                      <span className={cn('text-[12.5px]', check.data.ok ? 'text-read' : 'text-danger')}>
                        {check.data.ok ? '✓ Funcionou' : '✗ ' + check.data.detail}
                      </span>
                    )}
                  </div>
                </form>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </Card>
  )
}
