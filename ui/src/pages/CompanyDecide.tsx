import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { capabilityLabel } from '../components/RoutineCard'
import { Field } from '../components/Modal'
import { Button, Card, Switch } from '../components/ui'
import { api, type Autonomy, type Decider, type DeciderKind, type Level, type Levels, type Org } from '../lib/api'
import { relative } from '../lib/format'
import { type T, useT } from '../lib/i18n'
import { field, SectionHead } from './Companies'

const kinds: DeciderKind[] = ['person', 'self', 'jev', 'laya', 'model', 'boss', 'committee', 'cascade']
const stepKinds: DeciderKind[] = ['jev', 'laya', 'model', 'boss', 'committee', 'self', 'person']
const risks = ['reversible', 'irreversible', 'notify', 'read']

// DeciderEditor chooses who decides; a cascade's steps are chosen the
// same way, one after another.
export function DeciderEditor({ org, value, onChange, step }: { org: Org; value: Decider; onChange: (d: Decider) => void; step?: boolean }) {
  const t = useT()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const set = (kind: DeciderKind) => {
    const d: Decider = { kind }
    if (kind === 'jev' || kind === 'laya') d.threshold = 0.9
    if (kind === 'committee') d.members = agents.slice(0, 2).map((a) => a.id)
    if (kind === 'cascade') d.steps = [{ kind: 'jev', threshold: 0.9 }, { kind: 'person' }]
    onChange(d)
  }
  return (
    <div className="space-y-2">
      <select className={field} aria-label={t('co.decider')} value={value.kind} onChange={(e) => set(e.target.value as DeciderKind)}>
        {(step ? stepKinds : kinds).map((k) => <option key={k} value={k}>{t(`co.decider.${k}` as 'co.decider.self')}</option>)}
      </select>
      {(value.kind === 'jev' || value.kind === 'laya') && (
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
          {a.earned && <p className="text-[12px] text-read">{t('co.earned')}</p>}
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

// levelWhen says in words what puts a decision at a level; the first
// level is what no other takes.
function levelWhen(t: T, l: Level, first: boolean) {
  if (first) return t('co.levelRest')
  const w = l.when
  const out: string[] = []
  if (w.over_usd) out.push(t('co.levelOver', { usd: `$${w.over_usd}` }))
  if (w.min_risk) out.push(t('rules.orMore', { what: t(`rules.risk.${w.min_risk}` as 'rules.risk.read') }))
  if (w.kinds?.length) out.push(t('co.levelKinds', { list: w.kinds.join(', ') }))
  if (w.words?.length) out.push(t('co.levelWords', { list: w.words.join(', ') }))
  if (w.public) out.push(t('co.levelPublic'))
  return out.length ? t('co.levelWhen', { list: out.join(t('co.levelOr')) }) : t('co.levelNever')
}

function LevelCard({ l, first, last, can, onChange }: { l: Level; first: boolean; last: boolean; can: boolean; onChange: (l: Level) => void }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const who = t(`co.levelBy.${l.decides}` as 'co.levelBy.ceo') + (l.decides === 'ceo' && l.route === 'opinions' ? ` · ${t('co.route.opinions')}` : '')
  return (
    <Card className="p-0">
      <div className="flex items-start gap-3 p-3.5">
        <span className="grid size-7 shrink-0 place-items-center rounded-full bg-sunken text-[12.5px] font-semibold" aria-hidden>{l.level}</span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-[13.5px] font-medium">{l.name || t('co.levelN', { n: l.level })}</p>
          <p className="text-[12.5px] text-ink-2">{levelWhen(t, l, first)}</p>
          <p className="text-[12.5px] text-ink-3">{t('co.levelWho', { who })}</p>
        </div>
        {can && <Button type="button" size="sm" variant="ghost" aria-expanded={open} aria-label={t('co.editLevel', { name: l.name || t('co.levelN', { n: l.level }) })} onClick={() => setOpen(!open)}><Pencil size={14} /></Button>}
      </div>
      {open && (
        <div className="space-y-3 border-t border-line p-3.5">
          <Field label={t('co.levelName')}><input className={field} value={l.name} onChange={(e) => onChange({ ...l, name: e.target.value })} /></Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('co.levelDecides')}>
              <select className={field} value={l.decides} disabled={last} onChange={(e) => onChange({ ...l, decides: e.target.value as Level['decides'] })}>
                {decides.map((d) => <option key={d} value={d}>{t(`co.levelBy.${d}` as 'co.levelBy.ceo')}</option>)}
              </select>
            </Field>
            {l.decides === 'ceo' && (
              <Field label={t('co.route')}>
                <select className={field} value={l.route ?? 'direct'} onChange={(e) => onChange({ ...l, route: e.target.value as Level['route'] })}>
                  <option value="direct">{t('co.route.direct')}</option>
                  <option value="opinions">{t('co.route.opinions')}</option>
                </select>
              </Field>
            )}
          </div>
          {!first && (
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={t('co.overUsd')}><input type="number" min={0} className={field} value={l.when.over_usd ?? ''} onChange={(e) => onChange({ ...l, when: { ...l.when, over_usd: e.target.value ? Number(e.target.value) : undefined } })} /></Field>
              <Field label={t('co.ruleRisk')}>
                <select className={field} value={l.when.min_risk ?? ''} onChange={(e) => onChange({ ...l, when: { ...l.when, min_risk: e.target.value || undefined } })}>
                  <option value="">{t('co.anyRisk')}</option>
                  {risks.map((r) => <option key={r} value={r}>{t('rules.orMore', { what: t(`rules.risk.${r}` as 'rules.risk.read') })}</option>)}
                </select>
              </Field>
              <Field label={t('co.kinds')}><input className={field} value={(l.when.kinds ?? []).join(', ')} placeholder={t('co.kindsHint')} onChange={(e) => onChange({ ...l, when: { ...l.when, kinds: list(e.target.value) } })} /></Field>
              <Field label={t('co.words')}><input className={field} value={(l.when.words ?? []).join(', ')} onChange={(e) => onChange({ ...l, when: { ...l.when, words: list(e.target.value) } })} /></Field>
              <label className="flex items-center gap-2 text-[13px] sm:col-span-2"><input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={!!l.when.public} onChange={(e) => onChange({ ...l, when: { ...l.when, public: e.target.checked } })} /> {t('co.publicTrigger')}</label>
            </div>
          )}
          <Button type="button" size="sm" onClick={() => setOpen(false)}>{t('co.levelDone')}</Button>
        </div>
      )}
    </Card>
  )
}

export function LevelsTab({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [levels, setLevels] = useState<Levels>(org.levels ?? {})
  const [decider, setDecider] = useState<Decider>(org.decider?.kind ? org.decider : { kind: 'person' })
  const [earn, setEarn] = useState({ after: org.earn_after ?? 10, off: !!org.earn_off })
  const decisions = useQuery({ queryKey: ['company-decisions', org.id], queryFn: () => api.companyDecisions(org.id) })
  const save = useMutation({
    mutationFn: () => api.saveCompany(org.id, { ...org, levels, decider: decider.kind === 'person' ? undefined : decider, earn_after: earn.after === 10 ? undefined : earn.after, earn_off: earn.off || undefined }),
    onSuccess: (o) => { onSaved(o); qc.invalidateQueries({ queryKey: ['company', org.id] }) },
  })
  const all = levels.list ?? []
  const put = (i: number, l: Level) => setLevels({ ...levels, list: all.map((x, j) => (j === i ? l : x)) })
  const name = (id: string) => org.members.find((m) => m.id === id)?.name ?? id
  return (
    <div className="grid gap-8 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      <fieldset disabled={!can} className="min-w-0 space-y-8">
        <section className="space-y-3">
          <SectionHead title={t('co.levels')} hint={t('co.levelsHint')} />
          {all.length === 0 && (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-card)] border border-dashed border-line-strong p-4">
              <p className="min-w-0 flex-1 basis-56 text-[13px] text-ink-2">{t('co.noLevels')}</p>
              {can && <Button type="button" onClick={() => setLevels(suggested(t))}>{t('co.useSuggested')}</Button>}
            </div>
          )}
          <ol className="space-y-2">
            {all.map((l, i) => <li key={l.level}><LevelCard l={l} first={i === 0} last={i === all.length - 1} can={can} onChange={(x) => put(i, x)} /></li>)}
          </ol>
        </section>
        <section className="space-y-4">
          <SectionHead title={t('co.decideMore')} />
          <div className="space-y-2">
            <span className="text-[13px] font-medium">{t('co.defaultDecider')}</span>
            <DeciderEditor org={org} value={decider} onChange={setDecider} />
          </div>
          <div className="space-y-2 rounded-xl bg-sunken/60 p-3.5">
            <div className="flex items-center justify-between gap-3">
              <span className="text-[13px] font-medium">{t('co.earn')}</span>
              <Switch on={!earn.off} onChange={(on) => setEarn({ ...earn, off: !on })} label={t('co.earn')} />
            </div>
            <p className="text-[12.5px] text-ink-3">{t('co.earnHint')}</p>
            {!earn.off && <Field label={t('co.earnAfter')}><input type="number" min={1} max={100} className={field + ' w-24'} value={earn.after} onChange={(e) => setEarn({ ...earn, after: Math.max(1, Math.min(100, Number(e.target.value) || 1)) })} /></Field>}
          </div>
        </section>
        {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        {can && <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>}
      </fieldset>
      <aside className="min-w-0 space-y-6">
        <Simulator org={org} />
        <section className="space-y-2">
          <SectionHead title={t('co.decisions')} />
          {(decisions.data ?? []).length === 0 && <p className="text-[12.5px] text-ink-3">{t('co.noDecisions')}</p>}
          {(decisions.data ?? []).map((d) => (
            <Card key={d.id} className="p-3 text-[12.5px]">
              <p className="text-ink-3">{name(d.member)} · {d.decider}{d.level ? ` · ${t('co.levelN', { n: d.level })}` : ''} · {relative(d.created)}</p>
              <p className="truncate font-mono text-[12px]">{d.question}</p>
              <p><b className="font-medium">{t(`co.answer.${d.answer}` as 'co.answer.allow')}</b>{d.reason ? `: ${d.reason}` : ''}</p>
            </Card>
          ))}
        </section>
      </aside>
    </div>
  )
}

function Simulator({ org }: { org: Org }) {
  const t = useT()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const [q, setQ] = useState({ member: agents[0]?.id ?? '', text: '', kind: '', amount_usd: 0, public: false })
  const run = useMutation({ mutationFn: () => api.simulateLevel(org.id, q) })
  return (
    <div className="space-y-3 rounded-[var(--radius-card)] border border-line bg-sunken/60 p-4">
      <SectionHead title={t('co.simulator')} hint={t('co.simulatorHint')} />
      <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); run.mutate() }}>
        {/* The panel is narrow beside the levels: the decision gets two lines, and who, kind and amount stack. */}
        <textarea rows={2} className={field + ' h-auto resize-none py-2'} aria-label={t('co.simText')} placeholder={t('co.simTextHint')} value={q.text} onChange={(e) => setQ({ ...q, text: e.target.value })} />
        <select className={field} aria-label={t('co.simWho')} value={q.member} onChange={(e) => setQ({ ...q, member: e.target.value })}>{agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}</select>
        <div className="grid grid-cols-2 gap-2">
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
    </div>
  )
}
