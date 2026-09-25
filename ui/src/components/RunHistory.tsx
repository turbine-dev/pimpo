import { useInfiniteQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type RecentRun } from '../lib/api'
import { cn } from '../lib/cn'
import { usd, when } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card } from './ui'

const dot: Record<RecentRun['outcome'], string> = { ok: 'bg-read', failed: 'bg-danger', skipped: 'bg-ink-3', running: 'bg-accent animate-pulse-soft' }
const label: Record<RecentRun['outcome'], TKey> = { ok: 'runs.ok', failed: 'runs.failedLabel', skipped: 'runs.skippedLabel', running: 'runs.running' }

function took(r: RecentRun) {
  if (!r.ended_at) return ''
  const s = Math.max(0, (new Date(r.ended_at).getTime() - new Date(r.started_at).getTime()) / 1000)
  return s < 60 ? `${s.toFixed(s < 10 ? 1 : 0)} s` : `${Math.round(s / 60)} min`
}

// Every run of every routine, newest first.
export function RunHistory() {
  const t = useT()
  const [outcome, setOutcome] = useState<'' | 'failed'>('')
  const pages = useInfiniteQuery({
    queryKey: ['runs', outcome],
    queryFn: ({ pageParam }) => api.recentRuns(outcome, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.length === 50 ? last[last.length - 1].id : undefined),
  })
  const runs = pages.data?.pages.flat() ?? []
  return (
    <div className="space-y-3">
      <div className="flex gap-1.5" role="group">
        {(['', 'failed'] as const).map((o) => (
          <button key={o} type="button" aria-pressed={outcome === o} onClick={() => setOutcome(o)}
            className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', outcome === o ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
            {t(o ? 'runs.failed' : 'runs.all')}
          </button>
        ))}
      </div>
      {pages.isPending ? <Loader2 size={16} className="animate-spin text-ink-3" /> : runs.length === 0 ? (
        <p className="text-[13px] text-ink-3">{t(outcome ? 'runs.noneFailed' : 'runs.none')}</p>
      ) : (
        <Card className="divide-y divide-line">
          {runs.map((r) => (
            <div key={r.id} className="flex items-start gap-3 px-4 py-3">
              <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', dot[r.outcome])} aria-hidden />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <Link to={`/routines/${r.routine}`} className="text-[13.5px] font-medium hover:underline">{r.name}</Link>
                  <span className="text-[12px] text-ink-3">{t(label[r.outcome])} · {when(r.started_at)}</span>
                </div>
                {r.error && <p className="mt-0.5 line-clamp-2 text-[12.5px] text-danger">{r.error}</p>}
              </div>
              <div className="shrink-0 text-right text-[12px] tabular-nums text-ink-3">
                <div>{usd(r.cost_usd)}</div>
                <div>{[t('runs.calls', { count: r.calls }), took(r) && t('runs.took', { s: took(r) })].filter(Boolean).join(' · ')}</div>
              </div>
            </div>
          ))}
        </Card>
      )}
      {pages.hasNextPage && <Button size="sm" variant="ghost" onClick={() => pages.fetchNextPage()} disabled={pages.isFetchingNextPage}>{t('runs.more')}</Button>}
    </div>
  )
}
