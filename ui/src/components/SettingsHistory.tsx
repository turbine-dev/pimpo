import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History, KeyRound, Loader2, Undo2 } from 'lucide-react'
import { useState } from 'react'
import { ApiError, api, type HistoryArea, type HistoryChange, type HistoryField } from '../lib/api'
import { cn } from '../lib/cn'
import { when } from '../lib/format'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card } from './ui'

const areas: HistoryArea[] = ['settings', 'models', 'rules', 'budget', 'connections', 'people']

// fieldName is how a field reads: without where it is kept.
function fieldName(f: HistoryField) {
  return f.field.replace(/^vault:/, '').replace(/^person\.[^.]+\./, '').replace(/^conn\./, '').replace(/^rule:.*/, 'rule').replace(/^host:/, '')
}

function shown(v: unknown, none: string): string {
  if (v === undefined || v === null || v === '') return none
  if (typeof v === 'string') return v
  if (typeof v === 'object' && v && 'text' in v && typeof (v as { text: unknown }).text === 'string') return (v as { text: string }).text
  const s = JSON.stringify(v)
  return s.length > 120 ? s.slice(0, 117) + '…' : s
}

// SettingsHistory lists changes to settings, newest first, each with who
// made it and an undo. mine is a member's own accounts; the house's
// settings are the owner's alone.
export function SettingsHistory({ mine = false }: { mine?: boolean }) {
  const t = useT()
  const [area, setArea] = useState<HistoryArea | ''>('')
  const list = useQuery({ queryKey: ['history', area], queryFn: () => api.history(area) })
  const shownAreas = mine ? [] : areas
  return (
    <Card className="space-y-3 p-5">
      <div>
        <h2 className="flex items-center gap-2 text-[15px] font-medium"><History size={16} /> {t('hist.title')}</h2>
        <p className="text-[13px] text-ink-3">{t(mine ? 'hist.mineText' : 'hist.text')}</p>
      </div>
      {shownAreas.length > 0 && (
        <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('hist.filter')}>
          {(['', ...shownAreas] as const).map((a) => (
            <button key={a} type="button" aria-pressed={area === a} onClick={() => setArea(a)}
              className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', area === a ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
              {a ? t(`hist.area.${a}` as TKey) : t('hist.all')}
            </button>
          ))}
        </div>
      )}
      {list.isPending ? <Loader2 size={16} className="animate-spin text-ink-3" /> : (list.data ?? []).length === 0 ? (
        <p className="text-[13px] text-ink-3">{t('hist.empty')}</p>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line" aria-label={t('hist.title')}>
          {(list.data ?? []).map((c) => <Change key={c.id} c={c} />)}
        </ul>
      )}
    </Card>
  )
}

function Change({ c }: { c: HistoryChange }) {
  const t = useT()
  const qc = useQueryClient()
  const [asking, setAsking] = useState(false)
  const undo = useMutation({
    mutationFn: () => api.undoChange(c.id),
    onSuccess: () => { setAsking(false); qc.invalidateQueries() },
  })
  const problem = undo.error instanceof ApiError && undo.error.problem ? t(`hist.problem.${undo.error.problem}` as TKey) : undo.error?.message
  const none = t('hist.none')
  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <span className={cn('text-[13.5px] font-medium', c.undone && 'text-ink-3 line-through')}>
          {t(`hist.area.${c.area}` as TKey)}{c.target && <span className="font-normal text-ink-2"> · {c.target}</span>}
        </span>
        <span className="text-[12px] text-ink-3">{when(c.ts)} · {t('hist.by', { who: c.who || t('hist.someone') })}</span>
        {c.undo_of ? <span className="text-[12px] text-ink-3">· {t('hist.undoOf')}</span> : null}
      </div>
      <ul className="mt-1 space-y-0.5 text-[12.5px]">
        {c.fields.map((f) => (
          <li key={f.field} className="break-words text-ink-2">
            <span className="font-mono text-[12px] text-ink-3">{fieldName(f)}</span>{' '}
            {f.secret ? (
              <span className="inline-flex items-center gap-1"><KeyRound size={12} aria-hidden /> {t(`hist.secret.${f.secret}` as TKey)}</span>
            ) : (
              <><span className="line-through decoration-ink-3/50">{shown(f.before, none)}</span> <span aria-hidden>→</span><span className="sr-only">{t('hist.to')}</span> <span className="text-ink">{shown(f.after, none)}</span></>
            )}
          </li>
        ))}
      </ul>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {c.undone && <span className="text-[12px] text-ink-3">{t('hist.undone')}</span>}
        {c.undoable && !asking && (
          <Button size="sm" variant="ghost" onClick={() => { undo.reset(); setAsking(true) }}><Undo2 size={14} /> {t('hist.undo')}</Button>
        )}
        {!c.undoable && !c.undone && <span className="text-[12px] text-ink-3">{t(c.reenter.length ? 'hist.secretOnly' : 'hist.noUndo')}</span>}
        {asking && (
          <div role="group" aria-label={t('hist.confirm')} className="flex flex-wrap items-center gap-2 rounded-lg bg-sunken px-3 py-2">
            <span className="text-[12.5px]">{t('hist.confirm')}</span>
            {c.reenter.length > 0 && <span className="text-[12.5px] text-ink-3">{t('hist.reenter', { names: c.reenter.join(', ') })}</span>}
            <Button size="sm" variant="primary" onClick={() => undo.mutate()} disabled={undo.isPending}>{t('hist.confirmYes')}</Button>
            <Button size="sm" variant="ghost" onClick={() => setAsking(false)}>{t('common.cancel')}</Button>
          </div>
        )}
      </div>
      {undo.data && undo.data.reenter.length > 0 && <p className="mt-1 text-[12.5px] text-change">{t('hist.reenter', { names: undo.data.reenter.join(', ') })}</p>}
      {undo.error && <p role="alert" className="mt-1 text-[12.5px] text-danger">{problem}</p>}
    </li>
  )
}
