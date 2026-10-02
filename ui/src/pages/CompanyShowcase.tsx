import { useMutation, useQuery } from '@tanstack/react-query'
import { ExternalLink, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Field } from '../components/Modal'
import { Button, Card, Switch } from '../components/ui'
import { api, type Org } from '../lib/api'
import { useT } from '../lib/i18n'
import { field, SectionHead } from './Companies'

// ShowcaseTab is the company's optional public page: off by default, with
// only what its person puts there.
export function ShowcaseTab({ org, onSaved }: { org: Org; onSaved: (o: Org) => void }) {
  const t = useT()
  const sc = org.showcase ?? {}
  const [slug, setSlug] = useState(sc.slug ?? '')
  const [link, setLink] = useState({ title: '', url: '' })
  const product = useQuery({ queryKey: ['company-product', org.id], queryFn: () => api.companyProduct(org.id) })
  const media = useQuery({ queryKey: ['company-media', org.id], queryFn: () => api.companyMedia(org.id) })
  const toggle = useMutation({ mutationFn: (on: boolean) => api.saveShowcase(org.id, on, slug.trim().toLowerCase()), onSuccess: onSaved })
  const add = useMutation({ mutationFn: (item: { kind: string; ref?: string; title?: string; url?: string }) => api.addShowcaseItem(org.id, item), onSuccess: (o) => { onSaved(o); setLink({ title: '', url: '' }) } })
  const remove = useMutation({ mutationFn: (id: string) => api.removeShowcaseItem(org.id, id), onSuccess: onSaved })
  const shipped = (product.data?.briefs ?? []).filter((b) => b.state === 'shipped' && !(sc.items ?? []).some((i) => i.ref === b.id))
  const videos = (media.data ?? []).filter((m) => m.kind === 'video' && !(sc.items ?? []).some((i) => i.ref === m.id))
  // Only the address the server saved and checked goes in the link.
  const address = sc.slug ? `${location.origin}/showcase/${encodeURIComponent(sc.slug)}` : ''
  return (
    <div className="max-w-3xl space-y-6">
      <section className="space-y-3">
        <SectionHead title={t('co.showcase')} hint={t('co.showcaseHint')} />
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-48 flex-1"><Field label={t('co.showcaseSlug')}><input className={field} value={slug} maxLength={40} placeholder="lume-moda" onChange={(e) => setSlug(e.target.value)} /></Field></div>
          <div className="flex items-center gap-2 pb-2 text-[13px]"><span>{t('co.showcaseOn')}</span><Switch on={!!sc.on} disabled={!slug.trim() || toggle.isPending} onChange={(on) => toggle.mutate(on)} label={t('co.showcaseOn')} /></div>
        </div>
        {sc.on && address && <a href={address} target="_blank" rel="noreferrer" className="inline-flex max-w-full items-center gap-1 break-all text-[13px] underline-offset-2 hover:underline"><ExternalLink size={13} /> {address}</a>}
        {toggle.error && <p className="text-[13px] text-danger">{toggle.error.message}</p>}
      </section>
      <section className="space-y-2" aria-label={t('co.showcaseItems')}>
        <h3 className="text-[13.5px] font-semibold">{t('co.showcaseItems')}</h3>
        {(sc.items ?? []).length === 0 && <p className="text-[13px] text-ink-3">{t('co.showcaseEmpty')}</p>}
        {(sc.items ?? []).map((i) => (
          <Card key={i.id} className="flex items-center gap-3 p-3">
            <div className="min-w-0 flex-1">
              <p className="text-[12px] text-ink-3">{t(`co.show.${i.kind}` as 'co.show.link')}</p>
              <p className="truncate text-[13.5px] font-medium">{i.title}</p>
            </div>
            <Button size="sm" variant="ghost" aria-label={t('co.forget', { name: i.title })} onClick={() => remove.mutate(i.id)}><Trash2 size={14} /></Button>
          </Card>
        ))}
      </section>
      <section className="space-y-3">
        <h3 className="text-[13.5px] font-semibold">{t('co.showcaseAdd')}</h3>
        {shipped.map((b) => <Button key={b.id} size="sm" onClick={() => add.mutate({ kind: 'brief', ref: b.id })}><Plus size={13} /> {t('co.show.brief')}: {b.title}</Button>)}
        {videos.map((m) => <Button key={m.id} size="sm" onClick={() => add.mutate({ kind: 'video', ref: m.id })}><Plus size={13} /> {t('co.show.video')}: {m.title || m.id}</Button>)}
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); add.mutate({ kind: 'link', ...link }) }}>
          <input className={field + ' min-w-0 flex-1 basis-40'} aria-label={t('co.noteTitle')} placeholder={t('co.noteTitle')} value={link.title} onChange={(e) => setLink({ ...link, title: e.target.value })} />
          <input className={field + ' min-w-0 flex-1 basis-56'} aria-label={t('co.showcaseURL')} placeholder="https://" value={link.url} onChange={(e) => setLink({ ...link, url: e.target.value })} />
          <Button type="submit" size="sm" disabled={!link.title.trim() || !link.url.trim() || add.isPending}><Plus size={13} /> {t('co.show.link')}</Button>
        </form>
        {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
      </section>
    </div>
  )
}
