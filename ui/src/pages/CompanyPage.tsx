import * as Menu from '@radix-ui/react-dropdown-menu'
import * as Tabs from '@radix-ui/react-tabs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Briefcase, CirclePause, Download, MoreHorizontal, Pencil, Play, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { CapabilityPicker } from '../components/CapabilityPicker'
import { Field, Modal } from '../components/Modal'
import { ModelChoice } from '../components/ModelChoice'
import { OrgChart } from '../components/OrgChart'
import { Button, Card, EmptyState, PageSkeleton, Switch } from '../components/ui'
import { api, type CompanyRole, type Department, type Member, type Org } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { deptColor, slug, unique } from '../lib/org'
import { area, field, SectionHead } from './Companies'
import { LayersTab } from './CompanyLayers'
import { HoursEditor, WorkLog } from './CompanyWork'
import { TaskBoard } from './CompanyTasks'
import { Digest, MemoryTab } from './CompanyMemory'
import { AutonomyEditor, LevelsTab } from './CompanyDecide'
import { CostsTab } from './CompanyCosts'
import { SharedAccounts } from './CompanyAccounts'
import { MeetingsTab, NewMeeting } from './CompanyMeetings'
import { hasProduct, ProductTab } from './CompanyProduct'
import { hasMedia, MediaTab } from './CompanyMedia'
import { Performance } from './CompanyPerformance'
import { ShowcaseTab } from './CompanyShowcase'
import { CodeEnvField, envOf, envText } from './CompanyCode'
import { MemberDialog } from './CompanyMember'

// Deleting reads in the danger color on a plain button, which keeps its
// contrast in both themes.
const danger = 'text-danger hover:bg-danger-soft hover:text-danger'

const colors = ['chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5']

// A company's page has five parts, each with a few sections; a section is
// what ?tab= names, as it did when every section was a tab of its own.
type Part = 'team' | 'work' | 'knowledge' | 'governance' | 'output'
const parts: { id: Part; sections: string[] }[] = [
  { id: 'team', sections: ['chart', 'roles'] },
  { id: 'work', sections: ['tasks', 'meetings', 'work'] },
  { id: 'knowledge', sections: ['layers', 'memory'] },
  { id: 'governance', sections: ['levels', 'costs'] },
  { id: 'output', sections: ['product', 'media', 'showcase'] },
]

// Other names a link may use for a section, and parts by name.
const aliases: Record<string, string> = {
  team: 'chart', org: 'chart', activity: 'work', knowledge: 'layers', context: 'layers', rules: 'layers',
  governance: 'levels', decisions: 'levels', budget: 'costs', output: 'product',
}

export function sectionOf(tab: string | null, shown: string[]) {
  const s = tab ? (aliases[tab] && !shown.includes(tab) ? aliases[tab] : tab) : 'chart'
  return shown.includes(s) ? s : 'chart'
}

const usd = (n?: number) => `$${(n ?? 0).toFixed(2)}`

const triggerCls = '-mb-px whitespace-nowrap border-b-2 border-transparent px-3 py-2.5 text-[13.5px] text-ink-3 hover:text-ink data-[state=active]:border-ink data-[state=active]:font-medium data-[state=active]:text-ink'
const segmentCls = 'whitespace-nowrap rounded-lg px-3 py-1.5 text-[13px] text-ink-2 hover:text-ink data-[state=active]:bg-surface data-[state=active]:font-medium data-[state=active]:text-ink data-[state=active]:shadow-[var(--shadow-card)]'
const menuItem = 'flex cursor-default items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] outline-none data-[highlighted]:bg-sunken'

