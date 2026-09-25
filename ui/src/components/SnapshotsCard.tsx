import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History, RotateCcw, Save } from 'lucide-react'
import { api, type Snapshot } from '../lib/api'
import { date } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card } from './ui'

const when = (s: Snapshot) => date(s.when, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

// SnapshotsCard lists the copies Zodim keeps on this computer and puts one
// back on the next start.
export function SnapshotsCard() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['snapshots'], queryFn: api.snapshots })
  const done = () => qc.invalidateQueries({ queryKey: ['snapshots'] })
  const create = useMutation({ mutationFn: api.createSnapshot, onSuccess: done })
  const stage = useMutation({ mutationFn: api.stageRestore, onSuccess: done })
  const cancel = useMutation({ mutationFn: api.cancelRestore, onSuccess: done })
  const label = (s: Snapshot) => {
    if (s.label.startsWith('before-') && !(s.label in labels)) return t('snap.label.before', { version: s.label.slice(7) })
    return labels[s.label] ? t(labels[s.label]) : s.label
  }
  const list = q.data?.snapshots ?? []
  const staged = list.find((s) => s.name === q.data?.staged)
  const error = create.error ?? stage.error ?? cancel.error

  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-[15px] font-medium"><History size={17} /> {t('snap.title')}</div>
        {q.data?.available && <Button size="sm" onClick={() => create.mutate()} disabled={create.isPending}><Save size={14} /> {t('snap.now')}</Button>}
      </div>
      <p className="mb-4 text-[13px] text-ink-3">{t('snap.text')}</p>
      {q.data && !q.data.available && <p className="text-[13px] text-ink-3">{t('snap.unavailable')}</p>}
      {staged && (
        <div className="mb-3 flex flex-wrap items-center gap-3 rounded-lg bg-change-soft px-3 py-2 text-[13px] text-change">
          <span className="flex-1">{t('snap.staged', { when: when(staged) })}</span>
          <Button size="sm" variant="ghost" onClick={() => cancel.mutate()}>{t('snap.cancel')}</Button>
        </div>
      )}
      {q.data?.available && list.length === 0 && <p className="text-[13px] text-ink-3">{t('snap.empty')}</p>}
      {list.length > 0 && (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {list.map((s) => (
            <li key={s.name} className="flex items-center gap-3 px-3 py-2 text-[13px]">
              <span className="w-28 shrink-0 tabular-nums text-ink-2">{when(s)}</span>
              <span className="flex-1 truncate">{label(s)}</span>
              <span className="hidden text-[11.5px] tabular-nums text-ink-3 sm:inline">{(s.bytes / 1e6).toFixed(1)} MB</span>
              <Button size="sm" variant="ghost" aria-label={`${t('snap.restore')} — ${when(s)}`} disabled={s.name === staged?.name || stage.isPending}
                onClick={() => window.confirm(t('snap.confirm', { when: when(s) })) && stage.mutate(s.name)}>
                <RotateCcw size={14} /> <span className="hidden sm:inline">{t('snap.restore')}</span>
              </Button>
            </li>
          ))}
        </ul>
      )}
      {list.length > 0 && <p className="mt-3 text-[12px] text-ink-3">{t('snap.oldVersion')}</p>}
      {error && <p className="mt-2 text-[13px] text-danger">{error.message}</p>}
    </Card>
  )
}

const labels: Record<string, TKey> = {
  daily: 'snap.label.daily',
  manual: 'snap.label.manual',
  'before-import': 'snap.label.before-import',
  'before-restore': 'snap.label.before-restore',
}
