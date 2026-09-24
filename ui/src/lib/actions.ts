import type { ActionRecord } from './api'

const short = (s: string, n = 80) => (s.length > n ? s.slice(0, n - 1) + '…' : s)
const count = (v: unknown) => (Array.isArray(v) ? v.length : undefined)

// describe turns a capability call into the sentence a person would say.
export function describe(a: ActionRecord & { done?: string }): string {
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
    case 'gmail.trash':
      return `${would ? 'Mandaria' : 'Mandou'} um e-mail para a lixeira`
    case 'gmail.delete':
      return a.done === 'gmail.trash' ? 'Apagou um e-mail (foi para a lixeira)' : `${would ? 'Apagaria' : 'Apagou'} um e-mail`
    case 'gmail.draft':
      return `${would ? 'Escreveria' : 'Escreveu'} um rascunho para ${String(args.to ?? '')}`
    case 'gmail.send':
      return a.done === 'outbox.send_later' ? `Vai enviar um e-mail para ${String(args.to ?? '')}` : `${would ? 'Enviaria' : 'Enviou'} um e-mail para ${String(args.to ?? '')}`
    case 'gmail.unsubscribe':
      return `${would ? 'Cancelaria' : 'Cancelou'} a inscrição de uma lista`
    case 'http.getJSON':
      return `Consultou ${a.scope ?? 'um serviço'}`
    case 'telegram.send':
      return `Te mandou: “${short(String(args.text ?? ''), 90)}”`
  }
  return a.capability
}
