import { Inbox } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useT } from '../lib/i18n'
import { KindFilter, NeedRow, useNeedAction, useNeeds } from './Needs'

// InboxPanel opens from the bell: the same list as the Needs you page,
// answered in place, with the full page one click away.
export function InboxPanel({ onClose }: { onClose: () => void }) {
  const t = useT()
  const nav = useNavigate()
  const [kind, setKind] = useState('all')
  const needs = useNeeds()
  const act = useNeedAction((to) => { onClose(); nav(to) })
  const items = needs.data?.items ?? []
  const shown = items.filter((n) => kind === 'all' || n.kind === kind)

  return (
    <div role="dialog" aria-label={t('inbox.title')}
      className="absolute right-0 top-full z-40 mt-2 w-[min(400px,calc(100vw-24px))] rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] md:bottom-full md:left-0 md:right-auto md:top-auto md:mb-2 md:mt-0">
      <div className="flex items-center gap-2 px-4 pt-3.5">
        <Inbox size={16} className="text-ink-3" />
        <span className="flex-1 text-[14px] font-semibold">{t('inbox.title')}</span>
        <Link to="/inbox" onClick={onClose} className="text-[12px] text-ink-3 hover:text-ink">{t('inbox.openAll')}</Link>
      </div>
      <KindFilter counts={needs.data?.counts ?? {}} total={needs.data?.total ?? 0} value={kind} onChange={setKind} className="mt-2 px-3" />
      {act.error && <p role="alert" className="px-4 pt-2 text-[12px] text-danger">{act.error.message}</p>}
      <div className="max-h-[60vh] overflow-y-auto p-2">
        {shown.length === 0 ? (
          <div className="grid place-items-center px-4 py-10 text-center">
            <Inbox size={22} className="mb-2 text-ink-3" />
            <div className="text-[13.5px] font-medium">{t('inbox.nothing')}</div>
            <div className="text-[12.5px] text-ink-3">{t('inbox.nothingText')}</div>
          </div>
        ) : shown.map((n) => <NeedRow key={`${n.kind}:${n.id}`} need={n} compact busy={act.isPending} onAct={(a) => act.mutate(a)} />)}
      </div>
    </div>
  )
}
