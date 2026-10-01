import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { capabilityLabel } from '../components/RoutineCard'
import { Field } from '../components/Modal'
import { Button, Card } from '../components/ui'
import { api, type Autonomy, type Decider, type DeciderKind, type Level, type Levels, type Org } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { field } from './Companies'

const kinds: DeciderKind[] = ['person', 'self', 'jev', 'model', 'boss', 'committee', 'cascade']
const stepKinds: DeciderKind[] = ['jev', 'model', 'boss', 'committee', 'self', 'person']
const risks = ['reversible', 'irreversible', 'notify', 'read']

// DeciderEditor chooses who decides; a cascade's steps are chosen the
// same way, one after another.
export function DeciderEditor({ org, value, onChange, step }: { org: Org; value: Decider; onChange: (d: Decider) => void; step?: boolean }) {
  const t = useT()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const set = (kind: DeciderKind) => {
    const d: Decider = { kind }
    if (kind === 'jev') d.threshold = 0.9
    if (kind === 'committee') d.members = agents.slice(0, 2).map((a) => a.id)
    if (kind === 'cascade') d.steps = [{ kind: 'jev', threshold: 0.9 }, { kind: 'person' }]
    onChange(d)
  }
  return (
    <div className="space-y-2">
      <select className={field} aria-label={t('co.decider')} value={value.kind} onChange={(e) => set(e.target.value as DeciderKind)}>
        {(step ? stepKinds : kinds).map((k) => <option key={k} value={k}>{t(`co.decider.${k}` as 'co.decider.self')}</option>)}
      </select>
      {value.kind === 'jev' && (
        <label className="flex items-center gap-2 text-[12.5px] text-ink-2">{t('co.threshold')}
          <input type="number" min={0.5} max={0.99} step={0.01} className={field + ' h-8 w-24'} value={value.threshold ?? 0.9} onChange={(e) => onChange({ ...value, threshold: Number(e.target.value) })} />
        </label>
      )}
      {value.kind === 'committee' && (
        <div className="flex flex-wrap gap-3">
          {agents.map((a) => (
            <label key={a.id} className="flex items-center gap-1.5 text-[12.5px]">
              <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={!!value.members?.includes(a.id)}
                onChange={(e) => onChange({ ...value, members: e.target.checked ? [...(value.members ?? []), a.id] : (value.members ?? []).filter((x) => x !== a.id) })} /> {a.name}
            </label>
          ))}
          <label className="flex items-center gap-1.5 text-[12.5px]">
            <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={!!value.unanimous} onChange={(e) => onChange({ ...value, unanimous: e.target.checked })} /> {t('co.unanimous')}
          </label>
        </div>
      )}
      {value.kind === 'cascade' && (
        <ol className="space-y-2 border-l-2 border-line pl-3">
          {(value.steps ?? []).map((s, i) => (
            <li key={i} className="flex items-start gap-2">
              <div className="flex-1"><DeciderEditor org={org} step value={s} onChange={(d) => onChange({ ...value, steps: (value.steps ?? []).map((x, j) => (j === i ? d : x)) })} /></div>
              {(value.steps ?? []).length > 2 && <Button type="button" size="sm" variant="ghost" aria-label={t('co.removeStep')} onClick={() => onChange({ ...value, steps: (value.steps ?? []).filter((_, j) => j !== i) })}><Trash2 size={13} /></Button>}
            </li>
          ))}
          <Button type="button" size="sm" variant="ghost" onClick={() => onChange({ ...value, steps: [...(value.steps ?? []), { kind: 'person' }] })}><Plus size={13} /> {t('co.addStep')}</Button>
        </ol>
      )}
    </div>
  )
}

