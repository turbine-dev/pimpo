import { useQuery } from '@tanstack/react-query'
import { Card } from '../components/ui'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'

export function Cost() {
  const q = useQuery({ queryKey: ['cost'], queryFn: api.cost })
  if (!q.data) return null
  const c = q.data
  const pct = c.limit > 0 ? Math.min(100, (c.today / c.limit) * 100) : 0
  const days = Object.entries(c.by_day).sort(([a], [b]) => a.localeCompare(b)).slice(-14)
  const max = Math.max(0.0001, ...days.map(([, v]) => v))
  const sources = Object.entries(c.by_source).sort(([, a], [, b]) => b - a)
  const label = (k: string) => (k.startsWith('routine:') ? `Rotina ${k.slice(8)}` : { exploration: 'Explorações', compile: 'Compilações', judgment: 'Julgamentos', rule: 'Regras' }[k] ?? k)
  return (
    <div className="mx-auto max-w-4xl space-y-4">
      <div>
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Custo</h1>
        <p className="text-sm text-ink-2">Quanto os modelos custaram. Rotinas compiladas quase não gastam: o custo fica nas explorações e compilações.</p>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">Hoje</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.today)}</div>
          {c.limit > 0 && (
            <>
              <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-sunken">
                <div className={cn('h-full rounded-full', pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read')} style={{ width: `${pct}%` }} />
              </div>
              <div className="mt-1.5 text-[12px] text-ink-3">de {usd(c.limit)} por dia</div>
            </>
          )}
        </Card>
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">Este mês</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.month)}</div>
        </Card>
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">Projeção do mês</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.projected_month)}</div>
          <div className="mt-1.5 text-[12px] text-ink-3">no ritmo atual</div>
        </Card>
      </div>
      <Card className="p-5">
        <div className="mb-4 text-[14px] font-medium">Últimos dias</div>
        {days.length === 0 ? (
          <p className="text-sm text-ink-3">Nenhum gasto ainda este mês.</p>
        ) : (
          <div className="flex h-36 items-end gap-2">
            {days.map(([d, v]) => (
              <div key={d} className="flex flex-1 flex-col items-center gap-1.5" title={`${d}: ${usd(v)}`}>
                <div className="w-full rounded-t-md bg-accent/70" style={{ height: `${Math.max(3, (v / max) * 100)}%` }} />
                <span className="text-[10.5px] text-ink-3">{d.slice(8)}</span>
              </div>
            ))}
          </div>
        )}
      </Card>
      <Card className="divide-y divide-line">
        <div className="px-5 py-3 text-[14px] font-medium">Para onde foi</div>
        {sources.map(([k, v]) => (
          <div key={k} className="flex items-center justify-between px-5 py-2.5 text-[13.5px]">
            <span>{label(k)}</span>
            <span className="tabular-nums text-ink-2">{usd(v)}</span>
          </div>
        ))}
      </Card>
    </div>
  )
}
