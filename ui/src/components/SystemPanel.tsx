import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { api, type SysComponent, type SystemState } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'

const gb = (b: number) => (b >= 1e12 ? `${(b / 1e12).toFixed(1)} TB` : `${(b / 1e9).toFixed(b >= 1e10 ? 0 : 1)} GB`)
const mb = (b: number) => `${Math.round(b / 1e6)} MB`

function uptime(s: number) {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h} h ${m} min` : `${m} min`
}

function Spark({ values, max }: { values: number[]; max?: number }) {
  if (values.length < 2) return <div className="h-10" />
  const top = Math.max(max ?? 0, ...values, 0.0001)
  const pts = values.map((v, i) => `${(i / (values.length - 1)) * 100},${40 - (v / top) * 36}`).join(' ')
  return (
    <svg viewBox="0 0 100 40" preserveAspectRatio="none" className="h-10 w-full" aria-hidden>
      <polygon points={`0,40 ${pts} 100,40`} className="fill-accent/15" />
      <polyline points={pts} className="fill-none stroke-accent" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function Meter({ label, value, sub, history, max, pct }: { label: string; value: string; sub?: string; history?: number[]; max?: number; pct?: number }) {
  return (
    <div className="rounded-xl border border-line bg-bg/40 p-3.5">
      <div className="text-[11.5px] font-medium uppercase tracking-wide text-ink-3">{label}</div>
      <div className="mt-1 text-[22px] font-semibold tabular-nums">{value}</div>
      {sub && <div className="text-[12px] text-ink-3">{sub}</div>}
      {pct !== undefined && (
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-sunken">
          <div className={cn('h-full rounded-full', pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read')} style={{ width: `${Math.min(100, pct)}%` }} />
        </div>
      )}
      {history && <div className="mt-2"><Spark values={history} max={max} /></div>}
    </div>
  )
}

const dot: Record<SysComponent['state'], string> = { ok: 'bg-read', off: 'bg-line-strong', error: 'bg-danger', waiting: 'bg-change' }
const groups: [SysComponent['group'], TKey][] = [['channel', 'sys.channels'], ['account', 'sys.accounts'], ['brain', 'sys.brain'], ['service', 'sys.services'], ['access', 'sys.access'], ['backup', 'sys.backup']]

// SystemPanel is the live view of how busy the computer and Zodim are and
// whether every part is working.
export function SystemPanel({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const t = useT()
  const q = useQuery({ queryKey: ['system'], queryFn: api.system, enabled: open, refetchInterval: open ? 2000 : false })
  const [hist, setHist] = useState<{ cpu: number[]; heap: number[]; load: number[] }>({ cpu: [], heap: [], load: [] })
  const last = useRef<SystemState>(undefined)
  useEffect(() => {
    const d = q.data
    if (!d || d === last.current) return
    last.current = d
    setHist((h) => ({
      cpu: [...h.cpu, d.process.cpu_percent].slice(-60),
      heap: [...h.heap, d.process.heap_bytes].slice(-60),
      load: [...h.load, d.host.load[0]].slice(-60),
    }))
  }, [q.data])
  const d = q.data
  const hostPct = d ? (d.host.load[0] / Math.max(1, d.host.cpus)) * 100 : 0
  const diskUsed = d && d.host.disk_size ? ((d.host.disk_size - d.host.disk_free) / d.host.disk_size) * 100 : 0

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[5vh] z-50 max-h-[90vh] w-[min(760px,calc(100vw-24px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-5 flex items-start justify-between gap-4">
            <div>
              <div className="text-[11.5px] font-semibold uppercase tracking-wide text-ink-3">{t('sys.live')}</div>
              <Dialog.Title className="text-[19px] font-semibold tracking-tight">{t('sys.title')}</Dialog.Title>
            </div>
            <Dialog.Close className="grid size-8 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <Dialog.Description className="sr-only">{t('sys.text')}</Dialog.Description>
          {!d ? <p className="text-[13px] text-ink-3">{q.error ? q.error.message : t('sys.loading')}</p> : <>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Meter label={t('sys.computer')} value={hostPct > 100 ? '100%+' : `${Math.round(hostPct)}%`} sub={t('sys.load', { load: d.host.load[0].toFixed(1), cpus: d.host.cpus })} history={hist.load} max={d.host.cpus} />
              <Meter label={t('sys.zodimCpu')} value={`${d.process.cpu_percent.toFixed(1)}%`} sub={t('sys.goroutines', { n: d.process.goroutines })} history={hist.cpu} max={5} />
              <Meter label={t('sys.memory')} value={mb(d.process.heap_bytes)} sub={t('sys.memoryOf', { sys: mb(d.process.sys_bytes), total: d.host.mem_total ? gb(d.host.mem_total) : '—' })} history={hist.heap} />
              <Meter label={t('sys.disk')} value={t('sys.free', { free: gb(d.host.disk_free) })} sub={t('sys.of', { total: gb(d.host.disk_size) })} pct={diskUsed} />
            </div>
            {hostPct > 90 && <p className="mt-3 rounded-lg bg-change-soft px-3 py-2 text-[12.5px] text-change">{t('sys.busy')}</p>}
            {d.host.disk_size > 0 && d.host.disk_free < 10e9 && <p className="mt-3 rounded-lg bg-danger-soft px-3 py-2 text-[12.5px] text-danger">{t('sys.lowDisk')}</p>}

            <h3 className="mb-2 mt-6 text-[12px] font-semibold uppercase tracking-wide text-ink-3">{t('sys.now')}</h3>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
              {([['sys.exploring', d.activity.explorations], ['sys.running', d.activity.runs], ['sys.approvals', d.activity.approvals], ['sys.okToday', d.activity.runs_ok_today], ['sys.failedToday', d.activity.runs_failed_today]] as [TKey, number][]).map(([k, v]) => (
                <div key={k} className="rounded-xl border border-line px-3 py-2.5">
                  <div className={cn('text-[18px] font-semibold tabular-nums', k === 'sys.failedToday' && v > 0 && 'text-danger')}>{v}</div>
                  <div className="text-[11.5px] text-ink-3">{t(k)}</div>
                </div>
              ))}
            </div>

            <h3 className="mb-2 mt-6 text-[12px] font-semibold uppercase tracking-wide text-ink-3">{t('sys.parts')}</h3>
            <div className="grid gap-4 sm:grid-cols-2">
              {groups.map(([g, label]) => {
                const list = d.components.filter((c) => c.group === g)
                if (list.length === 0) return null
                return (
                  <div key={g}>
                    <div className="mb-1 text-[12px] font-medium text-ink-2">{t(label)}</div>
                    <ul className="divide-y divide-line rounded-xl border border-line">
                      {list.map((c) => (
                        <li key={c.id} className="flex items-center gap-2.5 px-3 py-2 text-[13px]">
                          <span className={cn('size-2 shrink-0 rounded-full', dot[c.state])} aria-hidden />
                          <span className="flex-1">{c.name}</span>
                          <span className={cn('max-w-[55%] truncate text-[11.5px]', c.state === 'error' ? 'text-danger' : 'text-ink-3')} title={c.detail}>{c.detail || t(`sys.state.${c.state}` as TKey)}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )
              })}
            </div>
            <p className="mt-5 text-right text-[11.5px] text-ink-3">{t('sys.uptime', { time: uptime(d.process.uptime_s), version: d.version || 'dev' })}</p>
          </>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