// AutonomyEditor is a member's or a role's matrix: for a capability or a
// risk, who decides when it would ask first.
export function AutonomyEditor({ org, value, onChange, capabilities }: { org: Org; value: Autonomy[]; onChange: (a: Autonomy[]) => void; capabilities: string[] }) {
  const t = useT()
  const put = (i: number, a: Autonomy) => onChange(value.map((x, j) => (j === i ? a : x)))
  return (
    <fieldset className="space-y-2">
      <legend className="mb-1 text-[13px] font-medium text-ink-2">{t('co.autonomy')}</legend>
      <p className="text-[12px] text-ink-3">{t('co.autonomyHint')}</p>
      {value.map((a, i) => (
        <div key={i} className="space-y-2 rounded-xl border border-line p-3">
          <div className="flex gap-2">
            <select className={field} aria-label={t('co.autonomyFor')} value={a.capability ? `cap:${a.capability}` : `risk:${a.min_risk ?? 'irreversible'}`}
              onChange={(e) => {
                const [k, v] = e.target.value.split(':')
                put(i, k === 'cap' ? { capability: v, decider: a.decider } : { min_risk: v, decider: a.decider })
              }}>
              {risks.map((r) => <option key={r} value={`risk:${r}`}>{t('rules.orMore', { what: t(`rules.risk.${r}` as 'rules.risk.read') })}</option>)}
              {capabilities.map((c) => <option key={c} value={`cap:${c}`}>{capabilityLabel(c)}</option>)}
            </select>
            <Button type="button" size="sm" variant="ghost" aria-label={t('co.removeLine')} onClick={() => onChange(value.filter((_, j) => j !== i))}><Trash2 size={13} /></Button>
          </div>
          <DeciderEditor org={org} value={a.decider} onChange={(decider) => put(i, { ...a, decider })} />
        </div>
      ))}
      <Button type="button" size="sm" variant="ghost" onClick={() => onChange([...value, { min_risk: 'irreversible', decider: { kind: 'person' } }])}><Plus size={13} /> {t('co.addLine')}</Button>
    </fieldset>
  )
}

const decides = ['self', 'boss', 'head', 'ceo'] as const
const list = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)

// suggested are the RFC's four levels.
function suggested(t: (k: 'co.level.operational' | 'co.level.tactical' | 'co.level.managerial' | 'co.level.strategic') => string): Levels {
  return { unsure: 0.8, list: [
    { level: 1, name: t('co.level.operational'), decides: 'self', when: {} },
    { level: 2, name: t('co.level.tactical'), decides: 'boss', when: { min_risk: 'irreversible' } },
    { level: 3, name: t('co.level.managerial'), decides: 'head', when: { over_usd: 20, kinds: ['roadmap', 'architecture'] } },
    { level: 4, name: t('co.level.strategic'), decides: 'ceo', route: 'opinions', when: { over_usd: 100, public: true, kinds: ['price', 'contract', 'partnership', 'hiring', 'budget', 'legal'] } },
  ] }
}

