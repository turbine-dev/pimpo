import { useMutation, useQuery } from '@tanstack/react-query'
import { Pencil, Plus } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { CapabilityPicker } from '../components/CapabilityPicker'
import { capabilityLabel } from '../components/RoutineCard'
import { Button, Card } from '../components/ui'
import { api, ApiError, type CompanyContext, type CompanyRule, type Org, type Scope } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { slug, unique } from '../lib/org'
import { area, field } from './Companies'
import { Field, Modal } from '../components/Modal'
import { describeRule, verdictText } from './Rules'

// A scope as one select value: "company", "role:atendente"...
const encode = (scope: Scope, of?: string) => (scope === 'company' ? 'company' : `${scope}:${of}`)
const decode = (v: string): { scope: Scope; of?: string } => {
  const [scope, of] = v.split(':')
  return { scope: scope as Scope, of }
}

function useScopes(org: Org) {
  const t = useT()
  const options: [string, string][] = [['company', t('co.scope.company')]]
  for (const d of org.departments) options.push([encode('department', d.id), t('co.scope.department', { name: d.name })])
  for (const r of org.roles) options.push([encode('role', r.id), t('co.scope.role', { name: r.title })])
  for (const m of org.members.filter((x) => x.kind === 'agent')) options.push([encode('member', m.id), t('co.scope.member', { name: m.name })])
  const name = (scope: Scope, of?: string) => options.find(([v]) => v === encode(scope, of))?.[1] ?? of ?? scope
  return { options, name }
}

const order: Scope[] = ['company', 'department', 'role', 'member']

export function LayersTab({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const scopes = useScopes(org)
  const [context, setContext] = useState<CompanyContext | null>(null)
  const [rule, setRule] = useState<CompanyRule | null>(null)
  const byLayer = <T extends { scope: Scope }>(list: T[]) => [...list].sort((a, b) => order.indexOf(a.scope) - order.indexOf(b.scope))
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-[15px] font-semibold">{t('co.contexts')}</h2>
          {can && <Button size="sm" onClick={() => setContext({ id: '', scope: 'company', title: '', body: '' })}><Plus size={14} /> {t('co.newContext')}</Button>}
        </div>
        <p className="text-[12.5px] text-ink-3">{t('co.contextsHint')}</p>
        {byLayer(org.contexts).map((c) => (
          <Card key={c.id} className="flex items-start gap-3 p-4">
            <div className="min-w-0 flex-1">
              <div className="text-[12px] text-ink-3">{scopes.name(c.scope, c.of)}</div>
              <div className="text-[14px] font-medium">{c.title}</div>
              <p className="line-clamp-2 whitespace-pre-line text-[12.5px] text-ink-2">{c.body}</p>
              {(c.version ?? 1) > 1 && <p className="mt-1 text-[11.5px] text-ink-3">{t('co.version', { n: c.version ?? 1 })}</p>}
            </div>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.editContext', { name: c.title })} onClick={() => setContext(c)}><Pencil size={14} /></Button>}
          </Card>
        ))}
      </section>
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-[15px] font-semibold">{t('co.rules')}</h2>
          {can && <Button size="sm" onClick={() => setRule({ id: '', scope: 'company', text: '', when: {}, then: 'ask' })}><Plus size={14} /> {t('co.newRule')}</Button>}
        </div>
        <p className="text-[12.5px] text-ink-3">{t('co.rulesHint')}</p>
        {byLayer(org.rules).map((r) => (
          <Card key={r.id} className={cn('flex items-start gap-3 p-4', r.off && 'opacity-60')}>
            <div className="min-w-0 flex-1">
              <div className="text-[12px] text-ink-3">{scopes.name(r.scope, r.of)}</div>
              <div className="text-[14px]">{r.text}</div>
              <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[12px]">
                <span className={cn('rounded-md px-1.5 py-0.5 font-medium', verdictText[r.then].cls)}>{t(verdictText[r.then].label)}</span>
                <span className="text-ink-3">{describeRule({ ...r, when: r.when })}</span>
              </div>
              {r.exception && <p className="mt-1 text-[12px] text-change">{t('co.exceptionOf', { list: (r.overrides ?? []).map((id) => org.rules.find((x) => x.id === id)?.text ?? id).join('; ') })}</p>}
            </div>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.editRule', { name: r.text })} onClick={() => setRule(r)}><Pencil size={14} /></Button>}
          </Card>
        ))}
      </section>
      {context && <ContextDialog org={org} start={context} scopes={scopes.options} onClose={() => setContext(null)} onSaved={onSaved} />}
      {rule && <RuleDialog org={org} start={rule} scopes={scopes.options} onClose={() => setRule(null)} onSaved={onSaved} />}
    </div>
  )
}

function ScopeSelect({ value, options, onChange }: { value: string; options: [string, string][]; onChange: (v: string) => void }) {
  const t = useT()
  return (
    <Field label={t('co.appliesTo')}>
      <select className={field} value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    </Field>
  )
}

