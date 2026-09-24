import type { ActionRecord } from './api'

const short = (s: string, n = 80) => (s.length > n ? s.slice(0, n - 1) + '…' : s)
const count = (v: unknown) => (Array.isArray(v) ? v.length : undefined)

// describe turns a capability call into the sentence a person would say.
export function describe(a: ActionRecord): string {
  const args = (a.args ?? {}) as Record<string, unknown>
  const n = count(a.result)
  const would = a.dry_run
  switch (a.capability) {
    case 'gmail.search': {
      const q = String(args.query ?? '').trim()
      return `Buscou e-mails${q ? ` “${short(q, 50)}”` : ''}${n !== undefined ? ` · ${n} encontrado${n === 1 ? '' : 's'}` : ''}`
    }
    case 'calendar.events':
      return `Leu a agenda${n !== undefined ? ` · ${n} evento${n === 1 ? '' : 's'}` : ''}`
    case 'gmail.archive':
      return `${would ? 'Arquivaria' : 'Arquivou'} um e-mail`
    case 'gmail.label':
      return `${would ? 'Marcaria' : 'Marcou'} um e-mail como “${String(args.label ?? '')}”`
    case 'http.getJSON':
      return `Consultou ${a.scope ?? 'um serviço'}`
    case 'telegram.send':
      return `Te mandou: “${short(String(args.text ?? ''), 90)}”`
  }
  return a.capability
}
