import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Building2, Plus, Upload, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Org } from '../lib/api'
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

type Way = 'template' | 'describe' | 'blank'

function NewCompany({ onClose }: { onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const [way, setWay] = useState<Way>('template')
  const opened = (o: Org) => { qc.invalidateQueries({ queryKey: ['companies'] }); nav(`/companies/${o.id}`) }
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[8vh] z-50 max-h-[86vh] w-[min(640px,calc(100vw-32px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <Dialog.Title className="text-[17px] font-semibold tracking-tight">{t('co.new')}</Dialog.Title>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <Dialog.Description className="mb-4 text-[13px] text-ink-2">{t('co.newHint')}</Dialog.Description>
          <div role="group" aria-label={t('co.startFrom')} className="mb-5 flex gap-1 rounded-xl bg-sunken p-1">
            {(['template', 'describe', 'blank'] as Way[]).map((w) => (
              <button key={w} type="button" aria-pressed={way === w} onClick={() => setWay(w)}
                className={'flex-1 rounded-lg px-3 py-1.5 text-[13px] ' + (way === w ? 'bg-surface font-medium shadow-[var(--shadow-card)]' : 'text-ink-2 hover:text-ink')}>{t(`co.way.${w}` as 'co.way.blank')}</button>
            ))}
          </div>
          {way === 'template' && <FromTemplate onCreated={opened} />}
          {way === 'describe' && <Describe onCreated={opened} />}
          {way === 'blank' && <Blank onCreated={opened} />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function FromTemplate({ onCreated }: { onCreated: (o: Org) => void }) {
  const t = useT()
  const templates = useQuery({ queryKey: ['company-templates'], queryFn: api.companyTemplates })
  const [pick, setPick] = useState('')
  const [name, setName] = useState('')
  const chosen = templates.data?.find((x) => x.id === pick)
  const create = useMutation({ mutationFn: () => api.companyFromTemplate(pick, name.trim() || chosen?.name || ''), onSuccess: onCreated })
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate() }}>
      <fieldset className="grid gap-2 sm:grid-cols-2">
        <legend className="sr-only">{t('co.way.template')}</legend>
        {(templates.data ?? []).map((x) => (
          <label key={x.id} className={'cursor-pointer rounded-xl border p-3 ' + (pick === x.id ? 'border-ink' : 'border-line hover:border-line-strong')}>
            <input type="radio" name="template" value={x.id} className="sr-only" checked={pick === x.id} onChange={() => { setPick(x.id); setName(x.name) }} />
            <span className="block text-[13.5px] font-medium">{x.name}</span>
            <span className="block text-[12px] text-ink-3">{x.members ? t('co.templateHas', { n: x.members, roles: x.roles.join(', ') }) : t('co.templateBlank')}</span>
          </label>
        ))}
      </fieldset>
      {chosen && <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('co.name')}</span>
        <input className={field} value={name} maxLength={60} onChange={(e) => setName(e.target.value)} /></label>}
      {create.error && <p className="text-[13px] text-danger">{create.error.message}</p>}
      <Button type="submit" variant="primary" disabled={!chosen || create.isPending}>{t('co.create')}</Button>
    </form>
  )
}

function Describe({ onCreated }: { onCreated: (o: Org) => void }) {
  const t = useT()
  const [text, setText] = useState('')
  const propose = useMutation({ mutationFn: () => api.describeCompany(text) })
  const create = useMutation({ mutationFn: (file: string) => api.importCompany(file), onSuccess: onCreated })
  const d = propose.data
  return (
    <div className="space-y-4">
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); propose.mutate() }}>
        <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('co.describeLabel')}</span>
          <textarea className={area + ' min-h-28'} value={text} maxLength={4000} placeholder={t('co.describeHint')} onChange={(e) => setText(e.target.value)} /></label>
        {propose.error && <p className="text-[13px] text-danger">{propose.error.message}</p>}
        <Button type="submit" variant={d ? 'secondary' : 'primary'} disabled={!text.trim() || propose.isPending}>{propose.isPending ? t('co.proposing') : d ? t('co.proposeAgain') : t('co.propose')}</Button>
      </form>
      {d && (
        <section aria-label={t('co.proposed')} className="space-y-3 rounded-xl border border-line p-4">
          <div>
            <h3 className="text-[15px] font-semibold">{d.draft.name}</h3>
            {d.draft.industry && <p className="text-[12.5px] text-ink-3">{d.draft.industry}</p>}
            {d.draft.mission && <p className="mt-1 text-[13px] text-ink-2">{d.draft.mission}</p>}
          </div>
          <ul className="space-y-1.5 text-[13px]">
            {d.draft.members.filter((m) => m.kind === 'agent').map((m) => {
              const role = d.draft.roles.find((r) => r.id === m.role)
              return <li key={m.id}><b className="font-medium">{m.name}</b> · {role?.title}{role?.function ? <span className="text-ink-3"> — {role.function}</span> : null}</li>
            })}
          </ul>
          {d.draft.agent_routines.length > 0 && <p className="text-[12.5px] text-ink-3">{t('co.proposalRoutines', { names: d.draft.agent_routines.map((r) => r.name).join(', ') })}</p>}
          {d.dropped.length > 0 && <p className="text-[12.5px] text-change">{t('co.proposalDropped', { names: d.dropped.join(', ') })}</p>}
          {create.error && <p className="text-[13px] text-danger">{create.error.message}</p>}
          <Button variant="primary" disabled={create.isPending} onClick={() => create.mutate(d.file)}>{t('co.createProposal')}</Button>
        </section>
      )}
    </div>
  )
}

function Blank({ onCreated }: { onCreated: (o: Org) => void }) {
  const t = useT()
  const [c, setC] = useState({ name: '', industry: '', mission: '' })
  const create = useMutation({ mutationFn: () => api.createCompany(c), onSuccess: onCreated })
  return (
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
  )
}
