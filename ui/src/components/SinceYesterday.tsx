import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import { usd } from '../lib/format'
import { useT } from '../lib/i18n'
import { Card } from './ui'

// SinceYesterday sums up the last 24 hours in one line of numbers.
export function SinceYesterday() {
  const t = useT()
  const receipts = useQuery({ queryKey: ['receipts', ''], queryFn: () => api.receipts() })
  const cost = useQuery({ queryKey: ['cost'], queryFn: api.cost })
  const since = Date.now() - 24 * 3600 * 1000
  const recent = (receipts.data ?? []).filter((r) => new Date(r.ts).getTime() > since && !r.action.dry_run)
  if (recent.length === 0) return null
  const changes = recent.filter((r) => r.action.risk !== 'read' && r.action.risk !== 'notify' && !r.action.error).length
  const blocked = recent.filter((r) => r.action.verdict === 'block' || r.action.approved === 'no').length
  const messages = recent.filter((r) => r.action.capability === 'telegram.send').length
  const stats = [
    [String(recent.length), t('since.actions')],
    [String(messages), t('since.messages', { count: messages })],
    [String(changes), t('since.changes', { count: changes })],
    [String(blocked), t('since.blocked', { count: blocked })],
    [usd(cost.data?.today ?? 0), t('since.spent')],
  ]
  return (
    <Card className="mb-6 grid grid-cols-2 gap-px overflow-hidden bg-line p-0 sm:grid-cols-5">
      {stats.map(([n, label]) => (
        <Link key={label} to="/receipts" className="bg-surface px-4 py-3 hover:bg-sunken">
          <div className="text-[18px] font-semibold tabular-nums">{n}</div>
          <div className="text-[12px] text-ink-3">{label}</div>
        </Link>
      ))}
    </Card>
  )
}