export function CompanyPage({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const org = useQuery({ queryKey: ['company', id], queryFn: () => api.company(id) })
  const me = useQuery({ queryKey: ['state'], queryFn: api.state })
  const costs = useQuery({ queryKey: ['company-costs', id], queryFn: () => api.companyCosts(id), enabled: !!org.data })
  const [member, setMember] = useState<Member | null>(null)
  const [role, setRole] = useState<CompanyRole | null>(null)
  const [dept, setDept] = useState<Department | null>(null)
  const [editing, setEditing] = useState(false)
  const [params, setParams] = useSearchParams()
  const [tab, setTabState] = useState(params.get('tab') ?? 'chart')
  const [room, setRoom] = useState<string | null>(null)
  const [talk, setTalk] = useState<{ with?: string[]; question?: string; title: string } | null>(null)
  const setTab = (s: string) => {
    setTabState(s)
    setParams((p) => { const n = new URLSearchParams(p); if (s === 'chart') n.delete('tab'); else n.set('tab', s); return n }, { replace: true })
  }
  const openRoom = (id: string | null) => { setRoom(id); if (id) setTab('meetings') }
  const done = (o: Org) => qc.setQueryData(['company', id], o)
  const move = useMutation({
    mutationFn: ({ m, boss }: { m: Member; boss: string }) => api.saveMember(id, { ...m, reports_to: boss }),
    onSuccess: done,
  })
  const pause = useMutation({ mutationFn: (paused: boolean) => api.saveCompany(id, { ...org.data, paused }), onSuccess: done })
  const remove = useMutation({
    mutationFn: () => api.deleteCompany(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['companies'] }); nav('/companies') },
  })
  if (org.isLoading) return <PageSkeleton />
  if (org.error || !org.data) return <p className="text-sm text-danger">{org.error?.message}</p>
  const o = org.data
  const can = o.grant === 'configure'
  const mine = o.person === me.data?.person
  const newMember = (): Member => ({ id: '', kind: 'agent', name: '', role: o.roles[0]?.id, reports_to: 'ceo', state: 'active' })
  const optional: Record<string, boolean> = { product: hasProduct(o), media: hasMedia(o), showcase: mine }
  const shown = parts.map((p) => ({ ...p, sections: p.sections.filter((s) => optional[s] ?? true) })).filter((p) => p.sections.length > 0)
  const section = sectionOf(tab, shown.flatMap((p) => p.sections))
  const part = shown.find((p) => p.sections.includes(section))!
  const label = (s: string) => s === 'roles' ? t('co.tab.roles', { n: o.roles.length }) : t(`co.tab.${s}` as 'co.tab.chart')
  const spent = costs.data?.company?.month
  const limit = o.budget?.month_usd
  return (
    <div className="mx-auto max-w-6xl">
      <Link to="/companies" className="mb-3 inline-flex items-center gap-1 text-[13px] text-ink-3 hover:text-ink"><ArrowLeft size={14} /> {t('co.title')}</Link>
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 flex-1 basis-72">
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
            <h1 className="min-w-0 truncate text-[22px] font-semibold tracking-tight">{o.name}</h1>
            <span className={cn('inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-[12px] font-medium', o.paused ? 'bg-change-soft text-change' : 'bg-read-soft text-read')}>
              <span className={cn('size-1.5 rounded-full', o.paused ? 'bg-change' : 'bg-read')} aria-hidden />{o.paused ? t('co.state.paused') : t('co.state.active')}
            </span>
          </div>
          <p className="mt-0.5 flex flex-wrap gap-x-2 text-[13px] text-ink-2">
            {o.industry && <span>{o.industry}</span>}
            {o.industry && spent !== undefined && <span className="text-ink-3" aria-hidden>·</span>}
            {spent !== undefined && <span className="tabular-nums">{limit ? t('co.budgetGlance', { spent: usd(spent), limit: usd(limit) }) : t('co.spentMonth', { spent: usd(spent) })}</span>}
          </p>
          {o.mission && <p className="mt-1 line-clamp-2 max-w-2xl text-[13px] text-ink-3">{o.mission}</p>}
          {o.paused && <p className="mt-2 inline-flex items-center gap-1.5 text-[12.5px] text-change"><CirclePause size={13} /> {t('co.pausedNote')}</p>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {can && <Button onClick={() => setEditing(true)} aria-label={t('co.edit')}><Pencil size={15} /> <span className="hidden sm:inline">{t('co.edit')}</span></Button>}
          {can && (o.paused
            ? <Button onClick={() => pause.mutate(false)} disabled={pause.isPending}><Play size={15} /> {t('co.resume')}</Button>
            : <Button onClick={() => pause.mutate(true)} disabled={pause.isPending}><CirclePause size={15} /> {t('co.pause')}</Button>)}
          <Menu.Root>
            <Menu.Trigger className="grid size-9 place-items-center rounded-[10px] border border-line bg-surface text-ink-2 shadow-[var(--shadow-card)] hover:border-line-strong hover:text-ink data-[state=open]:border-line-strong" aria-label={t('co.more')}>
              <MoreHorizontal size={16} />
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Content align="end" sideOffset={6} className="z-50 min-w-48 rounded-xl border border-line bg-surface p-1 shadow-[var(--shadow-pop)]">
                <Menu.Item asChild className={menuItem}>
                  <a href={`/api/companies/${id}/export`} download><Download size={14} /> {t('co.export')}</a>
                </Menu.Item>
                {mine && (<>
                  <Menu.Separator className="my-1 h-px bg-line" />
                  <Menu.Item className={cn(menuItem, 'text-danger data-[highlighted]:bg-danger-soft')} onSelect={() => window.confirm(t('co.deleteAsk', { name: o.name })) && remove.mutate()}>
                    <Trash2 size={14} /> {t('co.delete')}
                  </Menu.Item>
                </>)}
              </Menu.Content>
            </Menu.Portal>
          </Menu.Root>
        </div>
        {(pause.error || remove.error) && <p className="w-full text-[13px] text-danger">{(pause.error ?? remove.error)?.message}</p>}
      </header>

      <label className="mb-5 block md:hidden">
        <span className="sr-only">{t('co.section')}</span>
        <select className={field} value={section} onChange={(e) => setTab(e.target.value)}>
          {shown.map((p) => (
            <optgroup key={p.id} label={t(`co.group.${p.id}`)}>
              {p.sections.map((s) => <option key={s} value={s}>{label(s)}</option>)}
            </optgroup>
          ))}
        </select>
      </label>

      <Tabs.Root value={part.id} onValueChange={(v) => setTab(shown.find((p) => p.id === v)!.sections[0])}>
        <Tabs.List className="mb-4 hidden gap-1 border-b border-line md:flex" aria-label={t('co.sections')}>
          {shown.map((p) => <Tabs.Trigger key={p.id} value={p.id} className={triggerCls}>{t(`co.group.${p.id}`)}</Tabs.Trigger>)}
        </Tabs.List>
        {shown.map((p) => (
          <Tabs.Content key={p.id} value={p.id}>
            <Tabs.Root value={section} onValueChange={setTab}>
              {p.sections.length > 1 && (
                <Tabs.List className="mb-5 hidden w-fit gap-1 rounded-xl bg-sunken p-1 md:flex" aria-label={t(`co.group.${p.id}`)}>
                  {p.sections.map((s) => <Tabs.Trigger key={s} value={s} className={segmentCls}>{label(s)}</Tabs.Trigger>)}
                </Tabs.List>
              )}
              <Tabs.Content value="chart" className="space-y-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    {o.departments.map((d) => (
                      <button key={d.id} type="button" disabled={!can} onClick={() => setDept(d)} className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-line px-2.5 py-1 text-[12.5px] hover:border-line-strong">
                        <span className="size-2 shrink-0 rounded-full" style={{ background: deptColor(o, d.id) }} aria-hidden /> <span className="truncate">{d.name}</span>
                      </button>
                    ))}
                    {can && <Button size="sm" variant="ghost" onClick={() => setDept({ id: '', name: '', color: colors[o.departments.length % colors.length] })}><Plus size={14} /> {t('co.newDepartment')}</Button>}
                  </div>
                  {can && (o.roles.length > 0
                    ? <Button variant="primary" onClick={() => setMember(newMember())}><Plus size={15} /> {t('co.hire')}</Button>
                    : (
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-[12.5px] text-ink-3">{t('co.roleFirst')}</p>
                        <Button variant="primary" onClick={() => { setTab('roles'); setRole({ id: '', title: '' }) }}><Plus size={15} /> {t('co.newRole')}</Button>
                      </div>
                    ))}
                </div>
                {can && o.members.length > 1 && <p className="hidden text-[12.5px] text-ink-3 md:block">{t('co.dragHint')}</p>}
                <OrgChart org={o} onOpen={setMember} onMove={can ? (m, boss) => { const x = o.members.find((y) => y.id === m); if (x) move.mutate({ m: x, boss }) } : undefined} />
                {move.error && <p className="text-[13px] text-danger">{move.error.message}</p>}
              </Tabs.Content>
              <Tabs.Content value="roles" className="space-y-4">
                <SectionHead title={t('co.roles')} hint={t('co.rolesHint')} action={can && o.roles.length > 0 && <Button variant="primary" onClick={() => setRole({ id: '', title: '' })}><Plus size={15} /> {t('co.newRole')}</Button>} />
                {o.roles.length === 0 && (
                  <EmptyState icon={<Briefcase />} title={t('co.empty.roles')} action={can && <Button variant="primary" onClick={() => setRole({ id: '', title: '' })}><Plus size={15} /> {t('co.newRole')}</Button>}>{t('co.noRoles')}</EmptyState>
                )}
                <div className="grid gap-3 sm:grid-cols-2">
                  {o.roles.map((r) => {
                    const holders = o.members.filter((m) => m.role === r.id)
                    return (
                      <Card key={r.id} className="flex items-start gap-3 p-4">
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-[14.5px] font-medium">{r.title}</div>
                          {r.function && <p className="line-clamp-2 text-[12.5px] text-ink-3">{r.function}</p>}
                          <p className="mt-1 truncate text-[12px] text-ink-2">{holders.length ? holders.map((m) => m.name).join(', ') : t('co.nobodyHolds')}</p>
                        </div>
                        {can && <Button size="sm" variant="ghost" aria-label={t('co.editRole', { name: r.title })} onClick={() => setRole(r)}><Pencil size={14} /></Button>}
                      </Card>
                    )
                  })}
                </div>
              </Tabs.Content>
              <Tabs.Content value="layers"><LayersTab org={o} can={can} onSaved={done} /></Tabs.Content>
              <Tabs.Content value="tasks">
                <TaskBoard org={o} can={can} onDiscuss={(q) => setTalk({ with: [q.from], question: q.id, title: q.text.slice(0, 60) })} />
              </Tabs.Content>
              <Tabs.Content value="meetings"><MeetingsTab org={o} can={can} room={room} onRoom={openRoom} /></Tabs.Content>
              <Tabs.Content value="costs" className="space-y-10">
                <CostsTab org={o} can={can} onSaved={done} />
                <SharedAccounts org={o} can={can} onSaved={done} />
              </Tabs.Content>
              <Tabs.Content value="levels"><LevelsTab org={o} can={can} onSaved={done} /></Tabs.Content>
              <Tabs.Content value="memory"><MemoryTab org={o} can={can} onSaved={done} /></Tabs.Content>
              <Tabs.Content value="showcase"><ShowcaseTab org={o} onSaved={done} /></Tabs.Content>
              <Tabs.Content value="media"><MediaTab org={o} /></Tabs.Content>
              <Tabs.Content value="product"><ProductTab org={o} can={o.grant !== 'view'} /></Tabs.Content>
              <Tabs.Content value="work" className="space-y-4">
                <Digest org={o} />
                <Performance org={o} />
                <WorkLog org={o} can={can} />
              </Tabs.Content>
            </Tabs.Root>
          </Tabs.Content>
        ))}
      </Tabs.Root>

      {member && <MemberDialog org={o} start={member} can={can} onClose={() => setMember(null)} onSaved={done} onTalk={(m) => { setMember(null); setTalk({ with: [m.id], title: m.name }) }} />}
      {talk && <NewMeeting org={o} with={talk.with} question={talk.question} onClose={() => setTalk(null)} onOpen={openRoom} />}
      {role && <RoleDialog org={o} start={role} onClose={() => setRole(null)} onSaved={done} />}
      {dept && <DepartmentDialog org={o} start={dept} onClose={() => setDept(null)} onSaved={done} />}
      {editing && <CompanyDialog org={o} onClose={() => setEditing(false)} onSaved={done} />}
    </div>
  )
}

