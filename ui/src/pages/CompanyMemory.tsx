import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BookOpen, Check, Plus, Trash2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button, Card, EmptyState, Switch } from '../components/ui'
import { api, type CompanyNote, type MemoryPolicy, type MemoryScope, type Org, type ScopePolicy } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { area, field, SectionHead } from './Companies'
import { DeciderEditor } from './CompanyDecide'

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

// author is who wrote a note: a person, Pimpo itself, or a member.
function author(org: Org, by: string, you: string, pimpo: string) {
  if (by.startsWith('human:')) return you
  if (by === 'system') return pimpo
  return nameOf(org, by.split('/').pop() ?? by)
}

const scopes: MemoryScope[] = ['company', 'member', 'task']

// defaults mirror the server's: members write freely to their own and
// their tasks' memory, and the company's asks a person.
function policyOf(p: MemoryPolicy | undefined, scope: MemoryScope): ScopePolicy {
  const sp = { ...p?.[scope] }
  sp.write ??= scope === 'company' ? 'decide' : 'free'
  if (sp.write === 'decide') sp.decider ??= { kind: 'person' }
  return sp
}

export function Digest({ org }: { org: Org }) {
  const t = useT()
  const digest = useQuery({ queryKey: ['company-digest', org.id], queryFn: () => api.companyDigest(org.id) })
  if (!digest.data) return null
  return (
    <Card className="p-4">
      <h2 className="mb-1 text-[13.5px] font-semibold">{t('co.thisWeek')}</h2>
      <p className="whitespace-pre-line text-[12.5px] text-ink-2">{digest.data.text}</p>
    </Card>
  )
}

