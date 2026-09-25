import { useQuery } from '@tanstack/react-query'
import { Card } from '../components/ui'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { hasKey, useT } from '../lib/i18n'

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
