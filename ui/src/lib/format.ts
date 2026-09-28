import { currentLocale, localeTag, tr, type Locale } from './i18n'

const rtfs: Partial<Record<Locale, Intl.RelativeTimeFormat>> = {}
const rtf = (l: Locale) => (rtfs[l] ??= new Intl.RelativeTimeFormat(localeTag(l), { numeric: 'auto' }))

export function relative(iso?: string) {
  if (!iso) return ''
  const diff = (new Date(iso).getTime() - Date.now()) / 1000
  const abs = Math.abs(diff)
  const f = rtf(currentLocale())
  if (abs < 45) return diff > 0 ? tr('time.soon') : tr('time.justNow')
  if (abs < 3600) return f.format(Math.round(diff / 60), 'minute')
  if (abs < 86400) return f.format(Math.round(diff / 3600), 'hour')
  return f.format(Math.round(diff / 86400), 'day')
}

export function time(d: Date | string) {
  return new Date(d).toLocaleTimeString(localeTag(), { hour: '2-digit', minute: '2-digit' })
}

export function date(d: Date | string, o: Intl.DateTimeFormatOptions) {
  return new Date(d).toLocaleDateString(localeTag(), o)
}

export function when(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  const today = new Date()
  const tomorrow = new Date(Date.now() + 86400000)
  const at = time(d)
  if (d.toDateString() === today.toDateString()) return tr('time.todayAt', { time: at })
  if (d.toDateString() === tomorrow.toDateString()) return tr('time.tomorrowAt', { time: at })
  return `${date(d, { weekday: 'short', day: '2-digit', month: '2-digit' })} ${at}`
}

export function usd(v: number) {
  return `$${v.toFixed(v > 0 && v < 0.1 ? 3 : 2)}`
}

export function money(v: number, currency = 'BRL') {
  return new Intl.NumberFormat(localeTag(), { style: 'currency', currency }).format(v)
}

// cronText describes the common cron shapes in words; anything else shows as is.
export function cronText(expr: string) {
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return expr
  const [min, hour, dom, mon, dow] = f
  const at = /^\d+$/.test(min) && /^\d+$/.test(hour) ? `${hour.padStart(2, '0')}:${min.padStart(2, '0')}` : ''
  if (min.startsWith('*/') && hour === '*' && dom === '*' && dow === '*') return tr('cron.everyMinutes', { n: min.slice(2) })
  if (min === '0' && hour.startsWith('*/')) return tr('cron.everyHours', { n: hour.slice(2) })
  if (dom === '*' && mon === '*' && dow === '*' && at) return tr('cron.daily', { at })
  if (dom === '*' && mon === '*' && dow === '1-5' && at) return tr('cron.weekdays', { at })
  if (dom === '*' && mon === '*' && /^[0-6]$/.test(dow) && at) return tr('cron.weekly', { day: tr(`cron.day${dow}` as 'cron.day0'), at })
  if (/^\d+$/.test(dom) && mon === '*' && dow === '*' && at) return tr('cron.monthly', { dom, at })
  return expr
}

// size writes a byte count the way people read it.
export function size(n: number) {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(1).replace('.0', '')} GB`
  if (n >= 1024 ** 2) return `${Math.round(n / 1024 ** 2)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}
