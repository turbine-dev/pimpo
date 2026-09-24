const rtf = new Intl.RelativeTimeFormat('pt-BR', { numeric: 'auto' })

export function relative(iso?: string) {
  if (!iso) return ''
  const diff = (new Date(iso).getTime() - Date.now()) / 1000
  const abs = Math.abs(diff)
  if (abs < 45) return diff > 0 ? 'em instantes' : 'agora há pouco'
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour')
  return rtf.format(Math.round(diff / 86400), 'day')
}

export function when(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  const today = new Date()
  const tomorrow = new Date(Date.now() + 86400000)
  const time = d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  if (d.toDateString() === today.toDateString()) return `hoje ${time}`
  if (d.toDateString() === tomorrow.toDateString()) return `amanhã ${time}`
  return `${d.toLocaleDateString('pt-BR', { weekday: 'short', day: '2-digit', month: '2-digit' })} ${time}`
}

export function usd(v: number) {
  return `$${v.toFixed(v > 0 && v < 0.1 ? 3 : 2)}`
}

const days = ['domingo', 'segunda', 'terça', 'quarta', 'quinta', 'sexta', 'sábado']

// cronText describes the common cron shapes in words; anything else shows as is.
export function cronText(expr: string) {
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return expr
  const [min, hour, dom, mon, dow] = f
  const at = /^\d+$/.test(min) && /^\d+$/.test(hour) ? `às ${hour.padStart(2, '0')}:${min.padStart(2, '0')}` : ''
  if (min.startsWith('*/') && hour === '*' && dom === '*' && dow === '*') return `a cada ${min.slice(2)} min`
  if (min === '0' && hour.startsWith('*/')) return `a cada ${hour.slice(2)} h`
  if (dom === '*' && mon === '*' && dow === '*' && at) return `todo dia ${at}`
  if (dom === '*' && mon === '*' && dow === '1-5' && at) return `dias úteis ${at}`
  if (dom === '*' && mon === '*' && /^\d$/.test(dow) && at) return `toda ${days[Number(dow)]} ${at}`
  if (/^\d+$/.test(dom) && mon === '*' && dow === '*' && at) return `todo dia ${dom} ${at}`
  return expr
}