function ContextDialog({ org, start, scopes, onClose, onSaved }: { org: Org; start: CompanyContext; scopes: [string, string][]; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [c, setC] = useState(start)
  const save = useMutation({
    mutationFn: () => api.saveContext(org.id, { ...c, id: c.id || unique(slug(c.title, 'contexto'), org.contexts.map((x) => x.id)) }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteContext(org.id, c.id), onSuccess: (o) => { onSaved(o); onClose() } })
  return (
    <Modal title={start.id ? start.title : t('co.newContext')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <ScopeSelect value={encode(c.scope, c.of)} options={scopes} onChange={(v) => setC({ ...c, ...decode(v) })} />
        <Field label={t('co.contextTitle')}><input className={field} value={c.title} maxLength={80} onChange={(e) => setC({ ...c, title: e.target.value })} /></Field>
        <Field label={t('co.contextBody')}><textarea className={area + ' min-h-48 font-mono text-[13px]'} value={c.body} placeholder={t('co.contextBodyHint')} onChange={(e) => setC({ ...c, body: e.target.value })} /></Field>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!c.title.trim() || save.isPending}>{t('common.save')}</Button>
          {start.id && <Button type="button" variant="ghost" className="text-danger hover:bg-danger-soft hover:text-danger" onClick={() => remove.mutate()}>{t('co.deleteContext')}</Button>}
        </div>
      </form>
    </Modal>
  )
}

const verdicts = ['allow', 'reversible', 'ask', 'block'] as const
const risks = ['', 'read', 'notify', 'reversible', 'irreversible'] as const

function RuleDialog({ org, start, scopes, onClose, onSaved }: { org: Org; start: CompanyRule; scopes: [string, string][]; onClose: () => void; onSaved: (o: Org) => void }) {
  const t = useT()
  const [r, setR] = useState(start)
  const save = useMutation({
    mutationFn: (exception: boolean) => api.saveCompanyRule(org.id, { ...r, exception, id: r.id || unique(slug(r.text, 'regra'), org.rules.map((x) => x.id)) }),
    onSuccess: (o) => { onSaved(o); onClose() },
  })
  const remove = useMutation({ mutationFn: () => api.deleteCompanyRule(org.id, r.id), onSuccess: (o) => { onSaved(o); onClose() } })
  const conflict = save.error instanceof ApiError && save.error.status === 409
  return (
    <Modal title={start.id ? t('co.editRule', { name: start.text }) : t('co.newRule')} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(!!r.exception) }}>
        <ScopeSelect value={encode(r.scope, r.of)} options={scopes} onChange={(v) => setR({ ...r, ...decode(v) })} />
        <Field label={t('co.ruleText')}><input className={field} value={r.text} maxLength={500} placeholder={t('co.ruleTextHint')} onChange={(e) => setR({ ...r, text: e.target.value })} /></Field>
        <Field label={t('co.ruleThen')}>
          <select className={field} value={r.then} onChange={(e) => setR({ ...r, then: e.target.value as CompanyRule['then'] })}>
            {verdicts.map((v) => <option key={v} value={v}>{t(verdictText[v].label)}</option>)}
          </select>
        </Field>
        <Field label={t('co.ruleRisk')}>
          <select className={field} value={r.when.min_risk ?? ''} onChange={(e) => setR({ ...r, when: { ...r.when, min_risk: e.target.value || undefined } })}>
            {risks.map((v) => <option key={v} value={v}>{v ? t('rules.orMore', { what: t(`rules.risk.${v}`) }) : t('co.anyRisk')}</option>)}
          </select>
        </Field>
        <fieldset className="space-y-2">
          <legend className="mb-1 text-[13px] font-medium text-ink-2">{t('co.ruleCapabilities')}</legend>
          <CapabilityPicker value={r.when.capabilities ?? []} onChange={(capabilities) => setR({ ...r, when: { ...r.when, capabilities: capabilities.length ? capabilities : undefined } })} />
        </fieldset>
        {conflict ? (
          <div className="rounded-xl border border-change/40 bg-change-soft p-3 text-[13px] text-change">
            <p className="mb-2">{t('co.exceptionAsk')}</p>
            <Button type="button" onClick={() => save.mutate(true)} disabled={save.isPending}>{t('co.saveException')}</Button>
          </div>
        ) : save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!r.text.trim() || save.isPending}>{t('common.save')}</Button>
          {start.id && <Button type="button" variant="ghost" className="text-danger hover:bg-danger-soft hover:text-danger" onClick={() => remove.mutate()}>{t('co.deleteRule')}</Button>}
        </div>
      </form>
    </Modal>
  )
}

// MemberPreview is what a member receives: its brief and what each of its
// tools would do, with the rule that decides.
export function MemberPreview({ org, member }: { org: Org; member: string }): ReactNode {
  const t = useT()
  const preview = useQuery({ queryKey: ['company', org.id, 'preview', member, org.updated], queryFn: () => api.memberPreview(org.id, member) })
  if (!preview.data) return null
  return (
    <div className="space-y-3">
      <pre className="max-h-64 overflow-y-auto whitespace-pre-wrap rounded-xl border border-line bg-sunken p-3 font-sans text-[12.5px] text-ink-2">{preview.data.brief}</pre>
      {preview.data.rules.length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noTools')}</p>}
      <ul className="space-y-1">
        {preview.data.rules.map((r) => (
          <li key={r.capability} className="flex items-center gap-2 text-[12.5px]">
            <span className="min-w-0 flex-1 truncate">{capabilityLabel(r.capability)}</span>
            <span className={cn('rounded-md px-1.5 py-0.5 font-medium', verdictText[r.verdict].cls)} title={r.reason}>{t(verdictText[r.verdict].label)}</span>
            {r.decider && <span className="text-[11.5px] text-ink-3">{t('co.decidedBy', { who: r.decider })}</span>}
          </li>
        ))}
      </ul>
    </div>
  )
}
