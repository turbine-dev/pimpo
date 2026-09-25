import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Clock, MapPin, Search, SlidersHorizontal, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type Place, type RoutineParam, type RoutineSummary } from '../lib/api'
import { cn } from '../lib/cn'
import { cronText, when } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card } from './ui'

const field = 'h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
const input = field + ' w-full'

// The schedule shapes people use, as a form; anything else stays cron.
type Freq = 'daily' | 'weekdays' | 'weekly' | 'monthly' | 'hourly' | 'minutes' | 'custom'
type Sched = { freq: Freq; time: string; days: number[]; dom: number; every: number; minutes: number; cron: string }

const minuteSteps = [5, 10, 15, 30]

const pad = (n: number | string) => String(n).padStart(2, '0')

export function parseCron(expr: string): Sched {
  const base: Sched = { freq: 'custom', time: '07:00', days: [1], dom: 1, every: 2, minutes: 15, cron: expr }
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return base
  const [m, h, dom, mon, dow] = f
  const num = (s: string) => /^\d+$/.test(s)
  if (mon !== '*') return base
  if (m.startsWith('*/') && minuteSteps.includes(+m.slice(2)) && h === '*' && dom === '*' && dow === '*') return { ...base, freq: 'minutes', minutes: +m.slice(2) }
  if (num(m) && h.startsWith('*/') && num(h.slice(2)) && dom === '*' && dow === '*') return { ...base, freq: 'hourly', every: +h.slice(2), time: `00:${pad(m)}` }
  if (!num(m) || !num(h)) return base
  const time = `${pad(h)}:${pad(m)}`
  if (dom === '*' && dow === '*') return { ...base, freq: 'daily', time }
  if (dom === '*' && dow === '1-5') return { ...base, freq: 'weekdays', time }
  if (dom === '*' && /^[0-6](,[0-6])*$/.test(dow)) return { ...base, freq: 'weekly', time, days: dow.split(',').map(Number) }
  if (num(dom) && dow === '*') return { ...base, freq: 'monthly', time, dom: +dom }
  return base
}

export function toCron(s: Sched): string {
  const [h, m] = s.time.split(':').map(Number)
  switch (s.freq) {
    case 'daily': return `${m} ${h} * * *`
    case 'weekdays': return `${m} ${h} * * 1-5`
    case 'weekly': return `${m} ${h} * * ${[...s.days].sort().join(',') || '1'}`
    case 'monthly': return `${m} ${h} ${s.dom} * *`
    case 'hourly': return `${m} */${s.every} * * *`
    case 'minutes': return `*/${s.minutes} * * * *`
    default: return s.cron.trim()
  }
}

export function RoutineSettings({ s, onRedo }: { s: RoutineSummary; onRedo?: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const params = s.params ?? []
  const [sched, setSched] = useState(() => parseCron(s.schedule))
  const [values, setValues] = useState<Record<string, unknown>>(s.values ?? {})
  useEffect(() => { setSched(parseCron(s.schedule)); setValues(s.values ?? {}) }, [s.schedule, s.values])
  const cron = toCron(sched)
  const dirty = cron !== s.schedule || JSON.stringify(values) !== JSON.stringify(s.values ?? {})
  const save = useMutation({
    mutationFn: () => api.saveRoutineSettings(s.id, cron, values),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['routine', s.id] }); qc.invalidateQueries({ queryKey: ['routines'] }) },
  })

  return (
    <>
    {s.gallery_update && <UpdateBanner s={s} />}
    <Card className="mb-6 p-5">
      <div className="mb-4 flex items-start gap-3">
        <div className="grid size-9 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore"><SlidersHorizontal size={17} /></div>
        <div>
          <h2 className="text-[15px] font-medium">{t('rs.title')}</h2>
          <p className="text-[13px] text-ink-3">{t('rs.subtitle')}</p>
        </div>
      </div>
      <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <ScheduleEditor value={sched} onChange={setSched} />
        {s.default_schedule && cron !== s.default_schedule && (
          <button type="button" className="-mt-3 text-[12.5px] text-ink-3 underline" onClick={() => setSched(parseCron(s.default_schedule!))}>{t('rs.reset')} ({cronText(s.default_schedule)})</button>
        )}
        {params.map((p) => (
          <Field key={p.name} p={p} value={values[p.name]} onChange={(v) => setValues({ ...values, [p.name]: v })} />
        ))}
        {params.length === 0 && (
          <p className="text-[13px] text-ink-3">
            {t('rs.onlySchedule')}{!s.gallery_update && onRedo && <> {t('rs.makeAdjustable')} <button type="button" className="underline" onClick={onRedo}>{t('rs.redo')}</button></>}
          </p>
        )}
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <div className="flex flex-wrap items-center justify-end gap-3">
          {save.isSuccess && !dirty && <span className="flex items-center gap-1 text-[13px] text-read"><Check size={14} /> {t('rs.saved')} {s.next_run && when(s.next_run)}</span>}
          {dirty && <span className="text-[12.5px] text-ink-3">{t('rs.unsaved')}</span>}
          <Button variant="primary" type="submit" disabled={!dirty || save.isPending}>{t('rs.save')}</Button>
        </div>
      </form>
    </Card>
    </>
  )
}

