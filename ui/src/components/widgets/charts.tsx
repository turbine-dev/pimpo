import { useId, useMemo, useState, type PointerEvent } from 'react'
import { cn } from '../../lib/cn'

// Charts for widgets, drawn in SVG with the app's colours. Each fills its
// box, keeps thin lines thin at any size, and says the value under the
// pointer.

export const palette = ['var(--color-chart-1)', 'var(--color-chart-2)', 'var(--color-chart-3)', 'var(--color-chart-4)', 'var(--color-chart-5)']

export type Point = { label?: string; y: number }
export type Series = { name?: string; points: Point[] }

const W = 300
const H = 100

// smooth turns points into a gentle curve (Catmull-Rom as Béziers) that
// never overshoots the drawing box.
function smooth(pts: [number, number][]) {
  if (pts.length < 2) return pts.length ? `M${pts[0][0]},${pts[0][1]}` : ''
  let d = `M${pts[0][0]},${pts[0][1]}`
  for (let i = 0; i < pts.length - 1; i++) {
    const [x0, y0] = pts[Math.max(0, i - 1)]
    const [x1, y1] = pts[i]
    const [x2, y2] = pts[i + 1]
    const [x3, y3] = pts[Math.min(pts.length - 1, i + 2)]
    const c1x = x1 + (x2 - x0) / 6
    const c1y = Math.min(H, Math.max(0, y1 + (y2 - y0) / 6))
    const c2x = x2 - (x3 - x1) / 6
    const c2y = Math.min(H, Math.max(0, y2 - (y3 - y1) / 6))
    d += ` C${c1x},${c1y} ${c2x},${c2y} ${x2},${y2}`
  }
  return d
}

function bounds(series: Series[]) {
  const ys = series.flatMap((s) => s.points.map((p) => p.y))
  let lo = Math.min(...ys, 0)
  let hi = Math.max(...ys, 0)
  if (hi === lo) hi = lo + 1
  const pad = (hi - lo) * 0.08
  return { lo: lo < 0 ? lo - pad : lo, hi: hi + pad }
}

function Tip({ x, label, lines }: { x: number; label?: string; lines: { color: string; text: string }[] }) {
  return (
    <div className="pointer-events-none absolute top-1 z-10 -translate-x-1/2 whitespace-nowrap rounded-lg border border-line bg-raised px-2 py-1 text-[11.5px] shadow-[var(--shadow-pop)]" style={{ left: `${Math.min(88, Math.max(12, x))}%` }}>
      {label && <div className="text-ink-3">{label}</div>}
      {lines.map((l, i) => <div key={i} className="flex items-center gap-1.5 font-medium tabular-nums text-ink"><span className="size-2 rounded-full" style={{ background: l.color }} />{l.text}</div>)}
    </div>
  )
}

// LineChart draws one or more series as lines, with the area under the
// first filled when area is set.
export function LineChart({ series, area, format, className }: { series: Series[]; area?: boolean; format: (v: number) => string; className?: string }) {
  const id = useId()
  const [hover, setHover] = useState<number | null>(null)
  const n = Math.max(...series.map((s) => s.points.length))
  const { lo, hi } = useMemo(() => bounds(series), [series])
  const xOf = (i: number) => (n <= 1 ? W / 2 : (i / (n - 1)) * W)
  const yOf = (v: number) => H - ((v - lo) / (hi - lo)) * H
  const move = (e: PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    setHover(Math.round(((e.clientX - r.left) / r.width) * (n - 1)))
  }
  const labels = series[0]?.points.map((p) => p.label ?? '') ?? []
  return (
    <div className={cn('relative flex h-full min-h-0 flex-col', className)}>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-full min-h-0 w-full flex-1 overflow-visible" onPointerMove={move} onPointerLeave={() => setHover(null)} aria-hidden="true">
        <defs>
          {series.map((_, k) => (
            <linearGradient key={k} id={`${id}-g${k}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={palette[k % palette.length]} stopOpacity={0.28} />
              <stop offset="100%" stopColor={palette[k % palette.length]} stopOpacity={0} />
            </linearGradient>
          ))}
        </defs>
        {[0.25, 0.5, 0.75].map((f) => <line key={f} x1={0} x2={W} y1={H * f} y2={H * f} stroke="var(--color-line)" strokeDasharray="2 4" vectorEffect="non-scaling-stroke" />)}
        {lo < 0 && <line x1={0} x2={W} y1={yOf(0)} y2={yOf(0)} stroke="var(--color-line-strong)" vectorEffect="non-scaling-stroke" />}
        {series.map((s, k) => {
          const pts = s.points.map((p, i) => [xOf(i), yOf(p.y)] as [number, number])
          const line = smooth(pts)
          return (
            <g key={k}>
              {(area || k === 0 && series.length === 1) && pts.length > 1 && (
                <path d={`${line} L${pts[pts.length - 1][0]},${H} L${pts[0][0]},${H} Z`} fill={`url(#${id}-g${k})`} opacity={area ? 1 : 0.6} />
              )}
              <path d={line} fill="none" stroke={palette[k % palette.length]} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
            </g>
          )
        })}
        {hover !== null && hover >= 0 && hover < n && (
          <line x1={xOf(hover)} x2={xOf(hover)} y1={0} y2={H} stroke="var(--color-ink-3)" strokeDasharray="3 3" vectorEffect="non-scaling-stroke" />
        )}
      </svg>
      {hover !== null && hover >= 0 && hover < n && (
        <>
          {series.map((s, k) => s.points[hover] && (
            <span key={k} className="pointer-events-none absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface" style={{ left: `${(xOf(hover) / W) * 100}%`, top: `${(yOf(s.points[hover].y) / H) * 100}%`, background: palette[k % palette.length] }} />
          ))}
          <Tip x={(xOf(hover) / W) * 100} label={labels[hover]} lines={series.filter((s) => s.points[hover]).map((s, k) => ({ color: palette[k % palette.length], text: (s.name ? s.name + ': ' : '') + format(s.points[hover].y) }))} />
        </>
      )}
      <SrValues series={series} format={format} />
      {labels.some(Boolean) && (
        <div className="mt-1 flex justify-between text-[10.5px] text-ink-3 tabular-nums" aria-hidden="true">
          <span>{labels[0]}</span>
          {n > 2 && <span>{labels[Math.floor((n - 1) / 2)]}</span>}
          <span>{labels[n - 1]}</span>
        </div>
      )}
    </div>
  )
}

