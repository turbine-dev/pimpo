import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ReceiptText, Search, ShieldAlert, Undo2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button, Card, EmptyState, RiskBadge } from '../components/ui'
import { describe } from '../lib/actions'
import { api, type Receipt } from '../lib/api'
import { cn } from '../lib/cn'

type Filter = 'all' | 'changes' | 'blocked'

export function Receipts() {
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const receipts = useQuery({ queryKey: ['receipts', q], queryFn: () => api.receipts(q) })
  const list = useMemo(() => {
    const all = receipts.data ?? []
    if (filter === 'changes') return all.filter((r) => r.action.risk !== 'read')
    if (filter === 'blocked') return all.filter((r) => r.action.verdict === 'block' || r.action.approved === 'no')
    return all
  }, [receipts.data, filter])
  const byDay = useMemo(() => {
    const groups: [string, Receipt[]][] = []
    for (const r of list) {
      const day = new Date(r.ts).toLocaleDateString('pt-BR', { weekday: 'long', day: '2-digit', month: 'long' })
      const last = groups[groups.length - 1]
      if (last && last[0] === day) last[1].push(r)
      else groups.push([day, [r]])
    }
    return groups
  }, [list])

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Recibos</h1>
      <p className="mb-5 text-sm text-ink-2">Tudo o que eu fiz, com os detalhes e o botão de desfazer. O registro é encadeado: qualquer alteração no histórico é detectada.</p>
      <div className="mb-5 flex flex-wrap items-center gap-2">
        <label className="flex h-9 flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 text-sm focus-within:border-accent">
          <Search size={15} className="text-ink-3" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Buscar nos recibos" aria-label="Buscar nos recibos" className="w-full bg-transparent outline-none placeholder:text-ink-3" />
        </label>
        {(['all', 'changes', 'blocked'] as Filter[]).map((f) => (
          <button key={f} onClick={() => setFilter(f)} className={cn('h-9 rounded-[10px] px-3 text-[13px]', filter === f ? 'bg-ink text-bg' : 'text-ink-2 hover:bg-sunken')}>
            {f === 'all' ? 'Tudo' : f === 'changes' ? 'Mudanças' : 'Bloqueados'}
          </button>
        ))}
      </div>
      {list.length === 0 && !receipts.isPending && (
        <EmptyState icon={<ReceiptText size={22} />} title="Nada por aqui">
          Quando uma rotina ou uma exploração fizer algo, o recibo aparece aqui.
        </EmptyState>
      )}
      <div className="space-y-6">
        {byDay.map(([day, items]) => (
          <section key={day}>
            <h2 className="mb-2 text-[12.5px] font-medium capitalize text-ink-3">{day}</h2>
            <Card className="divide-y divide-line">
              {items.map((r) => (
                <Row key={r.id} r={r} />
              ))}
            </Card>
          </section>
        ))}
      </div>
    </div>
  )
}

function Row({ r }: { r: Receipt }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const undo = useMutation({ mutationFn: () => api.undo(r.id), onSuccess: () => qc.invalidateQueries({ queryKey: ['receipts'] }) })
  const a = r.action
  const blocked = a.verdict === 'block' || a.approved === 'no'
  const who = a.source.startsWith('routine:') ? a.source.slice(8).split('#')[0] : 'exploração'
  return (
    <div className={cn('px-4 py-3', blocked && 'bg-danger-soft/40')}>
      <div className="flex items-center gap-3">
        <span className="w-12 shrink-0 text-[12px] tabular-nums text-ink-3">{new Date(r.ts).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}</span>
        <button onClick={() => setOpen(!open)} className="flex min-w-0 flex-1 items-center gap-2 text-left" aria-expanded={open}>
          {blocked && <ShieldAlert size={15} className="shrink-0 text-danger" />}
          <span className={cn('truncate text-[13.5px]', r.undone && 'text-ink-3 line-through')}>{blocked ? `Bloqueado: ${describe(a)}` : describe(a)}</span>
          <ChevronDown size={14} className={cn('shrink-0 text-ink-3 transition-transform', open && 'rotate-180')} />
        </button>
        <span className="hidden shrink-0 text-[12px] text-ink-3 sm:block">{who}</span>
        {a.dry_run && <span className="rounded-full bg-sunken px-2 py-0.5 text-[11px] text-ink-3">simulado</span>}
        {a.done === 'outbox.send_later' && !r.undone && r.undoable && <span className="rounded-full bg-change-soft px-2 py-0.5 text-[11px] text-change">sai às {new Date(r.undo_until!).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}</span>}
        {a.risk !== 'read' && !a.dry_run && <RiskBadge risk={a.risk} />}
        {r.undoable && (
          <Button size="sm" variant="ghost" onClick={() => undo.mutate()} disabled={undo.isPending} aria-label="Desfazer">
            <Undo2 size={14} /> Desfazer
          </Button>
        )}
        {r.undone && <span className="text-[12px] text-ink-3">desfeito</span>}
      </div>
      {undo.error && <p className="mt-1 pl-15 text-[12.5px] text-danger">{undo.error.message}</p>}
      {open && (
        <div className="mt-3 space-y-2 pl-15 text-[12.5px]">
          {a.reason && <div className="text-ink-2">Regra: “{a.reason}”</div>}
          {a.done && a.done !== a.capability && <div className="text-ink-2">Pedido: {a.capability} → feito de forma reversível: {a.done}</div>}
          {a.error && <div className="text-danger">{a.error}</div>}
          <pre className="max-h-64 overflow-auto rounded-lg bg-sunken p-3 font-mono text-[11.5px] text-ink-2">{JSON.stringify({ args: a.args, result: a.result }, null, 2)}</pre>
        </div>
      )}
    </div>
  )
}
