import { useMutation, useQuery } from '@tanstack/react-query'
import { ChevronRight, MessageSquare, User } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Field, Modal } from '../components/Modal'
import { ModelChoice } from '../components/ModelChoice'
import { Button, Switch } from '../components/ui'
import { api, type Member, type Org } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { below, deptColor, slug, unique } from '../lib/org'
import { area, field } from './Companies'
import { MemberAccounts } from './CompanyAccounts'
import { CoderChoice, codes } from './CompanyCode'
import { AutonomyEditor } from './CompanyDecide'
import { MemberPreview } from './CompanyLayers'
import { MemberWork } from './CompanyWork'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 border-t border-line pt-5" aria-label={title}>
      <h3 className="text-[13.5px] font-semibold">{title}</h3>
      {children}
    </section>
  )
}

// More opens a part that loads only when asked for.
function More({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details className="group rounded-xl border border-line">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3.5 py-3 text-[13.5px] font-medium hover:bg-sunken/50 [&::-webkit-details-marker]:hidden">
        <ChevronRight size={15} className="shrink-0 text-ink-3 transition group-open:rotate-90" aria-hidden />
        {title}
      </summary>
      <div className="border-t border-line p-3.5">{children}</div>
    </details>
  )
}

// MemberDialog is one member of the company: who it is at the top, what it
// does in sections, Save always in view, and removing it apart at the end.
export function MemberDialog({ org, start, can, onClose, onSaved, onTalk }: { org: Org; start: Member; can: boolean; onClose: () => void; onSaved: (o: Org) => void; onTalk: (m: Member) => void }) {
  const t = useT()
  const [m, setM] = useState(start)
  const [models, setModels] = useState<string[] | null>(start.models?.length ? start.models : null)
  const isNew = start.id === ''
  const seat = m.kind === 'person'
  const save = useMutation({
    mutationFn: () => api.saveMember(org.id, { ...m, id: m.id || unique(slug(m.name, 'membro'), org.members.map((x) => x.id)), models: models ?? undefined, autonomy: m.autonomy?.length ? m.autonomy : undefined }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteMember(org.id, m.id), onSuccess: (o) => { onSaved(o); onClose() } })
  const costs = useQuery({ queryKey: ['company-costs', org.id], queryFn: () => api.companyCosts(org.id), enabled: isNew })
  const estimate = m.role ? costs.data?.per_role[m.role] : undefined
  const under = below(org, m.id)
  const bosses = org.members.filter((x) => x.id !== m.id && !under.has(x.id))
  const role = org.roles.find((r) => r.id === m.role)
  const dept = org.departments.find((d) => d.id === m.department)
  const paused = m.state === 'paused'
  const agent = !isNew && !seat
  const footer = can && (<>
    <Button type="submit" form="member-form" variant="primary" disabled={!m.name.trim() || (!seat && !m.role) || models?.length === 0 || save.isPending}>{t('common.save')}</Button>
    {agent && <Button type="button" onClick={() => onTalk(m)}><MessageSquare size={14} /> {t('co.talk')}</Button>}
    {(save.error || remove.error) && <p className="min-w-0 flex-1 text-[13px] text-danger">{(save.error ?? remove.error)?.message}</p>}
  </>)
  return (
    <Modal title={isNew ? t('co.hire') : start.name} onClose={onClose} footer={footer || undefined}>
      <form id="member-form" className="space-y-5" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <fieldset disabled={!can} className="space-y-5">
          <div className="flex items-start gap-4">
            {seat
              ? <span className="grid size-14 shrink-0 place-items-center rounded-2xl bg-ink text-bg" aria-hidden><User size={22} /></span>
              : (
                <label className="relative block size-14 shrink-0" title={t('co.avatarHint')}>
                  <span className="sr-only">{t('co.avatar')}</span>
                  <input className="size-14 rounded-2xl border border-transparent bg-sunken text-center text-[24px] outline-none placeholder:text-ink-3 hover:border-line-strong focus:border-accent"
                    value={m.avatar ?? ''} maxLength={4} placeholder={(m.name.trim()[0] ?? '?').toUpperCase()} onChange={(e) => setM({ ...m, avatar: e.target.value.trim() || undefined })} />
                </label>
              )}
            <div className="min-w-0 flex-1 space-y-1.5">
              <Field label={t('co.memberName')}><input className={field} value={m.name} maxLength={60} onChange={(e) => setM({ ...m, name: e.target.value })} /></Field>
              {!seat && (
                <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[12.5px] text-ink-3">
                  {role && <span>{role.title}</span>}
                  {dept && <span className="inline-flex items-center gap-1"><span className="size-2 rounded-full" style={{ background: deptColor(org, dept.id) }} aria-hidden />{dept.name}</span>}
                  {!isNew && <span className={cn('rounded-full px-2 py-0.5 text-[11.5px] font-medium', paused ? 'bg-change-soft text-change' : 'bg-read-soft text-read')}>{paused ? t('co.paused') : t('co.working')}</span>}
                </p>
              )}
            </div>
          </div>
          {!seat && (
            <div className="flex items-center justify-between gap-4 rounded-xl bg-sunken/60 px-3.5 py-3">
              <div className="min-w-0">
                <p className="text-[13.5px] font-medium">{t('co.working')}</p>
                <p className="text-[12.5px] text-ink-3">{t('co.workingHint')}</p>
              </div>
              <Switch on={!paused} onChange={(on) => setM({ ...m, state: on ? 'active' : 'paused' })} label={t('co.working')} />
            </div>
          )}
          {seat ? (
            <Field label={t('co.seatTitle')}><input className={field} value={m.title ?? ''} maxLength={60} onChange={(e) => setM({ ...m, title: e.target.value })} /></Field>
          ) : (<>
            <Section title={t('co.section.profile')}>
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
              {isNew && estimate !== undefined && estimate > 0 && <p className="text-[12.5px] text-ink-3">{t('co.hireEstimate', { usd: `$${estimate.toFixed(2)}` })}</p>}
              <Field label={t('co.reportsTo')}>
                <select className={field} value={m.reports_to ?? 'ceo'} onChange={(e) => setM({ ...m, reports_to: e.target.value })}>
                  {bosses.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label={t('co.persona')}><textarea className={area} value={m.persona ?? ''} maxLength={2000} placeholder={t('co.personaHint')} onChange={(e) => setM({ ...m, persona: e.target.value })} /></Field>
            </Section>
            <Section title={t('co.section.models')}>
              <ModelChoice value={models} onChange={setModels} legend={t('co.models')} />
              {codes(org, m) && <CoderChoice org={org} m={m} onChange={setM} />}
              <AutonomyEditor org={org} value={m.autonomy ?? []} onChange={(autonomy) => setM({ ...m, autonomy })}
                capabilities={m.capabilities?.length ? m.capabilities : role?.capabilities ?? []} />
            </Section>
          </>)}
        </fieldset>
      </form>
      {agent && (
        <div className="mt-5 space-y-2 border-t border-line pt-5">
          {can && <More title={t('co.ownAccounts')}><MemberAccounts org={org} member={m.id} /></More>}
          {can && <More title={t('co.workAndRoutines')}><MemberWork org={org} member={m.id} onSaved={onSaved} /></More>}
          <More title={t('co.receives')}><MemberPreview org={org} member={m.id} /></More>
        </div>
      )}
      {agent && can && (
        <div className="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-danger/30 px-3.5 py-3">
          <p className="min-w-0 flex-1 basis-56 text-[12.5px] text-ink-2">{t('co.letGoHint')}</p>
          <Button type="button" variant="ghost" className="border border-danger/40 text-danger hover:bg-danger-soft hover:text-danger" disabled={remove.isPending}
            onClick={() => window.confirm(t('co.letGoAsk', { name: m.name })) && remove.mutate()}>{t('co.letGo')}</Button>
        </div>
      )}
    </Modal>
  )
}