// BarChart draws the first series as rounded bars; more series stack
// side by side.
export function BarChart({ series, format, className }: { series: Series[]; format: (v: number) => string; className?: string }) {
  const [hover, setHover] = useState<number | null>(null)
  const n = Math.max(...series.map((s) => s.points.length))
  const { lo, hi } = useMemo(() => bounds(series), [series])
  const labels = series[0]?.points.map((p) => p.label ?? '') ?? []
  const slot = W / n
  const gap = Math.min(slot * 0.28, 6)
  const bw = (slot - gap) / series.length
  const yOf = (v: number) => H - ((v - lo) / (hi - lo)) * H
  return (
    <div className="relative flex h-full min-h-0 flex-col">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className={cn('h-full min-h-0 w-full flex-1', className)} onPointerLeave={() => setHover(null)} aria-hidden="true">
        {[0.25, 0.5, 0.75].map((f) => <line key={f} x1={0} x2={W} y1={H * f} y2={H * f} stroke="var(--color-line)" strokeDasharray="2 4" vectorEffect="non-scaling-stroke" />)}
        <line x1={0} x2={W} y1={yOf(0)} y2={yOf(0)} stroke="var(--color-line-strong)" vectorEffect="non-scaling-stroke" />
        {Array.from({ length: n }, (_, i) => (
          <g key={i} onPointerEnter={() => setHover(i)}>
            <rect x={i * slot} y={0} width={slot} height={H} fill="transparent" />
            {series.map((s, k) => {
              const v = s.points[i]?.y ?? 0
              const top = Math.min(yOf(v), yOf(0))
              const h = Math.max(Math.abs(yOf(v) - yOf(0)), v === 0 ? 0 : 1.2)
              return <rect key={k} x={i * slot + gap / 2 + k * bw} y={top} width={Math.max(bw - 0.6, 0.6)} height={h} rx={Math.min(bw / 3, 3)} fill={palette[k % palette.length]} opacity={hover === null || hover === i ? 1 : 0.45} />
            })}
          </g>
        ))}
      </svg>
      {hover !== null && (
        <Tip x={((hover + 0.5) / n) * 100} label={labels[hover]} lines={series.filter((s) => s.points[hover]).map((s, k) => ({ color: palette[k % palette.length], text: (s.name ? s.name + ': ' : '') + format(s.points[hover].y) }))} />
      )}
      <SrValues series={series} format={format} />
      {labels.some(Boolean) && (
        <div className="mt-1 flex justify-between text-[10.5px] text-ink-3 tabular-nums" aria-hidden="true">
          <span>{labels[0]}</span>
          <span>{labels[n - 1]}</span>
        </div>
      )}
    </div>
  )
}

