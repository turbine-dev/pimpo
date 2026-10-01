import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Field } from '../components/Modal'
import { Button, Card } from '../components/ui'
import { api, type Budget, type Org } from '../lib/api'
import { useT } from '../lib/i18n'
import { field } from './Companies'

const usd = (n?: number) => `$${(n ?? 0).toFixed(2)}`

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card className="p-4">
      <p className="text-[12px] text-ink-3">{label}</p>
      <p className="text-[20px] font-semibold tabular-nums">{value}</p>
      {hint && <p className="text-[11.5px] text-ink-3">{hint}</p>}
    </Card>
  )
}

export function CostsTab({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const costs = useQuery({ queryKey: ['company-costs', org.id], queryFn: () => api.companyCosts(org.id) })
  const [budget, setBudget] = useState<Budget>(org.budget ?? {})
  const [salaries, setSalaries] = useState<Record<string, number | undefined>>(() => Object.fromEntries(org.members.map((m) => [m.id, m.budget?.month_usd])))
  const save = useMutation({
    mutationFn: async () => {
      let o = await api.saveCompany(org.id, { ...org, budget })
      for (const m of org.members.filter((x) => x.kind === 'agent' && (x.budget?.month_usd ?? undefined) !== salaries[x.id])) {
        o = await api.saveMember(org.id, { ...m, budget: { ...m.budget, month_usd: salaries[m.id] || undefined } })
      }
      return o
    },
    onSuccess: (o) => { onSaved(o); qc.invalidateQueries({ queryKey: ['company-costs', org.id] }) },
  })
  const c = costs.data
  if (!c) return null
  const sub = c.subscription
  const agents = org.members.filter((m) => m.kind === 'agent')
  return (
    <div className="space-y-5">
      <div className="grid gap-3 sm:grid-cols-4">
        <Stat label={t('co.costToday')} value={usd(c.company.day)} hint={budget.day_usd ? t('co.ofLimit', { limit: usd(budget.day_usd) }) : undefined} />
        <Stat label={t('co.costMonth')} value={usd(c.company.month)} hint={budget.month_usd ? t('co.ofLimit', { limit: usd(budget.month_usd) }) : undefined} />
        <Stat label={t('co.forecast')} value={usd(c.forecast_month)} hint={t('co.forecastHint')} />
        <Stat label={t('co.subscription')} value={usd(sub?.company.month)} hint={t('co.subscriptionHint')} />
      </div>
      {c.outcomes.task && <p className="text-[13px] text-ink-2">{t('co.perTask', { count: c.outcomes.task.count, each: usd(c.outcomes.task.cost_each) })}</p>}
      <Card className="overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead className="text-left text-[12px] text-ink-3">
            <tr><th className="px-4 py-2 font-medium">{t('co.member')}</th><th className="px-2 py-2 font-medium">{t('co.costMonth')}</th><th className="px-2 py-2 font-medium">{t('co.subscription')}</th>
              <th className="px-2 py-2 font-medium">{t('co.done')}</th><th className="px-2 py-2 font-medium">{t('co.perDone')}</th><th className="px-2 py-2 font-medium">{t('co.forecast')}</th><th className="px-4 py-2 font-medium">{t('co.salary')}</th></tr>
          </thead>
          <tbody className="divide-y divide-line">
            {agents.map((m) => {
              const pm = c.per_member[m.id]
              return (
                <tr key={m.id}>
                  <td className="px-4 py-2 font-medium">{m.name}</td>
                  <td className="px-2 py-2 tabular-nums">{usd(c.members[m.id]?.month)}</td>
                  <td className="px-2 py-2 tabular-nums text-ink-3">{usd(sub?.members[m.id]?.month)}</td>
                  <td className="px-2 py-2 tabular-nums">{pm?.done ?? 0}</td>
                  <td className="px-2 py-2 tabular-nums">{pm?.done ? usd(pm.cost_per_done) : '–'}</td>
                  <td className="px-2 py-2 tabular-nums">{usd(pm?.forecast_month)}</td>
                  <td className="px-4 py-2">
                    <input type="number" min={0} step={1} disabled={!can} aria-label={t('co.salaryOf', { name: m.name })} className={field + ' h-8 w-24'}
                      value={salaries[m.id] ?? ''} onChange={(e) => setSalaries({ ...salaries, [m.id]: e.target.value ? Number(e.target.value) : undefined })} />
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </Card>
      <fieldset disabled={!can} className="space-y-3">
        <legend className="mb-1 text-[15px] font-semibold">{t('co.companyBudget')}</legend>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label={t('co.dayLimit')}><input type="number" min={0} step={0.5} className={field} value={budget.day_usd ?? ''} onChange={(e) => setBudget({ ...budget, day_usd: e.target.value ? Number(e.target.value) : undefined })} /></Field>
          <Field label={t('co.monthLimit')}><input type="number" min={0} step={1} className={field} value={budget.month_usd ?? ''} onChange={(e) => setBudget({ ...budget, month_usd: e.target.value ? Number(e.target.value) : undefined })} /></Field>
          <Field label={t('co.onLimit')}>
            <select className={field} value={budget.on_limit ?? 'pause'} onChange={(e) => setBudget({ ...budget, on_limit: e.target.value as Budget['on_limit'] })}>
              <option value="pause">{t('co.onLimit.pause')}</option>
              <option value="warn">{t('co.onLimit.warn')}</option>
            </select>
          </Field>
        </div>
        <label className="flex items-start gap-2 text-[13px]">
          <input type="checkbox" className="mt-0.5 size-4 accent-[var(--color-accent)]" checked={!!budget.subscription} onChange={(e) => setBudget({ ...budget, subscription: e.target.checked })} />
          <span>{t('co.countSubscription')}<span className="block text-[12px] text-ink-3">{t('co.countSubscriptionHint')}</span></span>
        </label>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        {can && <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>}
      </fieldset>
    </div>
  )
}
