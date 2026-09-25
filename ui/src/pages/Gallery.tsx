import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BadgeCheck, Check, Flag, LibraryBig, Search, ShieldAlert, X } from 'lucide-react'
import { motion } from 'motion/react'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Code } from '../components/Code'
import { CapabilityChip, capabilityLabel, capRisk } from '../components/RoutineCard'
import { Button, Card, EmptyState, PageSkeleton, RiskBadge } from '../components/ui'
import { api, type GalleryItem } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

type Filter = 'all' | 'quiet' | 'nothing-out'

const filters: { id: Filter; label: 'gallery.all' | 'gallery.nothingOut' | 'gallery.quiet'; test: (g: GalleryItem) => boolean }[] = [
  { id: 'all', label: 'gallery.all', test: () => true },
  { id: 'nothing-out', label: 'gallery.nothingOut', test: (g) => !g.report.sends && !g.routine.manifest.capabilities.some((c) => c.startsWith('http.')) },
  { id: 'quiet', label: 'gallery.quiet', test: (g) => g.report.risk === 'read' || g.report.risk === 'notify' },
]

export function Gallery() {
  const t = useT()
  const q = useQuery({ queryKey: ['gallery'], queryFn: () => api.gallery() })
  const [filter, setFilter] = useState<Filter>('all')
  const [text, setText] = useState('')
  const [open, setOpen] = useState<GalleryItem | null>(null)
  const list = useMemo(() => {
    const f = filters.find((x) => x.id === filter)!
    const needle = text.trim().toLowerCase()
    return (q.data ?? []).filter((g) => f.test(g) && (!needle || (g.routine.name + ' ' + g.routine.description).toLowerCase().includes(needle)))
  }, [q.data, filter, text])

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6">
        <h1 className="text-[22px] font-semibold tracking-tight">{t('gallery.title')}</h1>
        <p className="mt-1 max-w-2xl text-sm text-ink-2">{t('gallery.subtitle')}</p>
      </div>

      <div className="mb-5 flex flex-wrap items-center gap-2">
        <div className="flex h-9 min-w-[220px] flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 focus-within:border-accent sm:max-w-xs">
          <Search size={15} className="text-ink-3" />
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder={t('gallery.search')} aria-label={t('gallery.searchLabel')} className="flex-1 bg-transparent text-sm outline-none" />
        </div>
        <div role="tablist" aria-label={t('gallery.filter')} className="flex flex-wrap gap-1.5">
          {filters.map((f) => (
            <button key={f.id} role="tab" aria-selected={filter === f.id} onClick={() => setFilter(f.id)}
              className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', filter === f.id ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
              {t(f.label)}
            </button>
          ))}
        </div>
      </div>

      {q.isPending ? <PageSkeleton /> : q.error ? (
        <EmptyState icon={<LibraryBig size={22} />} title={t('gallery.error')}>{q.error.message}</EmptyState>
      ) : list.length === 0 ? (
        <EmptyState icon={<LibraryBig size={22} />} title={t('gallery.emptyTitle')}>{t('gallery.emptyText')}</EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {list.map((g, i) => (
            <motion.div key={g.id} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: Math.min(i, 8) * 0.03 }}>
              <Card role="button" tabIndex={0} onClick={() => setOpen(g)} onKeyDown={(k) => k.key === 'Enter' && setOpen(g)}
                className="flex h-full cursor-pointer flex-col p-5 transition hover:border-line-strong hover:shadow-[var(--shadow-pop)]">
                <div className="mb-2 flex items-start justify-between gap-3">
                  <div className="text-[15px] font-medium leading-snug">{g.routine.name}</div>
                  {g.installed ? <span className="flex shrink-0 items-center gap-1 text-[12px] text-read"><Check size={13} /> {t('gallery.installed')}</span> : <RiskBadge risk={g.report.risk} />}
                </div>
                <p className="mb-4 line-clamp-2 text-[13px] text-ink-2">{g.routine.description}</p>
                <div className="mt-auto flex flex-wrap gap-1.5">
                  {g.routine.manifest.capabilities.map((c) => <CapabilityChip key={c} entry={c} />)}
                </div>
                <div className="mt-4 flex items-center gap-1.5 border-t border-line pt-3 text-[12px] text-ink-3">
                  {g.report.verified ? <BadgeCheck size={14} className="text-read" /> : <ShieldAlert size={14} className="text-danger" />}
                  {g.report.verified ? t('gallery.verified') : t('gallery.unverified')} · {g.author_name || g.author}
                </div>
              </Card>
            </motion.div>
          ))}
        </div>
      )}
      <Detail item={open} onClose={() => setOpen(null)} />
    </div>
  )
}