export function LevelsTab({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [levels, setLevels] = useState<Levels>(org.levels ?? {})
  const [decider, setDecider] = useState<Decider>(org.decider?.kind ? org.decider : { kind: 'person' })
  const decisions = useQuery({ queryKey: ['company-decisions', org.id], queryFn: () => api.companyDecisions(org.id) })
  const save = useMutation({
    mutationFn: () => api.saveCompany(org.id, { ...org, levels, decider: decider.kind === 'person' ? undefined : decider }),
    onSuccess: (o) => { onSaved(o); qc.invalidateQueries({ queryKey: ['company', org.id] }) },
  })
  const put = (i: number, l: Level) => setLevels({ ...levels, list: (levels.list ?? []).map((x, j) => (j === i ? l : x)) })
  const name = (id: string) => org.members.find((m) => m.id === id)?.name ?? id
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.levels')}</h2>
        <p className="text-[12.5px] text-ink-3">{t('co.levelsHint')}</p>
        <fieldset disabled={!can} className="space-y-3">
          {!(levels.list ?? []).length && <Button type="button" onClick={() => setLevels(suggested(t))}>{t('co.useSuggested')}</Button>}
          {(levels.list ?? []).map((l, i) => (
            <Card key={l.level} className="space-y-2 p-3">
              <div className="flex items-center gap-2">
                <span className="grid size-7 shrink-0 place-items-center rounded-full bg-sunken text-[12.5px] font-semibold">{l.level}</span>
                <input className={field} aria-label={t('co.levelName')} value={l.name} onChange={(e) => put(i, { ...l, name: e.target.value })} />
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <Field label={t('co.levelDecides')}>
                  <select className={field} value={l.decides} disabled={i === (levels.list ?? []).length - 1} onChange={(e) => put(i, { ...l, decides: e.target.value as Level['decides'] })}>
                    {decides.map((d) => <option key={d} value={d}>{t(`co.levelBy.${d}` as 'co.levelBy.ceo')}</option>)}
                  </select>
                </Field>
                {l.decides === 'ceo' && (
                  <Field label={t('co.route')}>
                    <select className={field} value={l.route ?? 'direct'} onChange={(e) => put(i, { ...l, route: e.target.value as Level['route'] })}>
                      <option value="direct">{t('co.route.direct')}</option>
                      <option value="opinions">{t('co.route.opinions')}</option>
                    </select>
                  </Field>
                )}
              </div>
              {i > 0 && (
                <div className="grid gap-2 sm:grid-cols-2">
                  <Field label={t('co.overUsd')}><input type="number" min={0} className={field} value={l.when.over_usd ?? ''} onChange={(e) => put(i, { ...l, when: { ...l.when, over_usd: e.target.value ? Number(e.target.value) : undefined } })} /></Field>
                  <Field label={t('co.ruleRisk')}>
                    <select className={field} value={l.when.min_risk ?? ''} onChange={(e) => put(i, { ...l, when: { ...l.when, min_risk: e.target.value || undefined } })}>
                      <option value="">{t('co.anyRisk')}</option>
                      {risks.map((r) => <option key={r} value={r}>{t('rules.orMore', { what: t(`rules.risk.${r}` as 'rules.risk.read') })}</option>)}
                    </select>
                  </Field>
                  <Field label={t('co.kinds')}><input className={field} value={(l.when.kinds ?? []).join(', ')} placeholder={t('co.kindsHint')} onChange={(e) => put(i, { ...l, when: { ...l.when, kinds: list(e.target.value) } })} /></Field>
                  <Field label={t('co.words')}><input className={field} value={(l.when.words ?? []).join(', ')} onChange={(e) => put(i, { ...l, when: { ...l.when, words: list(e.target.value) } })} /></Field>
                  <label className="flex items-center gap-2 text-[12.5px]"><input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={!!l.when.public} onChange={(e) => put(i, { ...l, when: { ...l.when, public: e.target.checked } })} /> {t('co.publicTrigger')}</label>
                </div>
              )}
            </Card>
          ))}
          <div className="space-y-2">
            <span className="text-[13px] font-medium">{t('co.defaultDecider')}</span>
            <DeciderEditor org={org} value={decider} onChange={setDecider} />
          </div>
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
          {can && <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>}
        </fieldset>
      </section>
      <section className="space-y-4">
        <Simulator org={org} />
        <div className="space-y-2">
          <h2 className="text-[15px] font-semibold">{t('co.decisions')}</h2>
          {(decisions.data ?? []).length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noDecisions')}</p>}
          {(decisions.data ?? []).map((d) => (
            <Card key={d.id} className="p-3 text-[12.5px]">
              <p className="text-ink-3">{name(d.member)} · {d.decider}{d.level ? ` · ${t('co.levelN', { n: d.level })}` : ''} · {relative(d.created)}</p>
              <p className="truncate font-mono text-[12px]">{d.question}</p>
              <p><b className="font-medium">{t(`co.answer.${d.answer}` as 'co.answer.allow')}</b>{d.reason ? `: ${d.reason}` : ''}</p>
            </Card>
          ))}
        </div>
      </section>
    </div>
  )
}

function Simulator({ org }: { org: Org }) {
  const t = useT()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const [q, setQ] = useState({ member: agents[0]?.id ?? '', text: '', kind: '', amount_usd: 0, public: false })
  const run = useMutation({ mutationFn: () => api.simulateLevel(org.id, q) })
  return (
    <Card className="space-y-2 p-4">
      <h2 className="text-[15px] font-semibold">{t('co.simulator')}</h2>
      <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); run.mutate() }}>
        <input className={field} aria-label={t('co.simText')} placeholder={t('co.simTextHint')} value={q.text} onChange={(e) => setQ({ ...q, text: e.target.value })} />
        <div className="grid gap-2 sm:grid-cols-3">
          <select className={field} aria-label={t('co.simWho')} value={q.member} onChange={(e) => setQ({ ...q, member: e.target.value })}>{agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}</select>
          <input className={field} aria-label={t('co.simKind')} placeholder={t('co.simKind')} value={q.kind} onChange={(e) => setQ({ ...q, kind: e.target.value })} />
          <input type="number" min={0} className={field} aria-label={t('co.simAmount')} placeholder={t('co.simAmount')} value={q.amount_usd || ''} onChange={(e) => setQ({ ...q, amount_usd: Number(e.target.value) })} />
        </div>
        <label className="flex items-center gap-2 text-[12.5px]"><input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={q.public} onChange={(e) => setQ({ ...q, public: e.target.checked })} /> {t('co.publicTrigger')}</label>
        <Button type="submit" size="sm" disabled={!q.text.trim() || run.isPending}>{t('co.simulate')}</Button>
      </form>
      {run.data && (
        <p className="text-[13px]" role="status">
          {run.data.level ? t('co.simResult', { n: run.data.level, name: run.data.name ?? '', who: run.data.decider ?? '' }) : t('co.simNoLevels')}
          {run.data.why && <span className="text-ink-3"> · {run.data.why}</span>}
        </p>
      )}
      {run.error && <p className="text-[13px] text-danger">{run.error.message}</p>}
    </Card>
  )
}
