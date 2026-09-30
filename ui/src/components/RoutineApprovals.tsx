import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ShieldCheck, X } from 'lucide-react'
import { api, type Grant } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { capabilityLabel } from './RoutineCard'
import { Card } from './ui'

// describeGrant says what a grant lets through: the identifying arguments
// that must stay the same, and the most an amount may be.
export function describeGrant(g: Grant) {
  const parts = Object.entries(g.match).filter(([k]) => k !== '').map(([k, v]) => `${k}: ${v.replace(/,/g, ', ')}`)
  if (g.match['']) parts.push(g.match[''])
  if (g.scope) parts.push(g.scope)
  for (const [k, n] of Object.entries(g.limits ?? {})) parts.push(`${k} ≤ ${n}`)
  return parts.join(' · ')
}

// RoutineApprovals lists the person's "approve for this routine" answers,
// each revocable; a routine's new version or anything different asks again.
export function RoutineApprovals() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['grants'], queryFn: api.grants })
  const revoke = useMutation({ mutationFn: api.revokeGrant, onSuccess: () => qc.invalidateQueries({ queryKey: ['grants'] }) })
  const list = q.data ?? []
  if (list.length === 0) return null
  return (
    <section className="mb-6" aria-label={t('grants.title')}>
      <Card className="p-4">
        <div className="mb-1 flex items-center gap-2 text-[14px] font-medium"><ShieldCheck size={15} /> {t('grants.title')}</div>
        <p className="mb-2 text-[12.5px] text-ink-3">{t('grants.subtitle')}</p>
        <ul className="divide-y divide-line">
          {list.map((g) => (
            <li key={g.id} className="flex items-center gap-3 py-2 text-[13.5px]">
              <div className="min-w-0 flex-1">
                <div>{t('grants.item', { routine: g.routine_name || g.routine, action: capabilityLabel(g.capability) })}</div>
                <div className="truncate text-[12.5px] text-ink-3">{describeGrant(g)} · {relative(g.created)}</div>
              </div>
              <button type="button" aria-label={t('grants.revoke', { routine: g.routine_name || g.routine, action: capabilityLabel(g.capability) })}
                onClick={() => revoke.mutate(g.id)} className="grid size-7 place-items-center rounded-lg text-ink-3 hover:bg-sunken hover:text-ink"><X size={14} /></button>
            </li>
          ))}
        </ul>
      </Card>
    </section>
  )
}
