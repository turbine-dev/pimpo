import * as Dialog from '@radix-ui/react-dialog'
import * as Tabs from '@radix-ui/react-tabs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Download, Pencil, Plus, Trash2, X } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { CapabilityPicker } from '../components/CapabilityPicker'
import { ModelChoice } from '../components/ModelChoice'
import { OrgChart } from '../components/OrgChart'
import { Button, Card, PageSkeleton, Switch } from '../components/ui'
import { api, type CompanyRole, type Department, type Member, type Org } from '../lib/api'
import { useT } from '../lib/i18n'
import { below, deptColor, slug, unique } from '../lib/org'
import { area, field } from './Companies'

// Deleting reads in the danger color on a plain button, which keeps its
// contrast in both themes.
const danger = 'text-danger hover:bg-danger-soft hover:text-danger'

const colors = ['chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5']

export function CompanyPage({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const org = useQuery({ queryKey: ['company', id], queryFn: () => api.company(id) })
  const me = useQuery({ queryKey: ['state'], queryFn: api.state })
  const [member, setMember] = useState<Member | null>(null)
  const [role, setRole] = useState<CompanyRole | null>(null)
  const [dept, setDept] = useState<Department | null>(null)
  const [editing, setEditing] = useState(false)
  const done = (o: Org) => qc.setQueryData(['company', id], o)
  const move = useMutation({
    mutationFn: ({ m, boss }: { m: Member; boss: string }) => api.saveMember(id, { ...m, reports_to: boss }),
    onSuccess: done,
  })
  const remove = useMutation({
    mutationFn: () => api.deleteCompany(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['companies'] }); nav('/companies') },
  })
  if (org.isLoading) return <PageSkeleton />
  if (org.error || !org.data) return <p className="text-sm text-danger">{org.error?.message}</p>
  const o = org.data
  const can = o.grant === 'configure'
  const newMember = (): Member => ({ id: '', kind: 'agent', name: '', role: o.roles[0]?.id, reports_to: 'ceo', state: 'active' })
  return (
    <div className="mx-auto max-w-6xl">
      <Link to="/companies" className="mb-3 inline-flex items-center gap-1 text-[13px] text-ink-3 hover:text-ink"><ArrowLeft size={14} /> {t('co.title')}</Link>
      <div className="mb-5 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-[22px] font-semibold tracking-tight">{o.name}</h1>
          {o.industry && <p className="text-sm text-ink-2">{o.industry}</p>}
          {o.mission && <p className="mt-1 max-w-2xl text-[13px] text-ink-3">{o.mission}</p>}
        </div>
        <div className="flex flex-wrap gap-2">
          <a href={`/api/companies/${id}/export`} download className="inline-flex h-9 items-center gap-1.5 rounded-[10px] border border-line bg-surface px-3 text-[13.5px] hover:border-line-strong"><Download size={15} /> {t('co.export')}</a>
          {can && <Button onClick={() => setEditing(true)}><Pencil size={15} /> {t('co.edit')}</Button>}
          {o.person === me.data?.person && <Button variant="ghost" className={danger} onClick={() => window.confirm(t('co.deleteAsk', { name: o.name })) && remove.mutate()}><Trash2 size={15} /> {t('co.delete')}</Button>}
        </div>
      </div>

      <Tabs.Root defaultValue="chart">
        <Tabs.List className="mb-5 flex gap-1 border-b border-line" aria-label={t('co.sections')}>
          {[['chart', t('co.tab.chart')], ['roles', t('co.tab.roles', { n: o.roles.length })]].map(([v, l]) => (
            <Tabs.Trigger key={v} value={v} className="-mb-px border-b-2 border-transparent px-3 py-2.5 text-[13.5px] text-ink-3 hover:text-ink data-[state=active]:border-ink data-[state=active]:font-medium data-[state=active]:text-ink">{l}</Tabs.Trigger>
          ))}
        </Tabs.List>

        <Tabs.Content value="chart" className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            {o.departments.map((d) => (
              <button key={d.id} type="button" disabled={!can} onClick={() => setDept(d)} className="inline-flex items-center gap-1.5 rounded-full border border-line px-2.5 py-1 text-[12.5px] hover:border-line-strong">
                <span className="size-2 rounded-full" style={{ background: deptColor(o, d.id) }} aria-hidden /> {d.name}
              </button>
            ))}
            {can && <Button size="sm" variant="ghost" onClick={() => setDept({ id: '', name: '', color: colors[o.departments.length % colors.length] })}><Plus size={14} /> {t('co.newDepartment')}</Button>}
            <span className="flex-1" />
            {can && (o.roles.length > 0
              ? <Button variant="primary" onClick={() => setMember(newMember())}><Plus size={15} /> {t('co.hire')}</Button>
              : <p className="text-[12.5px] text-ink-3">{t('co.roleFirst')}</p>)}
          </div>
          {can && o.members.length > 1 && <p className="hidden text-[12.5px] text-ink-3 md:block">{t('co.dragHint')}</p>}
          <OrgChart org={o} onOpen={setMember} onMove={can ? (m, boss) => { const x = o.members.find((y) => y.id === m); if (x) move.mutate({ m: x, boss }) } : undefined} />
          {move.error && <p className="text-[13px] text-danger">{move.error.message}</p>}
        </Tabs.Content>

        <Tabs.Content value="roles" className="space-y-3">
          {can && <Button variant="primary" onClick={() => setRole({ id: '', title: '' })}><Plus size={15} /> {t('co.newRole')}</Button>}
          {o.roles.length === 0 && <p className="text-[13px] text-ink-3">{t('co.noRoles')}</p>}
          <div className="grid gap-3 sm:grid-cols-2">
            {o.roles.map((r) => {
              const holders = o.members.filter((m) => m.role === r.id)
              return (
                <Card key={r.id} className="flex items-start gap-3 p-4">
                  <div className="min-w-0 flex-1">
                    <div className="text-[14.5px] font-medium">{r.title}</div>
                    {r.function && <p className="line-clamp-2 text-[12.5px] text-ink-3">{r.function}</p>}
                    <p className="mt-1 text-[12px] text-ink-2">{holders.length ? holders.map((m) => m.name).join(', ') : t('co.nobodyHolds')}</p>
                  </div>
                  {can && <Button size="sm" variant="ghost" aria-label={t('co.editRole', { name: r.title })} onClick={() => setRole(r)}><Pencil size={14} /></Button>}
                </Card>
              )
            })}
          </div>
        </Tabs.Content>
      </Tabs.Root>

      {member && <MemberDialog org={o} start={member} can={can} onClose={() => setMember(null)} onSaved={done} />}
      {role && <RoleDialog org={o} start={role} onClose={() => setRole(null)} onSaved={done} />}
      {dept && <DepartmentDialog org={o} start={dept} onClose={() => setDept(null)} onSaved={done} />}
      {editing && <CompanyDialog org={o} onClose={() => setEditing(false)} onSaved={done} />}
    </div>
  )
}

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const t = useT()
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content aria-describedby={undefined} className="fixed left-1/2 top-[6vh] z-50 max-h-[88vh] w-[min(620px,calc(100vw-32px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <Dialog.Title className="text-[17px] font-semibold tracking-tight">{title}</Dialog.Title>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{label}</span>{children}</label>
}

