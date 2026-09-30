import { useQuery } from '@tanstack/react-query'
import { BellOff, Headphones } from 'lucide-react'
import { useState } from 'react'
import { KindFilter, NeedRow, useNeedAction, useNeeds } from '../components/Needs'
import { Card, EmptyState } from '../components/ui'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'

// Inbox is the one list of what needs you, the most urgent first, with a
// filter by kind; the bell opens the same list.
export function Inbox() {
  const t = useT()
  const [kind, setKind] = useState('all')
  const needs = useNeeds()
  const act = useNeedAction()
  const media = useQuery({ queryKey: ['media'], queryFn: api.media, refetchInterval: 60_000 })
  const items = needs.data?.items ?? []
  const shown = items.filter((n) => kind === 'all' || n.kind === kind)
  const audio = media.data ?? []

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('inbox.title')}</h1>
      <p className="mb-5 text-sm text-ink-2">{t('inbox.subtitle')}</p>
      {needs.data && items.length > 0 && <KindFilter counts={needs.data.counts} total={needs.data.total} value={kind} onChange={setKind} className="mb-4" />}
      {act.error && <p role="alert" className="mb-3 text-[13px] text-danger">{act.error.message}</p>}
      {needs.data && items.length === 0 && audio.length === 0 && (
        <EmptyState icon={<BellOff size={22} />} title={t('inbox.emptyTitle')}>
          {t('inbox.emptyText')}
        </EmptyState>
      )}
      {shown.length > 0 && (
        <Card className="overflow-hidden">
          <ul aria-label={t('inbox.title')}>
            {shown.map((n) => (
              <li key={`${n.kind}:${n.id}`} className="border-line [&:not(:first-child)]:border-t">
                <NeedRow need={n} busy={act.isPending} onAct={(a) => act.mutate(a)} />
              </li>
            ))}
          </ul>
        </Card>
      )}
      {audio.length > 0 && kind === 'all' && (
        <Card className="mt-3 p-4">
          <div className="mb-2 flex items-center gap-2 text-[14px] font-medium"><Headphones size={15} /> {t('inbox.audio')}</div>
          <ul className="space-y-3">
            {audio.map((m) => (
              <li key={m.id}>
                <div className="mb-1 flex items-baseline justify-between gap-3 text-[13px]"><span className="min-w-0 truncate">{m.title}</span><span className="shrink-0 text-[12px] text-ink-3">{relative(m.at)}</span></div>
                <audio controls preload="none" src={`/api/media/${m.id}`} className="w-full" aria-label={m.title} />
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  )
}
