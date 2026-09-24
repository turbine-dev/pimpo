import type { ActionRecord } from './api'
import { tr } from './i18n'

const short = (s: string, n = 80) => (s.length > n ? s.slice(0, n - 1) + '…' : s)
const count = (v: unknown) => (Array.isArray(v) ? v.length : undefined)

// describe turns a capability call into the sentence a person would say.
export function describe(a: ActionRecord & { done?: string }): string {
  const args = (a.args ?? {}) as Record<string, unknown>
  const n = count(a.result)
  const would = a.dry_run
  const to = String(args.to ?? '')
  switch (a.capability) {
    case 'gmail.search': {
      const q = String(args.query ?? '').trim()
      return `${tr('act.search')}${q ? ` “${short(q, 50)}”` : ''}${n !== undefined ? ` · ${tr('act.found', { count: n })}` : ''}`
    }
    case 'calendar.events':
      return `${tr('act.calendar')}${n !== undefined ? ` · ${tr('act.events', { count: n })}` : ''}`
    case 'gmail.archive':
      return tr(would ? 'act.archiveWould' : 'act.archive')
    case 'gmail.label':
      return tr(would ? 'act.labelWould' : 'act.label', { label: String(args.label ?? '') })
    case 'gmail.trash':
      return tr(would ? 'act.trashWould' : 'act.trash')
    case 'gmail.delete':
      return a.done === 'gmail.trash' ? tr('act.deleteTrashed') : tr(would ? 'act.deleteWould' : 'act.delete')
    case 'gmail.draft':
      return tr(would ? 'act.draftWould' : 'act.draft', { to })
    case 'gmail.send':
      return a.done === 'outbox.send_later' ? tr('act.sendLater', { to }) : tr(would ? 'act.sendWould' : 'act.send', { to })
    case 'gmail.unsubscribe':
      return tr(would ? 'act.unsubscribeWould' : 'act.unsubscribe')
    case 'http.getJSON':
      return tr('act.http', { what: a.scope ?? tr('act.httpService') })
    case 'telegram.send':
      return tr('act.telegram', { text: short(String(args.text ?? ''), 90) })
  }
  return a.capability
}