function Detail({ item, onClose }: { item: GalleryItem | null; onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const install = useMutation({
    mutationFn: (id: string) => api.installFromGallery(id),
    onSuccess: (r) => { qc.invalidateQueries(); onClose(); nav(`/routines/${r.id}`) },
  })
  const g = item
  return (
    <Dialog.Root open={!!g} onOpenChange={(o) => { if (!o) { install.reset(); onClose() } }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed inset-x-0 bottom-0 z-50 max-h-[92vh] overflow-y-auto rounded-t-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none sm:inset-x-auto sm:left-1/2 sm:top-[6vh] sm:bottom-auto sm:w-[min(760px,calc(100vw-32px))] sm:-translate-x-1/2 sm:rounded-2xl">
          {g && (
            <>
              <div className="mb-5 flex items-start justify-between gap-4">
                <div>
                  <Dialog.Title className="text-[18px] font-semibold tracking-tight">{g.routine.name}</Dialog.Title>
                  <Dialog.Description className="mt-1 text-sm text-ink-2">{g.routine.description}</Dialog.Description>
                  <div className="mt-2 font-mono text-[11.5px] text-ink-3">{g.author_name || g.author} · {g.hash.slice(0, 16)}</div>
                </div>
                <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
              </div>

              <section aria-label={t('gallery.touches')} className="mb-5">
                <h3 className="mb-2 text-[13px] font-medium text-ink-2">{t('gallery.touchesTitle')}</h3>
                <div className="space-y-2">
                  {g.routine.manifest.capabilities.map((c) => {
                    const risk = capRisk[c.split(':')[0]] ?? 'read'
                    return (
                      <div key={c} className="flex items-center justify-between gap-4 rounded-xl border border-line px-4 py-3">
                        <div>
                          <div className="text-[14px] font-medium">{capabilityLabel(c)}</div>
                          <div className="text-[12.5px] text-ink-3">{t(`gallery.risk.${risk}`)}</div>
                        </div>
                        <RiskBadge risk={risk} />
                      </div>
                    )
                  })}
                </div>
              </section>

              <section className={cn('mb-5 rounded-xl p-4 text-[13px]', g.report.verified ? 'bg-read-soft text-read' : 'bg-danger-soft text-danger')}>
                {g.report.verified ? (
                  <div className="flex gap-2"><BadgeCheck size={16} className="mt-0.5 shrink-0" />
                    <span>{t('gallery.verifiedText', { count: g.routine.tests.length, uses: g.report.uses.map(capabilityLabel).join(', ').toLowerCase() })}</span>
                  </div>
                ) : (
                  <div>
                    <div className="mb-1 flex items-center gap-2 font-medium"><ShieldAlert size={16} /> {t('gallery.refuse')}</div>
                    <ul className="list-disc pl-6">{g.report.problems?.map((p) => <li key={p}>{p}</li>)}</ul>
                  </div>
                )}
              </section>

              <section className="mb-5">
                <h3 className="mb-2 text-[13px] font-medium text-ink-2">{t('gallery.code')}</h3>
                <Code code={g.routine.code} />
              </section>

              <div className="flex flex-wrap items-center justify-between gap-3">
                <a className="flex items-center gap-1.5 text-[12.5px] text-ink-3 hover:text-ink" target="_blank" rel="noreferrer"
                  href={`https://github.com/denerFernandes/zodim-gallery/issues/new?template=report.yml&title=${encodeURIComponent('Report: ' + g.id)}`}>
                  <Flag size={13} /> {t('gallery.report')}
                </a>
                {install.error && <p className="text-[13px] text-danger">{install.error.message}</p>}
                {g.installed ? (
                  <span className="flex items-center gap-1.5 text-[13px] text-read"><Check size={15} /> {t('gallery.alreadyInstalled')}</span>
                ) : (
                  <Button variant="primary" disabled={!g.report.verified || install.isPending} onClick={() => install.mutate(g.id)}>{t('gallery.install')}</Button>
                )}
              </div>
            </>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