export function MemoryTab({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const notes = useQuery({ queryKey: ['company-notes', org.id], queryFn: () => api.companyNotes(org.id) })
  const tasks = useQuery({ queryKey: ['company-tasks', org.id], queryFn: () => api.companyTasks(org.id) })
  const [scope, setScope] = useState<MemoryScope>('company')
  const [of, setOf] = useState('')
  const [note, setNote] = useState({ kind: 'decision', title: '', body: '' })
  const [adding, setAdding] = useState(false)
  const refresh = () => { qc.invalidateQueries({ queryKey: ['company-notes', org.id] }); qc.invalidateQueries({ queryKey: ['needs'] }) }
  const save = useMutation({
    mutationFn: () => api.saveCompanyNote(org.id, { ...note, scope, of: scope === 'company' ? undefined : of }),
    onSuccess: () => { setNote({ ...note, title: '', body: '' }); setAdding(false); refresh() },
  })
  const remove = useMutation({ mutationFn: (id: string) => api.deleteCompanyNote(org.id, id), onSuccess: refresh })
  const approve = useMutation({ mutationFn: (id: string) => api.approveCompanyNote(org.id, id), onSuccess: refresh })
  const agents = org.members.filter((m) => m.kind === 'agent')
  const taskTitle = (id: string) => tasks.data?.find((x) => x.id === id)?.title ?? id
  const choices = scope === 'member' ? agents.map((m) => ({ id: m.id, name: m.name })) : scope === 'task' ? (tasks.data ?? []).map((x) => ({ id: x.id, name: x.title })) : []
  const all = notes.data ?? []
  const pending = all.filter((n) => n.pending)
  const shown = all.filter((n) => !n.pending && (n.scope ?? 'company') === scope && (scope === 'company' || !of || n.of === of))
  const whose = (n: CompanyNote) => n.scope === 'member' ? nameOf(org, n.of ?? '') : n.scope === 'task' ? taskTitle(n.of ?? '') : ''
  const pick = (s: MemoryScope) => { setScope(s); setOf('') }
  const canAdd = can && (scope === 'company' || !!of)
  const add = can && !adding && <Button variant="primary" disabled={!canAdd} onClick={() => setAdding(true)}><Plus size={15} /> {t('co.newNote')}</Button>
  const nothing = notes.isSuccess && all.length === 0
  return (
    <div className="max-w-3xl space-y-10">
      {pending.length > 0 && (
        <section className="space-y-3">
          <SectionHead title={t('co.notesPending')} hint={t('co.notesPendingHint')} />
          {pending.map((n) => (
            <NoteCard key={n.id} org={org} n={n} whose={whose(n)}>
              {can && <Button size="sm" onClick={() => approve.mutate(n.id)}><Check size={13} /> {t('co.keepNote')}</Button>}
              {can && <Button size="sm" variant="ghost" aria-label={t('co.forget', { name: n.title })} onClick={() => remove.mutate(n.id)}><Trash2 size={14} /></Button>}
            </NoteCard>
          ))}
        </section>
      )}
      <section className="space-y-3">
        <SectionHead title={t('co.memory')} hint={t('co.memoryHint')} action={!nothing && add} />
        {nothing && !adding ? <EmptyState icon={<BookOpen />} title={t('co.empty.memory')} action={add}>{t('co.empty.memoryText')}</EmptyState> : (<>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t('co.memoryScope')}>
          {scopes.map((s) => (
            <Button key={s} size="sm" variant={scope === s ? 'primary' : 'secondary'} aria-pressed={scope === s} onClick={() => pick(s)}>{t(`co.mem.${s}` as 'co.mem.company')}</Button>
          ))}
          {scope !== 'company' && (
            <select className={field + ' w-full sm:w-56'} aria-label={t(`co.scopeOf.${scope}` as 'co.scopeOf.member')} value={of} onChange={(e) => setOf(e.target.value)}>
              <option value="">{t('co.scopeAll')}</option>
              {choices.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          )}
        </div>
        {canAdd && adding && (
          <form className="space-y-2 rounded-[var(--radius-card)] border border-line p-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
            <div className="flex flex-col gap-2 sm:flex-row">
              <select className={field + ' sm:w-36'} aria-label={t('co.noteKind')} value={note.kind} onChange={(e) => setNote({ ...note, kind: e.target.value })}>
                {['decision', 'fact', 'lesson'].map((k) => <option key={k} value={k}>{t(`co.note.${k}` as 'co.note.fact')}</option>)}
              </select>
              <input className={field} aria-label={t('co.noteTitle')} placeholder={t('co.noteTitle')} value={note.title} maxLength={120} onChange={(e) => setNote({ ...note, title: e.target.value })} />
            </div>
            <textarea className={area} aria-label={t('co.noteText')} placeholder={t('co.noteText')} value={note.body} onChange={(e) => setNote({ ...note, body: e.target.value })} />
            {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
            <div className="flex gap-2">
              <Button type="submit" size="sm" variant="primary" disabled={!note.title.trim() || !note.body.trim() || save.isPending}><Plus size={13} /> {t('co.keep')}</Button>
              <Button type="button" size="sm" variant="ghost" onClick={() => setAdding(false)}>{t('common.cancel')}</Button>
            </div>
          </form>
        )}
        {shown.length === 0 && <p className="text-[13px] text-ink-3">{t('co.noNotes')}</p>}
        {shown.map((n) => (
          <NoteCard key={n.id} org={org} n={n} whose={of ? '' : whose(n)}>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.forget', { name: n.title })} onClick={() => remove.mutate(n.id)}><Trash2 size={14} /></Button>}
          </NoteCard>
        ))}
        </>)}
      </section>
      <MemoryRules org={org} can={can} onSaved={onSaved} />
    </div>
  )
}

function NoteCard({ org, n, whose, children }: { org: Org; n: CompanyNote; whose: string; children: ReactNode }) {
  const t = useT()
  return (
    <Card className="flex items-start gap-3 p-4">
      <div className="min-w-0 flex-1">
        <p className="text-[12px] text-ink-3">{[t(`co.note.${n.kind}` as 'co.note.fact'), whose, author(org, n.by, t('co.you'), 'Pimpo'), relative(n.created)].filter(Boolean).join(' · ')}</p>
        <p className="text-[14px] font-medium">{n.title}</p>
        <p className="line-clamp-4 whitespace-pre-line text-[12.5px] text-ink-2">{n.body}</p>
      </div>
      <div className="flex shrink-0 gap-1">{children}</div>
    </Card>
  )
}

// MemoryRules say, per scope, how agents write to it and whether Pimpo
// keeps a note there by itself after each piece of work.
function MemoryRules({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const [policy, setPolicy] = useState<MemoryPolicy>(() => Object.fromEntries(scopes.map((s) => [s, policyOf(org.memory, s)])))
  const save = useMutation({ mutationFn: () => api.saveCompany(org.id, { ...org, memory: policy }), onSuccess: onSaved })
  const set = (s: MemoryScope, sp: ScopePolicy) => setPolicy({ ...policy, [s]: sp })
  return (
    <section className="space-y-3">
      <SectionHead title={t('co.memoryRules')} hint={t('co.memoryRulesHint')} />
      {scopes.map((s) => {
        const sp = policyOf(policy, s)
        return (
          <Card key={s} className="space-y-3 p-4">
            <h3 className="text-[13.5px] font-semibold">{t(`co.mem.${s}` as 'co.mem.company')}</h3>
            <select className={field} aria-label={t('co.memoryWrite', { scope: t(`co.mem.${s}` as 'co.mem.company') })} disabled={!can} value={sp.write}
              onChange={(e) => set(s, { ...sp, write: e.target.value as ScopePolicy['write'], decider: e.target.value === 'decide' ? sp.decider ?? { kind: 'person' } : undefined })}>
              {(['free', 'decide', 'off'] as const).map((w) => <option key={w} value={w}>{t(`co.write.${w}` as 'co.write.free')}</option>)}
            </select>
            {sp.write === 'decide' && can && <DeciderEditor org={org} value={sp.decider ?? { kind: 'person' }} onChange={(d) => set(s, { ...sp, decider: d })} />}
            {s !== 'company' && (
              <div className="flex items-center justify-between gap-3 text-[13px]">
                <span>{t('co.autoNotes')}</span>
                <Switch on={!!sp.auto} disabled={!can || sp.write === 'off'} onChange={(v) => set(s, { ...sp, auto: v })} label={t('co.autoNotesOf', { scope: t(`co.mem.${s}` as 'co.mem.company') })} />
              </div>
            )}
          </Card>
        )
      })}
      {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
      {can && <Button onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>}
    </section>
  )
}