function UpdateBanner({ s }: { s: RoutineSummary }) {
  const t = useT()
  const qc = useQueryClient()
  const up = s.gallery_update!
  const apply = useMutation({
    mutationFn: () => api.updateFromGallery(s.id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['routine', s.id] }); qc.invalidateQueries({ queryKey: ['routines'] }) },
  })
  return (
    <Card className="mb-4 flex flex-wrap items-center gap-4 border-explore/40 bg-explore-soft/40 p-4">
      <Sparkles size={18} className="shrink-0 text-explore" />
      <div className="min-w-0 flex-1 text-[13.5px]">
        <div className="font-medium">{t('up.title')}: {up.description}</div>
        {up.settings.length > 0 && <div className="text-ink-2">{t('up.adds', { list: up.settings.join(', ') })}</div>}
        <div className="text-[12.5px] text-ink-3">{t('up.keeps')}</div>
        {apply.error && <div className="text-danger">{apply.error.message}</div>}
      </div>
      <Button variant="primary" onClick={() => apply.mutate()} disabled={apply.isPending}>{t('up.update')}</Button>
    </Card>
  )
}

function ScheduleEditor({ value, onChange }: { value: Sched; onChange: (s: Sched) => void }) {
  const t = useT()
  const freqs: Freq[] = ['daily', 'weekdays', 'weekly', 'monthly', 'hourly', 'minutes', 'custom']
  const set = (p: Partial<Sched>) => onChange({ ...value, ...p })
  return (
    <fieldset>
      <legend className="mb-1.5 flex items-center gap-1.5 text-[13px] font-medium text-ink-2"><Clock size={14} /> {t('rs.when')}</legend>
      <div className="flex flex-wrap items-center gap-2">
        <select aria-label={t('rs.when')} value={value.freq} onChange={(e) => set({ freq: e.target.value as Freq, cron: toCron(value) })} className={cn(field, 'w-auto')}>
          {freqs.map((f) => <option key={f} value={f}>{t(`rs.freq.${f}`)}</option>)}
        </select>
        {value.freq === 'monthly' && (
          <label className="flex items-center gap-2 text-[13px] text-ink-2">{t('rs.onDay')}
            <input type="number" min={1} max={28} value={value.dom} onChange={(e) => set({ dom: Math.min(28, Math.max(1, +e.target.value || 1)) })} className={cn(field, 'w-20')} aria-label={t('rs.onDay')} />
          </label>
        )}
        {value.freq === 'hourly' && (
          <label className="flex items-center gap-2 text-[13px] text-ink-2">{t('rs.every')}
            <input type="number" min={1} max={12} value={value.every} onChange={(e) => set({ every: Math.min(12, Math.max(1, +e.target.value || 1)) })} className={cn(field, 'w-20')} aria-label={t('rs.every')} />
            {t('rs.hours')}
          </label>
        )}
        {value.freq === 'minutes' && (
          <label className="flex items-center gap-2 text-[13px] text-ink-2">{t('rs.every')}
            <select value={value.minutes} onChange={(e) => set({ minutes: +e.target.value })} className={cn(field, 'w-auto')} aria-label={t('rs.minutesLabel')}>
              {minuteSteps.map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
            {t('rs.minutes')}
          </label>
        )}
        {value.freq !== 'hourly' && value.freq !== 'minutes' && value.freq !== 'custom' && (
          <label className="flex items-center gap-2 text-[13px] text-ink-2">{t('rs.at')}
            <input type="time" value={value.time} onChange={(e) => e.target.value && set({ time: e.target.value })} className={cn(field, 'w-32')} aria-label={t('rs.at')} />
          </label>
        )}
        {value.freq === 'custom' && (
          <input value={value.cron} onChange={(e) => set({ cron: e.target.value })} className={cn(field, 'w-48 font-mono')} aria-label={t('rs.cron')} placeholder="0 7 * * *" />
        )}
      </div>
      {value.freq === 'weekly' && (
        <div className="mt-2 flex flex-wrap gap-1.5" role="group" aria-label={t('rs.freq.weekly')}>
          {[1, 2, 3, 4, 5, 6, 0].map((d) => (
            <Chip key={d} on={value.days.includes(d)} onClick={() => set({ days: value.days.includes(d) ? value.days.filter((x) => x !== d) : [...value.days, d] })}>{t(`rs.day.${d}` as TKey)}</Chip>
          ))}
        </div>
      )}
      <p className="mt-1.5 text-[12.5px] text-ink-3">{cronText(toCron(value))}</p>
    </fieldset>
  )
}

function Chip({ on, onClick, children, muted }: { on: boolean; onClick: () => void; children: React.ReactNode; muted?: boolean }) {
  return (
    <button type="button" aria-pressed={on} onClick={onClick}
      className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', on ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong', muted && !on && 'border-dashed text-ink-3')}>
      {children}
    </button>
  )
}

