import { useQuery } from '@tanstack/react-query'
import { CircleCheck, TriangleAlert } from 'lucide-react'
import { Card } from '../components/ui'
import { api, type CompanyMedia, type Org } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'

// hasMedia says whether anyone in the company makes media.
export function hasMedia(org: Org) {
  return org.roles.some((r) => r.capabilities?.some((c) => c.startsWith('media.')))
}

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

export function MediaTab({ org }: { org: Org }) {
  const t = useT()
  const list = useQuery({ queryKey: ['company-media', org.id], queryFn: () => api.companyMedia(org.id) })
  const items = list.data ?? []
  const videos = items.filter((m) => m.kind === 'video')
  const parts = items.filter((m) => m.kind !== 'video')
  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.videos')}</h2>
        <p className="text-[12.5px] text-ink-3">{t('co.videosHint')}</p>
        {videos.length === 0 && <p className="text-[13px] text-ink-3">{t('co.noVideos')}</p>}
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {videos.map((m) => <Video key={m.id} org={org} m={m} />)}
        </div>
      </section>
      {parts.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-[15px] font-semibold">{t('co.mediaParts')}</h2>
          <ul className="grid gap-3 md:grid-cols-3">
            {parts.map((m) => (
              <li key={m.id} className="space-y-1 rounded-xl border border-line p-3">
                {m.kind === 'image'
                  ? <img src={api.mediaURL(org.id, m.id)} alt={m.title ?? m.id} className="max-h-40 w-full rounded-lg object-contain" />
                  : <audio controls preload="none" src={api.mediaURL(org.id, m.id)} className="w-full" aria-label={m.title ?? m.id} />}
                <p className="truncate text-[12px] text-ink-3">{[m.title, nameOf(org, m.member), relative(m.created)].filter(Boolean).join(' · ')}</p>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

function Video({ org, m }: { org: Org; m: CompanyMedia }) {
  const t = useT()
  const problems = m.check?.problems ?? []
  return (
    <Card className="space-y-2 p-3">
      <video controls preload="metadata" src={api.mediaURL(org.id, m.id)} className={m.format === 'short' ? 'mx-auto max-h-96 rounded-lg' : 'w-full rounded-lg'} aria-label={m.title ?? m.id} />
      <p className="text-[13.5px] font-medium">{m.title || m.id}</p>
      <p className="text-[12px] text-ink-3">{[t(`co.format.${m.format}` as 'co.format.short'), `${Math.round(m.seconds ?? 0)} s`, nameOf(org, m.member), relative(m.created)].join(' · ')}</p>
      {problems.length === 0
        ? <p className="flex items-center gap-1 text-[12.5px] text-read"><CircleCheck size={13} /> {t('co.checksPass')}</p>
        : <ul className="space-y-0.5 text-[12.5px] text-change">{problems.map((p) => <li key={p} className="flex items-center gap-1"><TriangleAlert size={12} /> {p}</li>)}</ul>}
    </Card>
  )
}