const lines = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean)

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
      account_kinds: kinds.split(',').map((x) => x.trim()).filter(Boolean), models: models ?? undefined, autonomy: r.autonomy?.length ? r.autonomy : undefined,
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
        <AutonomyEditor org={org} value={r.autonomy ?? []} onChange={(autonomy) => setR({ ...r, autonomy })} capabilities={r.capabilities ?? []} />
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
        <Field label={t('co.monthLimit')}><input type="number" min={0} className={field} value={d.month_usd ?? ''} onChange={(e) => setD({ ...d, month_usd: e.target.value ? Number(e.target.value) : undefined })} /></Field>
        {start.id && <Switch on={!d.paused} onChange={(on) => setD({ ...d, paused: !on })} label={t('co.departmentWorking')} />}
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
  const [c, setC] = useState({ name: org.name, industry: org.industry ?? '', mission: org.mission ?? '', hours: org.hours ?? {} })
  const [env, setEnv] = useState(envText(org.code_env))
  const [disclosure, setDisclosure] = useState(org.disclosure ?? '')
  const save = useMutation({
    mutationFn: () => api.saveCompany(org.id, { ...org, ...c, code_env: envOf(env), disclosure: disclosure.trim() || undefined }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  return (
    <Modal title={t('co.edit')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <Field label={t('co.name')}><input className={field} value={c.name} maxLength={60} onChange={(e) => setC({ ...c, name: e.target.value })} /></Field>
        <Field label={t('co.industry')}><input className={field} value={c.industry} maxLength={120} onChange={(e) => setC({ ...c, industry: e.target.value })} /></Field>
        <Field label={t('co.mission')}><textarea className={area} value={c.mission} maxLength={2000} onChange={(e) => setC({ ...c, mission: e.target.value })} /></Field>
        <HoursEditor value={c.hours} onChange={(hours) => setC({ ...c, hours })} />
        <div>
          <Field label={t('co.disclosure')}><input className={field} value={disclosure} maxLength={280} placeholder="Made with the help of AI." onChange={(e) => setDisclosure(e.target.value)} /></Field>
          <p className="mt-1 text-[12px] text-ink-3">{t('co.disclosureHint')}</p>
        </div>
        <CodeEnvField value={env} onChange={setEnv} />
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" disabled={!c.name.trim() || save.isPending}>{t('common.save')}</Button>
      </form>
    </Modal>
  )
}