function Field({ p, value, onChange }: { p: RoutineParam; value: unknown; onChange: (v: unknown) => void }) {
  const t = useT()
  const label = (
    <span className="mb-1.5 block text-[13px] font-medium text-ink-2">{p.label || p.name}{p.help && <span className="ml-1.5 font-normal text-ink-3">{p.help}</span>}</span>
  )
  switch (p.type) {
    case 'boolean':
      return (
        <div>
          {label}
          <div className="flex gap-1.5">
            <Chip on={value === true} onClick={() => onChange(true)}>{t('rs.yes')}</Chip>
            <Chip on={value === false} onClick={() => onChange(false)}>{t('rs.no')}</Chip>
          </div>
        </div>
      )
    case 'select':
      return <label className="block">{label}<select value={String(value ?? '')} onChange={(e) => onChange(e.target.value)} className={input}>{p.options?.map((o) => <option key={o}>{o}</option>)}</select></label>
    case 'multiselect': {
      const list = (value as string[]) ?? []
      return (
        <div>{label}
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={p.label}>
            {p.options?.map((o) => <Chip key={o} on={list.includes(o)} onClick={() => onChange(list.includes(o) ? list.filter((x) => x !== o) : [...list, o])}>{o}</Chip>)}
          </div>
        </div>
      )
    }
    case 'location':
      return <div>{label}<PlacePicker value={value as Place | undefined} onChange={onChange} label={p.label} /></div>
    case 'destinations':
      return <div>{label}<Destinations value={(value as string[]) ?? []} onChange={onChange} label={p.label} /></div>
    case 'number':
      return <label className="block">{label}<input type="number" step="any" value={value === undefined ? '' : String(value)} onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))} className={cn(field, 'w-40')} /></label>
    default: {
      const type = { date: 'date', time: 'time', email: 'email' }[p.type as string] ?? 'text'
      return <label className="block">{label}<input type={type} value={String(value ?? '')} onChange={(e) => onChange(e.target.value)} className={type === 'text' || type === 'email' ? input : cn(field, 'w-44')} /></label>
    }
  }
}

function PlacePicker({ value, onChange, label }: { value?: Place; onChange: (p: Place) => void; label: string }) {
  const t = useT()
  const [q, setQ] = useState('')
  const [debounced, setDebounced] = useState('')
  useEffect(() => { const id = setTimeout(() => setDebounced(q.trim()), 300); return () => clearTimeout(id) }, [q])
  const found = useQuery({ queryKey: ['geocode', debounced], queryFn: () => api.geocode(debounced), enabled: debounced.length >= 2 })
  return (
    <div>
      {value && (
        <div className="mb-2 flex items-center gap-2 text-[14px]"><MapPin size={15} className="text-explore" /> {value.name}{value.country && <span className="text-ink-3">· {value.country}</span>}</div>
      )}
      <div className="flex h-10 items-center gap-2 rounded-[10px] border border-line bg-bg px-3 focus-within:border-accent">
        <Search size={15} className="text-ink-3" />
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('rs.searchPlace')} aria-label={`${label}: ${t('rs.searchPlace')}`} className="flex-1 bg-transparent text-sm outline-none" />
      </div>
      {debounced.length >= 2 && found.data && (
        <ul className="mt-1 overflow-hidden rounded-[10px] border border-line bg-surface" role="listbox" aria-label={label}>
          {found.data.length === 0 && <li className="px-3 py-2 text-[13px] text-ink-3">{t('rs.noPlace')}</li>}
          {found.data.map((p) => (
            <li key={`${p.latitude},${p.longitude}`}>
              <button type="button" role="option" aria-selected={false} className="flex w-full items-center gap-2 px-3 py-2 text-left text-[13.5px] hover:bg-sunken" onClick={() => { onChange(p); setQ(''); setDebounced('') }}>
                <MapPin size={13} className="text-ink-3" /> {p.name} <span className="text-ink-3">· {p.country}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function Destinations({ value, onChange, label }: { value: string[]; onChange: (v: string[]) => void; label: string }) {
  const t = useT()
  const q = useQuery({ queryKey: ['destinations'], queryFn: api.destinations })
  const list = useMemo(() => q.data ?? [], [q.data])
  return (
    <div>
      <div className="flex flex-wrap gap-1.5" role="group" aria-label={label}>
        {list.map((d) => (
          <Chip key={d.id} on={value.includes(d.id)} muted={!d.ready} onClick={() => onChange(value.includes(d.id) ? value.filter((x) => x !== d.id) : [...value, d.id])}>
            {d.label}{!d.ready && ` · ${t('rs.notReady')}`}
          </Chip>
        ))}
      </div>
      <p className="mt-1.5 text-[12.5px] text-ink-3">
        {value.length === 0 && `${t('rs.destinationsNone')} `}
        <Link to="/connections" className="underline">{t('rs.moreChannels')}</Link>
      </p>
    </div>
  )
}
