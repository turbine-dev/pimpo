import { useQuery } from '@tanstack/react-query'
import { Card } from '../components/ui'
import { api, type Org } from '../lib/api'
import { useT } from '../lib/i18n'

const usd = (n: number) => `$${n.toFixed(2)}`

// Performance is how each agent did, from what it did.
export function Performance({ org }: { org: Org }) {
  const t = useT()
  const perf = useQuery({ queryKey: ['company-performance', org.id], queryFn: () => api.companyPerformance(org.id) })
  const rows = perf.data ?? []
  if (rows.length === 0) return null
  const briefs = rows.some((r) => r.briefs_checked)
  const videos = rows.some((r) => r.videos)
  return (
    <Card className="overflow-x-auto">
      <h2 className="px-4 pt-4 text-[13.5px] font-semibold">{t('co.performance')}</h2>
      <table className="w-full text-[13px]">
        <thead className="text-left text-[12px] text-ink-3">
          <tr>
            <th className="px-4 py-2 font-medium">{t('co.member')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfDone')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfFailed')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfMinutes')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perDone')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfTasks')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfQuestions')}</th>
            <th className="px-2 py-2 font-medium">{t('co.perfEarned')}</th>
            {briefs && <th className="px-2 py-2 font-medium">{t('co.perfBriefs')}</th>}
            {videos && <th className="px-4 py-2 font-medium">{t('co.perfVideos')}</th>}
          </tr>
        </thead>
        <tbody className="divide-y divide-line tabular-nums">
          {rows.map((r) => (
            <tr key={r.member}>
              <td className="px-4 py-2 font-medium">{r.name}</td>
              <td className="px-2 py-2">{r.done}</td>
              <td className="px-2 py-2">{r.failed}</td>
              <td className="px-2 py-2">{r.done ? Math.round(r.minutes) : '—'}</td>
              <td className="px-2 py-2">{r.done ? usd(r.cost_each) : '—'}</td>
              <td className="px-2 py-2">{r.tasks_done}{r.tasks_blocked ? <span className="text-change"> · {t('co.perfBlocked', { n: r.tasks_blocked })}</span> : null}</td>
              <td className="px-2 py-2">{r.questions}</td>
              <td className="px-2 py-2">{r.earned}</td>
              {briefs && <td className="px-2 py-2">{r.briefs_checked ? `${r.briefs_met}/${r.briefs_checked}` : '—'}</td>}
              {videos && <td className="px-4 py-2">{r.videos ? `${r.videos_ok}/${r.videos}` : '—'}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  )
}
