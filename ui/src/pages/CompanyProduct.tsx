import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, TriangleAlert, X } from 'lucide-react'
import { useState } from 'react'
import { Button, Card } from '../components/ui'
import { api, type CompanyBrief, type Org } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { field } from './Companies'

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id
const link = (s: string) => /^https?:\/\//.test(s)

// hasProduct says whether a company has anyone gathering signals or
// writing briefs, so its Product tab is worth showing.
export function hasProduct(org: Org) {
  return org.roles.some((r) => r.capabilities?.some((c) => c === 'company.brief' || c === 'company.signal'))
}

export function ProductTab({ org, can }: { org: Org; can: boolean }) {
  const t = useT()
  const product = useQuery({ queryKey: ['company-product', org.id], queryFn: () => api.companyProduct(org.id) })
  if (!product.data) return null
  // A list the server has nothing for may come as null.
  const briefs = product.data.briefs ?? []
  const signals = product.data.signals ?? []
  const accuracy = product.data.accuracy ?? {}
  return (
    <div className="grid gap-8 lg:grid-cols-[2fr_1fr]">
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.briefs')}</h2>
        <p className="text-[12.5px] text-ink-3">{t('co.briefsHint')}</p>
        {Object.entries(accuracy).filter(([, [, checked]]) => checked > 0).map(([who, [met, checked]]) => (
          <p key={who} className="text-[12.5px] text-ink-2">{t('co.accuracy', { name: nameOf(org, who), met, checked })}</p>
        ))}
        {briefs.length === 0 && <p className="text-[13px] text-ink-3">{t('co.noBriefs')}</p>}
        {briefs.map((b) => <BriefCard key={b.id} org={org} b={b} can={can} />)}
      </section>
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.signals')}</h2>
        {signals.length === 0 && <p className="text-[13px] text-ink-3">{t('co.noSignals')}</p>}
        <ul className="space-y-2">
          {signals.map((s) => (
            <li key={s.id} className="rounded-xl border border-line p-3">
              <p className="text-[13px] font-medium">{s.url ? <a className="underline-offset-2 hover:underline" href={s.url} target="_blank" rel="noreferrer">{s.title}</a> : s.title}</p>
              <p className="text-[12px] text-ink-3">{[s.source, ...(s.seen ?? [])].join(', ')} · {t('co.heard', { n: s.count })}</p>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}

function BriefCard({ org, b, can }: { org: Org; b: CompanyBrief; can: boolean }) {
  const t = useT()
  const qc = useQueryClient()
  const [ref, setRef] = useState('')
  const move = useMutation({
    mutationFn: (state: string) => api.briefState(org.id, b.id, { state, ref: ref || undefined }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['company-product', org.id] }); qc.invalidateQueries({ queryKey: ['needs'] }) },
  })
  const s = b.scores
  return (
    <Card className="space-y-3 p-4">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-[12px] text-ink-3">{t(`co.brief.${b.state}` as 'co.brief.proposed')} · {nameOf(org, b.author)} · {relative(b.created)}</p>
          <h3 className="text-[14px] font-medium">{b.title}</h3>
        </div>
        <span className="rounded-md bg-sunken px-2 py-1 text-[13px] font-semibold tabular-nums" title={t('co.scoreHow', { v: s.value, d: s.differentiation, a: s.adoption, b: s.build_risk, s: s.safety_risk })}>{t('co.score', { n: b.score })}</span>
      </div>
      <p className="text-[12.5px] text-ink-2"><b className="font-medium">{t('co.problem')}</b> {b.problem}</p>
      <p className="text-[12.5px] text-ink-2"><b className="font-medium">{t('co.proposal')}</b> {b.proposal}</p>
      <ul className="space-y-1.5">
        {(b.claims ?? []).map((c, i) => (
          <li key={i} className="text-[12.5px]">
            <span>{c.text}</span>{' '}
            <span className="text-ink-3">({link(c.source) ? <a className="underline-offset-2 hover:underline" href={c.source} target="_blank" rel="noreferrer">{t('co.source')}</a> : c.source})</span>
            {c.quote && <q className="block text-[12px] text-ink-3">{c.quote}</q>}
            {c.flag && <span className="mt-0.5 flex items-center gap-1 text-[12px] text-change"><TriangleAlert size={12} /> {c.flag}</span>}
          </li>
        ))}
      </ul>
      <div className="text-[12.5px]">
        <p className="font-medium">{t('co.predictions')}</p>
        <ul className="list-disc pl-5 text-ink-2">
          {(b.predictions ?? []).map((p, i) => <li key={i}>{p.metric}: {p.expected}</li>)}
        </ul>
      </div>
      {(b.reviews ?? []).map((r) => (
        <div key={r.day} className="rounded-lg bg-sunken/70 p-2 text-[12.5px]">
          <p className="font-medium">{t('co.reviewDay', { n: r.day })}</p>
          <ul>{(r.results ?? []).map((x, i) => <li key={i} className="flex items-center gap-1">{x.met ? <Check size={12} className="text-read" /> : <X size={12} className="text-danger" />} {x.metric}: {x.actual}</li>)}</ul>
        </div>
      ))}
      {b.ref && <p className="text-[12px] text-ink-3">{t('co.shippedIn')} {link(b.ref) ? <a className="underline" href={b.ref} target="_blank" rel="noreferrer">{b.ref}</a> : b.ref}</p>}
      {move.error && <p className="text-[13px] text-danger">{move.error.message}</p>}
      {can && b.state === 'proposed' && (
        <div className="flex gap-2">
          <Button size="sm" variant="primary" onClick={() => move.mutate('accepted')}>{t('co.acceptBrief')}</Button>
          <Button size="sm" variant="ghost" onClick={() => move.mutate('rejected')}>{t('co.rejectBrief')}</Button>
        </div>
      )}
      {can && b.state === 'accepted' && (
        <div className="flex flex-wrap gap-2">
          <input className={field + ' min-w-0 flex-1'} aria-label={t('co.shippedRef')} placeholder={t('co.shippedRef')} value={ref} onChange={(e) => setRef(e.target.value)} />
          <Button size="sm" variant="primary" onClick={() => move.mutate('shipped')}>{t('co.markShipped')}</Button>
          <Button size="sm" variant="ghost" onClick={() => move.mutate('rejected')}>{t('co.rejectBrief')}</Button>
        </div>
      )}
    </Card>
  )
}