const lines = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean)

function MemberDialog({ org, start, can, onClose, onSaved }: { org: Org; start: Member; can: boolean; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [m, setM] = useState(start)
  const [models, setModels] = useState<string[] | null>(start.models?.length ? start.models : null)
  const isNew = start.id === ''
  const seat = m.kind === 'person'
  const save = useMutation({
    mutationFn: () => api.saveMember(org.id, { ...m, id: m.id || unique(slug(m.name, 'membro'), org.members.map((x) => x.id)), models: models ?? undefined }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteMember(org.id, m.id), onSuccess: (o) => { onSaved(o); onClose() } })
  const under = below(org, m.id)
  const bosses = org.members.filter((x) => x.id !== m.id && !under.has(x.id))
  return (
    <Modal title={isNew ? t('co.hire') : m.name} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <fieldset disabled={!can} className="space-y-4">
          <div className="flex gap-3">
            {!seat && <Field label={t('co.avatar')}><input className={field + ' w-20 text-center text-[18px]'} value={m.avatar ?? ''} maxLength={4} onChange={(e) => setM({ ...m, avatar: e.target.value })} /></Field>}
            <div className="flex-1"><Field label={t('co.memberName')}><input className={field} value={m.name} maxLength={60} onChange={(e) => setM({ ...m, name: e.target.value })} /></Field></div>
          </div>
          {seat ? (
            <Field label={t('co.seatTitle')}><input className={field} value={m.title ?? ''} maxLength={60} onChange={(e) => setM({ ...m, title: e.target.value })} /></Field>
          ) : (<>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={t('co.role')}>
                <select className={field} value={m.role ?? ''} onChange={(e) => setM({ ...m, role: e.target.value })}>
                  {org.roles.map((r) => <option key={r.id} value={r.id}>{r.title}</option>)}
                </select>
              </Field>
              <Field label={t('co.department')}>
                <select className={field} value={m.department ?? ''} onChange={(e) => setM({ ...m, department: e.target.value || undefined })}>
                  <option value="">{t('co.noDepartment')}</option>
                  {org.departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                </select>
              </Field>
            </div>
            <Field label={t('co.reportsTo')}>
              <select className={field} value={m.reports_to ?? 'ceo'} onChange={(e) => setM({ ...m, reports_to: e.target.value })}>
                {bosses.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
              </select>
            </Field>
            <Field label={t('co.persona')}><textarea className={area} value={m.persona ?? ''} maxLength={2000} placeholder={t('co.personaHint')} onChange={(e) => setM({ ...m, persona: e.target.value })} /></Field>
            <ModelChoice value={models} onChange={setModels} legend={t('co.models')} />
            <Switch on={m.state !== 'paused'} onChange={(on) => setM({ ...m, state: on ? 'active' : 'paused' })} label={t('co.working')} />
          </>)}
        </fieldset>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        {remove.error && <p className="text-[13px] text-danger">{remove.error.message}</p>}
        {can && (
          <div className="flex gap-2">
            <Button type="submit" variant="primary" disabled={!m.name.trim() || (!seat && !m.role) || models?.length === 0 || save.isPending}>{t('common.save')}</Button>
            {!isNew && !seat && <Button type="button" variant="ghost" className={danger} onClick={() => window.confirm(t('co.letGoAsk', { name: m.name })) && remove.mutate()}>{t('co.letGo')}</Button>}
          </div>
        )}
      </form>
    </Modal>
  )
}

function RoleDialog({ org, start, onClose, onSaved }: { org: Org; start: CompanyRole; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [r, setR] = useState(start)
  const [duties, setDuties] = useState((start.responsibilities ?? []).join('\n'))
  const [deliverables, setDeliverables] = useState((start.deliverables ?? []).join('\n'))
  const [kinds, setKinds] = useState((start.account_kinds ?? []).join(', '))
  const [models, setModels] = useState<string[] | null>(start.models?.length ? start.models : null)
  const save = useMutation({
    mutationFn: () => api.saveRole(org.id, {
      ...r, id: r.id || unique(slug(r.title, 'cargo'), org.roles.map((x) => x.id)), responsibilities: lines(duties), deliverables: lines(deliverables),
      account_kinds: kinds.split(',').map((x) => x.trim()).filter(Boolean), models: models ?? undefined,
    }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteRole(org.id, r.id), onSuccess: (o) => { onSaved(o); onClose() } })
  return (
    <Modal title={start.id ? t('co.editRole', { name: start.title }) : t('co.newRole')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.roleTitle')}><input className={field} value={r.title} maxLength={60} onChange={(e) => setR({ ...r, title: e.target.value })} /></Field>
        <Field label={t('co.function')}><textarea className={area} value={r.function ?? ''} maxLength={4000} placeholder={t('co.functionHint')} onChange={(e) => setR({ ...r, function: e.target.value })} /></Field>
        <Field label={t('co.responsibilities')}><textarea className={area} value={duties} placeholder={t('co.oneALine')} onChange={(e) => setDuties(e.target.value)} /></Field>
        <Field label={t('co.deliverables')}><textarea className={area} value={deliverables} placeholder={t('co.oneALine')} onChange={(e) => setDeliverables(e.target.value)} /></Field>
        <Field label={t('co.accountKinds')}><input className={field} value={kinds} placeholder={t('co.accountKindsHint')} onChange={(e) => setKinds(e.target.value)} /></Field>
        <fieldset className="space-y-2">
          <legend className="mb-1 text-[13px] font-medium text-ink-2">{t('co.roleTools')}</legend>
          <CapabilityPicker value={r.capabilities ?? []} onChange={(capabilities) => setR({ ...r, capabilities })} />
        </fieldset>
        <ModelChoice value={models} onChange={setModels} legend={t('co.models')} />
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        {remove.error && <p className="text-[13px] text-danger">{remove.error.message}</p>}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!r.title.trim() || models?.length === 0 || save.isPending}>{t('common.save')}</Button>
          {start.id && <Button type="button" variant="ghost" className={danger} onClick={() => remove.mutate()}>{t('co.deleteRole')}</Button>}
        </div>
      </form>
    </Modal>
  )
}

function DepartmentDialog({ org, start, onClose, onSaved }: { org: Org; start: Department; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [d, setD] = useState(start)
  const save = useMutation({
    mutationFn: () => api.saveDepartment(org.id, { ...d, id: d.id || unique(slug(d.name, 'area'), org.departments.map((x) => x.id)) }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteDepartment(org.id, d.id), onSuccess: (o) => { onSaved(o); onClose() } })
  return (
    <Modal title={start.id ? d.name : t('co.newDepartment')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.departmentName')}><input className={field} value={d.name} maxLength={60} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
        <fieldset>
          <legend className="mb-2 text-[12.5px] text-ink-2">{t('co.color')}</legend>
          <div className="flex gap-2">
            {colors.map((c) => (
              <button key={c} type="button" aria-label={c} aria-pressed={d.color === c} onClick={() => setD({ ...d, color: c })}
                className="size-7 rounded-full border-2 aria-pressed:border-ink" style={{ background: `var(--color-${c})`, borderColor: d.color === c ? undefined : 'transparent' }} />
            ))}
          </div>
        </fieldset>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!d.name.trim() || save.isPending}>{t('common.save')}</Button>
          {start.id && <Button type="button" variant="ghost" className={danger} onClick={() => remove.mutate()}>{t('co.deleteDepartment')}</Button>}
        </div>
      </form>
    </Modal>
  )
}

function CompanyDialog({ org, onClose, onSaved }: { org: Org; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [c, setC] = useState({ name: org.name, industry: org.industry ?? '', mission: org.mission ?? '' })
  const save = useMutation({
    mutationFn: () => api.saveCompany(org.id, { ...c, zone: org.zone, partners: org.partners }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  return (
    <Modal title={t('co.edit')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.name')}><input className={field} value={c.name} maxLength={60} onChange={(e) => setC({ ...c, name: e.target.value })} /></Field>
        <Field label={t('co.industry')}><input className={field} value={c.industry} maxLength={120} onChange={(e) => setC({ ...c, industry: e.target.value })} /></Field>
        <Field label={t('co.mission')}><textarea className={area} value={c.mission} maxLength={2000} onChange={(e) => setC({ ...c, mission: e.target.value })} /></Field>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" disabled={!c.name.trim() || save.isPending}>{t('common.save')}</Button>
      </form>
    </Modal>
  )
}
