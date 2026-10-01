import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Building2, Plus, Upload, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { CompanyPage } from './CompanyPage'

export const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
export const area = 'min-h-20 w-full rounded-[10px] border border-line bg-bg p-3 text-sm outline-none focus:border-accent'

export function Companies() {
  const { id } = useParams()
  return id ? <CompanyPage id={id} /> : <CompanyList />
}

function CompanyList() {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const list = useQuery({ queryKey: ['companies'], queryFn: api.companies })
  const [creating, setCreating] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const load = useMutation({
    mutationFn: async (f: File) => api.importCompany(await f.text()),
    onSuccess: (o) => { qc.invalidateQueries({ queryKey: ['companies'] }); nav(`/companies/${o.id}`) },
  })
  const items = list.data ?? []
  const actions = (
    <div className="flex gap-2">
      <Button onClick={() => file.current?.click()} disabled={load.isPending}><Upload size={15} /> {t('co.import')}</Button>
      <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} /> {t('co.new')}</Button>
    </div>
  )
  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="mb-1 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><Building2 size={20} /> {t('co.title')}</h1>
          <p className="text-sm text-ink-2">{t('co.subtitle')}</p>
        </div>
        {actions}
      </div>
      <input ref={file} type="file" accept=".yaml,.yml,text/yaml" className="hidden" aria-label={t('co.import')}
        onChange={(e) => { const f = e.target.files?.[0]; if (f) load.mutate(f); e.target.value = '' }} />
      {load.error && <p className="mb-4 text-[13px] text-danger">{load.error.message}</p>}
      {list.isSuccess && items.length === 0 ? (
        <EmptyState icon={<Building2 size={22} />} title={t('co.emptyTitle')} action={actions}>{t('co.empty')}</EmptyState>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {items.map((c) => (
            <Link key={c.id} to={`/companies/${c.id}`}>
              <Card className="p-4 hover:border-line-strong">
                <div className="text-[14.5px] font-medium">{c.name}</div>
                {c.industry && <p className="text-[12.5px] text-ink-3">{c.industry}</p>}
                {c.grant !== 'configure' && <p className="mt-1 text-[12px] text-ink-2">{t(`co.grant.${c.grant}`)}</p>}
              </Card>
            </Link>
          ))}
        </div>
      )}
      {creating && <NewCompany onClose={() => setCreating(false)} />}
    </div>
  )
}

function NewCompany({ onClose }: { onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const [c, setC] = useState({ name: '', industry: '', mission: '' })
  const create = useMutation({
    mutationFn: () => api.createCompany(c),
    onSuccess: (o) => { qc.invalidateQueries({ queryKey: ['companies'] }); nav(`/companies/${o.id}`) },
  })
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[8vh] z-50 max-h-[86vh] w-[min(560px,calc(100vw-32px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <Dialog.Title className="text-[17px] font-semibold tracking-tight">{t('co.new')}</Dialog.Title>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <Dialog.Description className="mb-4 text-[13px] text-ink-2">{t('co.newHint')}</Dialog.Description>
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate() }}>
            <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('co.name')}</span>
              <input className={field} value={c.name} maxLength={60} onChange={(e) => setC({ ...c, name: e.target.value })} /></label>
            <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('co.industry')}</span>
              <input className={field} value={c.industry} maxLength={120} placeholder={t('co.industryHint')} onChange={(e) => setC({ ...c, industry: e.target.value })} /></label>
            <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('co.mission')}</span>
              <textarea className={area} value={c.mission} maxLength={2000} onChange={(e) => setC({ ...c, mission: e.target.value })} /></label>
            {create.error && <p className="text-[13px] text-danger">{create.error.message}</p>}
            <Button type="submit" variant="primary" disabled={!c.name.trim() || create.isPending}>{t('co.create')}</Button>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
