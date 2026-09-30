import { AlertTriangle, ArrowDownRight, ArrowUpRight, CheckCircle2, ExternalLink, GripVertical, Home, Loader2, MoreHorizontal, OctagonAlert, PictureInPicture2, RefreshCw, Trash2, Users } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import type { WidgetSnap, WidgetView } from '../../lib/api'
import { cn } from '../../lib/cn'
import { relative, when } from '../../lib/format'
import { localeTag, useT, type TKey } from '../../lib/i18n'
import { BarChart, Donut, LineChart, palette, Ring, Sparkline } from './charts'

// A value in the person's language: money for a currency code, a percent,
// or a number with its unit; big numbers get short.
export function formatValue(v: number, unit?: string) {
  const tag = localeTag()
  if (unit && /^[A-Z]{3}$/.test(unit)) {
    try {
      const digits = Math.abs(v) >= 1000 ? 0 : Math.abs(v) < 1 && v !== 0 ? 3 : 2
      return new Intl.NumberFormat(tag, { style: 'currency', currency: unit, maximumFractionDigits: digits, minimumFractionDigits: Math.min(digits, 2) }).format(v)
    } catch { /* not a currency */ }
  }
  const n = new Intl.NumberFormat(tag, Math.abs(v) >= 100000 ? { notation: 'compact', maximumFractionDigits: 1 } : { maximumFractionDigits: Math.abs(v) < 10 ? 2 : 1 }).format(v)
  if (!unit) return n
  return unit === '%' ? `${n}%` : `${n} ${unit}`
}

const statusStyle = {
  ok: { color: 'var(--color-read)', soft: 'bg-read-soft text-read', Icon: CheckCircle2 },
  warn: { color: 'var(--color-change)', soft: 'bg-change-soft text-change', Icon: AlertTriangle },
  alert: { color: 'var(--color-danger)', soft: 'bg-danger-soft text-danger', Icon: OctagonAlert },
} as const

function Dot({ status }: { status?: string }) {
  if (!status || !(status in statusStyle)) return <span className="size-2 shrink-0 rounded-full bg-line-strong" />
  return <span className="size-2 shrink-0 rounded-full" style={{ background: statusStyle[status as keyof typeof statusStyle].color }} />
}

function Trend({ trend }: { trend?: number }) {
  if (trend === undefined || trend === null) return null
  const up = trend >= 0
  const Arrow = up ? ArrowUpRight : ArrowDownRight
  return (
    <span className={cn('inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[12px] font-semibold tabular-nums', up ? 'bg-read-soft text-read' : 'bg-danger-soft text-danger')}>
      <Arrow size={13} />{Math.abs(trend).toLocaleString(localeTag(), { maximumFractionDigits: 1 })}%
    </span>
  )
}

// The words the app uses for built-in and status widgets.
function useWords() {
  const t = useT()
  return {
    title(w: WidgetView) {
      const k = `widget.title.${w.id.replace('builtin:', '')}` as TKey
      return w.source === 'builtin' ? t(k) : w.title
    },
    detail(d?: string, value?: string) {
      if (d === 'next') return t('widget.next', { when: when(value) })
      if (d === 'at') return when(value)
      if (d === 'ok' || d === 'failed' || d === 'skipped' || d === 'running') return t(`widget.outcome.${d}` as TKey)
      return d
    },
  }
}

function isTime(v?: string) {
  return !!v && /^\d{4}-\d{2}-\d{2}T/.test(v)
}