// SrValues says a chart's values to screen readers, which skip the drawing.
function SrValues({ series, format }: { series: Series[]; format: (v: number) => string }) {
  return (
    <ul className="sr-only">
      {series.map((s, k) => s.points.map((p, i) => <li key={`${k}-${i}`}>{[s.name, p.label, format(p.y)].filter(Boolean).join(': ')}</li>))}
    </ul>
  )
}

// Donut draws the first series' points as slices, with a legend.
export function Donut({ series, format }: { series: Series[]; format: (v: number) => string }) {
  const [hover, setHover] = useState<number | null>(null)
  const pts = (series[0]?.points ?? []).filter((p) => p.y > 0)
  const total = pts.reduce((a, p) => a + p.y, 0) || 1
  const R = 42
  const C = 2 * Math.PI * R
  let offset = 0
  return (
    <div className="flex h-full min-h-0 items-center gap-4 @max-[340px]:flex-col @max-[340px]:gap-2">
      <div className="relative aspect-square h-full max-h-40 min-h-20 shrink-0 @max-[340px]:h-auto @max-[340px]:min-h-0 @max-[340px]:w-1/2">
        <svg viewBox="0 0 100 100" className="size-full -rotate-90" aria-hidden="true">
          <circle cx={50} cy={50} r={R} fill="none" stroke="var(--color-sunken)" strokeWidth={12} />
          {pts.map((p, i) => {
            const len = (p.y / total) * C
            const el = <circle key={i} cx={50} cy={50} r={R} fill="none" stroke={palette[i % palette.length]} strokeWidth={hover === i ? 15 : 12} strokeDasharray={`${Math.max(len - 1.2, 0.1)} ${C}`} strokeDashoffset={-offset} strokeLinecap="butt" onPointerEnter={() => setHover(i)} onPointerLeave={() => setHover(null)} className="transition-[stroke-width]" />
            offset += len
            return el
          })}
        </svg>
        <div className="absolute inset-0 grid place-items-center text-center">
          <div>
            <div className="font-semibold tabular-nums leading-tight" style={{ fontSize: 'clamp(11px, 5cqw, 15px)' }}>{format(hover === null ? total : pts[hover].y)}</div>
            <div className="max-w-16 truncate text-[10.5px] text-ink-3">{hover === null ? '' : pts[hover].label}</div>
          </div>
        </div>
      </div>
      <ul className="min-w-0 flex-1 space-y-1 self-stretch text-[12.5px] @max-[340px]:flex @max-[340px]:flex-wrap @max-[340px]:gap-x-3 @max-[340px]:space-y-0">
        {pts.slice(0, 6).map((p, i) => (
          <li key={i} className={cn('flex items-center gap-2', hover !== null && hover !== i && 'opacity-50')} onPointerEnter={() => setHover(i)} onPointerLeave={() => setHover(null)}>
            <span className="size-2.5 shrink-0 rounded-full" style={{ background: palette[i % palette.length] }} />
            <span className="min-w-0 flex-1 truncate text-ink-2">{p.label}</span>
            <span className="tabular-nums text-ink-3">{Math.round((p.y / total) * 100)}%</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

// Sparkline is a tiny line with its area, for a metric's history.
export function Sparkline({ values, color = 'var(--color-chart-1)', className }: { values: number[]; color?: string; className?: string }) {
  const id = useId()
  if (values.length < 2) return null
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const span = hi - lo || 1
  const pts = values.map((v, i) => [(i / (values.length - 1)) * W, H - 8 - ((v - lo) / span) * (H - 16)] as [number, number])
  const line = smooth(pts)
  return (
    <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className={cn('w-full', className)} aria-hidden>
      <defs>
        <linearGradient id={`${id}-s`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.3} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={`${line} L${W},${H} L0,${H} Z`} fill={`url(#${id}-s)`} />
      <path d={line} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

// Ring is a progress circle.
export function Ring({ fraction, color, children }: { fraction: number; color: string; children?: React.ReactNode }) {
  const R = 42
  const C = 2 * Math.PI * R
  const f = Math.max(0, Math.min(1, fraction))
  return (
    <div className="relative aspect-square h-full max-h-36 min-h-16 shrink-0 @max-[300px]:h-auto @max-[300px]:max-h-24 @max-[300px]:w-24">
      <svg viewBox="0 0 100 100" className="size-full -rotate-90" aria-hidden>
        <circle cx={50} cy={50} r={R} fill="none" stroke="var(--color-sunken)" strokeWidth={10} />
        {f > 0 && <circle cx={50} cy={50} r={R} fill="none" stroke={color} strokeWidth={10} strokeDasharray={`${f * C} ${C}`} strokeLinecap="round" className="transition-[stroke-dasharray] duration-700" />}
      </svg>
      <div className="absolute inset-0 grid place-items-center">{children}</div>
    </div>
  )
}
