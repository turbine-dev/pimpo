import { useQuery } from '@tanstack/react-query'
import { Card } from '../components/ui'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { hasKey, useT, type TKey } from '../lib/i18n'

export function Cost() {
  const t = useT()
  const q = useQuery({ queryKey: ['cost'], queryFn: api.cost })
  if (!q.data) return null
  const c = q.data
  const pct = c.limit > 0 ? Math.min(100, (c.today / c.limit) * 100) : 0
  const days = Array.from({ length: 14 }, (_, i) => {
    const d = new Date(Date.now() - (13 - i) * 86400000)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    return [key, c.by_day[key] ?? 0] as const
  })
  const spent = days.some(([, v]) => v > 0)
  const max = Math.max(0.0001, ...days.map(([, v]) => v))
  const sources = Object.entries(c.by_source).sort(([, a], [, b]) => b - a)
  const label = (k: string) => (k.startsWith('routine:') ? t('cost.routine', { name: k.slice(8) }) : hasKey(`cost.${k}`) ? t(`cost.${k}` as 'cost.rule') : k)
  return (
    <div className="mx-auto max-w-4xl space-y-4">
      <div>
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('cost.title')}</h1>
        <p className="text-sm text-ink-2">{t('cost.subtitle')}</p>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">{t('cost.today')}</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.today)}</div>
          {c.limit > 0 && (
            <>
              <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-sunken">
                <div className={cn('h-full rounded-full', pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read')} style={{ width: `${pct}%` }} />
              </div>
              <div className="mt-1.5 text-[12px] text-ink-3">{t('cost.perDay', { limit: usd(c.limit) })}</div>
            </>
          )}
        </Card>
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">{t('cost.month')}</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.month)}</div>
        </Card>
        <Card className="p-5">
          <div className="text-[12.5px] text-ink-3">{t('cost.projected')}</div>
          <div className="mt-1 text-[26px] font-semibold tabular-nums">{usd(c.projected_month)}</div>
          <div className="mt-1.5 text-[12px] text-ink-3">{t('cost.pace')}</div>
        </Card>
      </div>
      {(c.subscription?.month ?? 0) > 0 && (
        <Card className="p-5">
          <div className="flex flex-wrap items-baseline justify-between gap-3">
            <div className="text-[14px] font-medium">{t('cost.subTitle')}</div>
            <div className="text-[13px] tabular-nums text-ink-2">{t('cost.subAmounts', { today: usd(c.subscription!.today), month: usd(c.subscription!.month) })}</div>
          </div>
          <p className="mt-1 text-[12.5px] text-ink-3">{t('cost.subText')}</p>
          <ul className="mt-3 space-y-1 text-[13px]">
            {Object.entries(c.subscription!.by_model).sort(([, a], [, b]) => b - a).map(([k, v]) => (
              <li key={k} className="flex justify-between gap-3"><span>{modelName(k)} <span className="text-[11.5px] text-ink-3">{t('cost.calls', { count: c.calls_by_model?.[k] ?? 0 })}</span></span><span className="tabular-nums text-ink-3">≈ {usd(v)}</span></li>
            ))}
          </ul>
        </Card>
      )}
      <Card className="p-5">
        <div className="mb-4 text-[14px] font-medium">{t('cost.days')}</div>
        {!spent ? (
          <p className="text-sm text-ink-3">{t('cost.none')}</p>
        ) : (
          <div className="flex items-end gap-1.5">
            {days.map(([d, v]) => (
              <div key={d} className="group flex flex-1 flex-col items-center gap-1.5" title={`${d.slice(8)}/${d.slice(5, 7)}: ${usd(v)}`}>
                <span className={cn('text-[10.5px] tabular-nums text-ink-2 opacity-0 transition group-hover:opacity-100', v === max && 'opacity-100')}>{v > 0 ? usd(v) : ''}</span>
                <div className={cn('w-full rounded-t-md', v > 0 ? 'bg-accent/75' : 'bg-sunken')} style={{ height: `${v > 0 ? Math.max(6, (v / max) * 120) : 3}px` }} />
                <span className="text-[10.5px] tabular-nums text-ink-3">{d.slice(8)}</span>
              </div>
            ))}
          </div>
        )}
      </Card>
      {Object.keys(c.by_model ?? {}).length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Bars title={t('cost.byModel')} rows={Object.entries(c.by_model ?? {}).map(([k, v]) => ({ key: k, label: modelName(k), usd: v, sub: t('cost.calls', { count: c.calls_by_model?.[k] ?? 0 }) }))} />
          <Bars title={t('cost.byJob')} rows={Object.entries(c.by_job ?? {}).map(([k, v]) => ({ key: k, label: t(jobLabel[k] ?? 'cost.exploration'), usd: v }))} />
        </div>
      )}
      <Card className="divide-y divide-line">
        <div className="px-5 py-3 text-[14px] font-medium">{t('cost.where')}</div>
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

const jobLabel: Record<string, TKey> = { explore: 'models.explore', compile: 'models.compile', judge: 'ms.judge' }
const modelName = (id: string) => (['sonnet', 'opus', 'haiku'].includes(id) ? `Claude Code · ${id}` : id === 'codex' ? 'Codex · ChatGPT' : id)

// Bars compares where the month's spending went.
function Bars({ title, rows }: { title: string; rows: { key: string; label: string; usd: number; sub?: string }[] }) {
  const sorted = [...rows].sort((a, b) => b.usd - a.usd)
  const top = Math.max(0.0001, ...sorted.map((r) => r.usd))
  return (
    <Card className="p-5">
      <div className="mb-3 text-[14px] font-medium">{title}</div>
      <ul className="space-y-2.5">
        {sorted.map((r) => (
          <li key={r.key} className="text-[13px]">
            <div className="flex items-baseline justify-between gap-3">
              <span className="min-w-0 truncate">{r.label}{r.sub && <span className="ml-1.5 text-[11.5px] text-ink-3">{r.sub}</span>}</span>
              <span className="tabular-nums text-ink-2">{usd(r.usd)}</span>
            </div>
            <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-sunken"><div className="h-full rounded-full bg-accent/75" style={{ width: `${Math.max(2, (r.usd / top) * 100)}%` }} /></div>
          </li>
        ))}
      </ul>
    </Card>
  )
}