// WidgetBody draws a widget's snapshot by its kind.
export function WidgetBody({ w, size }: { w: WidgetView; size: 'small' | 'wide' }) {
  const t = useT()
  const words = useWords()
  const s = w.snapshot as WidgetSnap
  const fmt = (v: number) => formatValue(v, s.unit)
  switch (s.kind) {
    case 'metric': {
      const hist = (w.history ?? []).map((p) => p.v)
      return (
        <div className="flex h-full min-h-0 flex-col justify-between">
          <div>
            <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
              <span className="font-semibold leading-none tracking-tight tabular-nums" style={{ fontSize: size === 'small' ? 'clamp(20px, 15cqw, 34px)' : 'clamp(22px, 13cqw, 44px)' }}>{fmt(s.value ?? 0)}</span>
              <Trend trend={s.trend} />
            </div>
            {(s.subtitle || s.meta?.nolimit) && <p className="mt-1.5 text-[12.5px] text-ink-3">{s.meta?.nolimit ? t('widget.noLimit') : s.subtitle}</p>}
          </div>
          {hist.length > 1 && <Sparkline values={hist} color={s.trend !== undefined && s.trend < 0 ? 'var(--color-chart-4)' : 'var(--color-chart-1)'} className="mt-2 h-12 min-h-0 flex-1" />}
        </div>
      )
    }
    case 'progress': {
      const f = (s.value ?? 0) / (s.goal || 1)
      const color = f >= 1 ? 'var(--color-danger)' : f >= 0.8 ? 'var(--color-change)' : 'var(--color-read)'
      return (
        <div className="flex h-full min-h-0 items-center gap-4 @max-[300px]:flex-col @max-[300px]:justify-center @max-[300px]:gap-2 @max-[300px]:text-center">
          <Ring fraction={f} color={color}>
            <span className="text-[17px] font-semibold tabular-nums">{Math.round(f * 100)}%</span>
          </Ring>
          <div className="min-w-0">
            <div className="font-semibold tabular-nums leading-tight" style={{ fontSize: 'clamp(16px, 9cqw, 24px)' }}>{fmt(s.value ?? 0)}</div>
            <div className="text-[12.5px] text-ink-3">{t('widget.ofGoal', { goal: fmt(s.goal ?? 0) })}</div>
            {s.subtitle && <p className="mt-1 text-[12.5px] text-ink-2">{s.subtitle}</p>}
          </div>
        </div>
      )
    }
    case 'status': {
      const st = statusStyle[(s.status as keyof typeof statusStyle) ?? 'ok'] ?? statusStyle.ok
      const bars = s.series?.[0]?.points ?? []
      const line = w.source === 'status'
        ? [s.meta?.last && t('widget.lastRun', { when: relative(s.meta.last), outcome: words.detail(s.meta.outcome) ?? '' }), s.meta?.next && t('widget.next', { when: when(s.meta.next) })].filter(Boolean).join(' · ') || t('widget.noRuns')
        : s.subtitle
      return (
        <div className="flex h-full min-h-0 flex-col">
          <div className="flex items-center gap-3">
            <span className={cn('grid size-11 shrink-0 place-items-center rounded-2xl', st.soft)}><st.Icon size={22} /></span>
            <div className="min-w-0">
              <div className="flex items-baseline gap-2">
                {s.value !== undefined && s.value !== null && <span className="text-[26px] font-semibold leading-none tabular-nums">{w.source === 'status' ? `${Math.round(s.value)}%` : formatValue(s.value)}</span>}
                <span className={cn('rounded-full px-2 py-0.5 text-[11.5px] font-semibold', st.soft)}>{t(`widget.status.${s.status ?? 'ok'}` as TKey)}</span>
              </div>
              {line && <p className="mt-1 truncate text-[12.5px] text-ink-3">{line}</p>}
            </div>
          </div>
          {bars.length > 0 && (
            <div className="mt-3 flex h-6 items-end gap-[3px]" aria-hidden>
              {bars.map((b, i) => <span key={i} title={when(b.label)} className="flex-1 rounded-sm" style={{ height: b.y ? '100%' : '45%', background: b.y ? 'var(--color-read)' : 'var(--color-danger)', opacity: 0.85 }} />)}
            </div>
          )}
          {!!s.items?.length && <ItemList items={s.items} dense />}
          {s.text && w.source !== 'status' && <p className="mt-2 text-[13px] text-ink-2">{s.text}</p>}
        </div>
      )
    }
    case 'text':
      if (s.meta?.empty) return <Empty text={t(`widget.empty.${s.meta.empty}` as TKey)} />
      return <p className="whitespace-pre-wrap text-[14.5px] leading-relaxed text-ink-2">{s.text}</p>
    case 'list':
      return <ItemList items={s.items ?? []} />
    case 'table':
      return (
        <div className="-mx-1 h-full min-h-0 overflow-auto">
          <table className="w-full border-separate border-spacing-0 text-[13px]">
            <thead className="sticky top-0 z-[1]">
              <tr>{(s.columns ?? []).map((c, i) => <th key={i} className={cn('border-b border-line bg-surface px-2 py-1.5 text-left text-[11.5px] font-semibold uppercase tracking-wide text-ink-3', numeric(s.rows, i) && 'text-right')}>{c}</th>)}</tr>
            </thead>
            <tbody>
              {(s.rows ?? []).map((r, i) => (
                <tr key={i} className="even:bg-sunken/60 hover:bg-sunken">
                  {r.map((v, j) => <td key={j} className={cn('px-2 py-1.5 text-ink-2 first:rounded-l-md first:font-medium first:text-ink last:rounded-r-md', numeric(s.rows, j) && 'text-right tabular-nums')}>{v}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    case 'chart': {
      const series = s.series ?? []
      const head = s.value !== undefined && s.value !== null && (
        <div className="mb-2 flex items-baseline gap-2">
          <span className="text-[22px] font-semibold tabular-nums leading-none">{fmt(s.value)}</span>
          <Trend trend={s.trend} />
        </div>
      )
      return (
        <div className="flex h-full min-h-0 flex-col">
          {head}
          <div className="min-h-0 flex-1">
            {s.chart === 'bar' ? <BarChart series={series} format={fmt} /> : s.chart === 'donut' ? <Donut series={series} format={fmt} /> : <LineChart series={series} area={s.chart === 'area'} format={fmt} />}
          </div>
          {series.length > 1 && s.chart !== 'donut' && (
            <div className="mt-2 flex flex-wrap gap-3 text-[12px] text-ink-3">
              {series.map((x, i) => <span key={i} className="flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette[i % palette.length] }} />{x.name}</span>)}
            </div>
          )}
        </div>
      )
    }
  }
  return null
}

function numeric(rows: string[][] | undefined, col: number) {
  const vals = (rows ?? []).map((r) => r[col]).filter((v) => v !== undefined && v !== '')
  return vals.length > 0 && vals.every((v) => /^[-+]?[\d.,\s]+%?$|^[A-Z$€£R]{0,3}\s?[-\d.,]+$/.test(v))
}

function Empty({ text }: { text: string }) {
  return <div className="grid h-full place-items-center text-center text-[13.5px] text-ink-3">{text}</div>
}

function ItemList({ items, dense }: { items: NonNullable<WidgetSnap['items']>; dense?: boolean }) {
  const words = useWords()
  return (
    <ul className={cn('min-h-0 overflow-auto', dense ? 'mt-3 space-y-1' : '-mx-1 h-full space-y-0.5')}>
      {items.map((it, i) => {
        const value = isTime(it.value) ? relative(it.value) : it.value
        const detail = words.detail(it.detail, it.value)
        const body = (
          <>
            <Dot status={it.status} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[13.5px] font-medium text-ink">{it.title}</span>
              {detail && !dense && <span className="block truncate text-[12px] text-ink-3">{detail}</span>}
            </span>
            {it.badge && <span className="shrink-0 rounded-full bg-sunken px-2 py-0.5 text-[11px] font-medium text-ink-2">{it.badge}</span>}
            {value && it.detail !== 'next' && it.detail !== 'at' && <span className="shrink-0 text-[12.5px] tabular-nums text-ink-3">{value}</span>}
          </>
        )
        return (
          <li key={i}>
            {it.link
              ? <a href={it.link} target="_blank" rel="noreferrer" className="flex items-center gap-2.5 rounded-lg px-1.5 py-1.5 hover:bg-sunken">{body}</a>
              : <div className="flex items-center gap-2.5 rounded-lg px-1.5 py-1.5">{body}</div>}
          </li>
        )
      })}
    </ul>
  )
}

// WidgetCard frames a widget: its title, when it was updated, and what can
// be done with it.
export function WidgetCard({ w, editing, onRemove, onRefresh, onShare, onFloat, floating, refreshing, drag, children }: {
  w: WidgetView | { id: string; hidden: true }; editing?: boolean; refreshing?: boolean; floating?: boolean
  onRemove?: () => void; onRefresh?: () => void; onShare?: (shared: boolean) => void; onFloat?: (on: boolean) => void
  drag?: ReactNode; children?: ReactNode
}) {
  const t = useT()
  const words = useWords()
  const [menu, setMenu] = useState(false)
  if ('hidden' in w) {
    return (
      <div className="flex h-full flex-col items-center justify-center rounded-[20px] border border-dashed border-line bg-surface/60 p-4 text-center text-[13px] text-ink-3">
        {drag}
        <Users size={18} className="mb-1.5" />{t('widget.private')}
        {editing && onRemove && <button type="button" onClick={onRemove} className="mt-2 text-[12.5px] underline">{t('widget.remove')}</button>}
      </div>
    )
  }
  const s = w.snapshot as WidgetSnap
  const link = s.link
  return (
    <div className={cn('@container group relative flex h-full min-h-0 flex-col overflow-hidden rounded-[20px] border border-line bg-surface p-4 shadow-[var(--shadow-card)] transition-shadow hover:shadow-[var(--shadow-pop)]', editing && 'ring-2 ring-explore/30')}>
      <div className="mb-3 flex items-start gap-2">
        {drag}
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[13.5px] font-semibold text-ink-2">{words.title(w)}</h3>
        </div>
        {w.shared && <span title={t('widget.sharedHint')} className="text-ink-3"><Home size={14} /></span>}
        {link && <a href={link} target="_blank" rel="noreferrer" aria-label={t('widget.open')} className="text-ink-3 hover:text-ink"><ExternalLink size={14} /></a>}
        {(onRefresh || onRemove || onShare || onFloat) && (
          <div className="relative">
            <button type="button" onClick={() => setMenu(!menu)} aria-label={t('widget.menu')} aria-expanded={menu} className="grid size-7 place-items-center rounded-lg text-ink-3 opacity-70 hover:bg-sunken hover:text-ink group-hover:opacity-100">
              {refreshing ? <Loader2 size={15} className="animate-spin" /> : <MoreHorizontal size={16} />}
            </button>
            {menu && (
              <div role="menu" className="absolute right-0 top-8 z-20 w-56 rounded-xl border border-line bg-raised p-1 shadow-[var(--shadow-pop)]" onMouseLeave={() => setMenu(false)}>
                {onRefresh && <button role="menuitem" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] hover:bg-sunken" onClick={() => { setMenu(false); onRefresh() }}><RefreshCw size={14} />{t('widget.refresh')}</button>}
                {onFloat && <button role="menuitem" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] hover:bg-sunken" onClick={() => { setMenu(false); onFloat(!floating) }}><PictureInPicture2 size={14} />{floating ? t('float.stop') : t('float.start')}</button>}
                {onShare && <button role="menuitem" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] hover:bg-sunken" onClick={() => { setMenu(false); onShare(!w.shared) }}><Home size={14} />{w.shared ? t('widget.unshare') : t('widget.share')}</button>}
                {onRemove && <button role="menuitem" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] text-danger hover:bg-danger-soft" onClick={() => { setMenu(false); onRemove() }}><Trash2 size={14} />{t('widget.remove')}</button>}
              </div>
            )}
          </div>
        )}
      </div>
      <div className="min-h-0 flex-1">{children}</div>
      <div className="mt-2 flex items-center gap-1.5 text-[11px] text-ink-3">
        {w.stale && <span className="rounded-full bg-change-soft px-1.5 py-0.5 font-medium text-change">{t('widget.stale')}</span>}
        <span>{relative(w.updated)}</span>
      </div>
    </div>
  )
}

export function DragHandle(props: React.HTMLAttributes<HTMLSpanElement>) {
  return <span {...props} className="-ml-1 mt-0.5 cursor-grab touch-none text-ink-3 active:cursor-grabbing"><GripVertical size={15} /></span>
}
