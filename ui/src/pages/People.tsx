import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarDays, Check, Copy, Crown, Gauge, Mail, MessageCircle, Plus, Trash2, UserRound, Users } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { ModelChoice } from '../components/ModelChoice'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Person, type Role } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'
import { fill, useT } from '../lib/i18n'

const roles = [
  { value: 'member', title: 'people.member', text: 'people.memberText' },
  { value: 'guest', title: 'people.guest', text: 'people.guestText' },
] as const

const input = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

export function People() {
  const t = useT()
  const qc = useQueryClient()
  const people = useQuery({ queryKey: ['people'], queryFn: api.people })
  const [adding, setAdding] = useState(false)
  const list = people.data ?? []
  const others = list.filter((p) => p.role !== 'owner')

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">{t('people.title')}</h1>
          <p className="mt-1 text-sm text-ink-2">{t('people.subtitle')}</p>
        </div>
        <Button variant="primary" onClick={() => setAdding(true)}><Plus size={16} /> {t('people.invite')}</Button>
      </div>

      <AnimatePresence>{adding && <Invite people={list} onDone={() => { setAdding(false); qc.invalidateQueries({ queryKey: ['people'] }) }} />}</AnimatePresence>

      <div className="space-y-3">
        {list.filter((p) => p.role === 'owner').map((p) => <OwnerCard key={p.id} p={p} />)}
        {people.isSuccess && others.length === 0 && !adding && (
          <EmptyState icon={<Users size={22} />} title={t('people.emptyTitle')} action={<Button onClick={() => setAdding(true)}>{t('people.inviteSomeone')}</Button>}>
            {t('people.emptyText')}
          </EmptyState>
        )}
        {others.map((p) => <PersonCard key={p.id} p={p} people={list} />)}
      </div>
    </div>
  )
}

function Avatar({ name, owner }: { name: string; owner?: boolean }) {
  return (
    <div className={cn('grid size-10 shrink-0 place-items-center rounded-full text-[15px] font-semibold', owner ? 'bg-ink text-bg' : 'bg-explore-soft text-explore')}>
      {owner ? <Crown size={17} /> : name.slice(0, 1).toUpperCase()}
    </div>
  )
}

function Telegram({ p }: { p: Person }) {
  const t = useT()
  return p.chat ? (
    <span className="flex items-center gap-1 text-[12px] text-read"><MessageCircle size={13} /> {t('people.onTelegram')}</span>
  ) : (
    <span className="flex items-center gap-1 text-[12px] text-ink-3"><MessageCircle size={13} /> {t('people.notJoined')}</span>
  )
}

function OwnerCard({ p }: { p: Person }) {
  const t = useT()
  return (
    <Card className="flex items-center gap-4 p-4">
      <Avatar name={p.name} owner />
      <div className="flex-1">
        <div className="text-[15px] font-medium">{t('people.you')} <span className="ml-1 text-[12px] font-normal text-ink-3">{t('people.owner')}</span></div>
        <div className="text-[13px] text-ink-3">{t('people.ownerText')}</div>
      </div>
      <Telegram p={p} />
    </Card>
  )
}

function Invite({ people, onDone }: { people: Person[]; onDone: () => void }) {
  const t = useT()
  const [name, setName] = useState('')
  const [role, setRole] = useState<Exclude<Role, 'owner'>>('member')
  const [responsible, setResponsible] = useState('owner')
  // A member answers for themselves; only a guest has a responsible.
  const add = useMutation({ mutationFn: () => api.addPerson(name, role, role === 'guest' ? responsible : '') })
  const [copied, setCopied] = useState(false)
  const created = add.data

  return (
    <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }} className="mb-4 overflow-hidden">
      <Card className="p-5">
        {created ? (
          <div className="text-center">
            <div className="mx-auto mb-3 grid size-11 place-items-center rounded-full bg-read-soft text-read"><Check size={20} /></div>
            <div className="text-[15px] font-medium">{t('people.inviteFor', { name: created.name })}</div>
            <p className="mx-auto mt-1 max-w-sm text-[13px] text-ink-2">{t('people.inviteHow', { name: created.name })}</p>
            <button type="button" onClick={() => { navigator.clipboard?.writeText(`/start ${created.invite}`); setCopied(true) }}
              className="mx-auto mt-3 flex items-center gap-2 rounded-xl border border-line bg-sunken px-4 py-2.5 font-mono text-[15px] tracking-wide hover:border-line-strong" aria-label={t('people.copy')}>
              /start {created.invite} {copied ? <Check size={15} className="text-read" /> : <Copy size={15} className="text-ink-3" />}
            </button>
            <p className="mt-3 text-[12px] text-ink-3">{t('people.once')}</p>
            <Button className="mt-4" onClick={onDone}>{t('people.done')}</Button>
          </div>
        ) : (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); add.mutate() }}>
            <label className="block text-[13px] text-ink-2">{t('people.name')}
              <input autoFocus value={name} onChange={(e) => setName(e.target.value)} className={cn(input, 'mt-1.5')} placeholder="Ana" />
            </label>
            <div role="radiogroup" aria-label={t('people.role')} className="grid gap-2 sm:grid-cols-2">
              {roles.map((r) => (
                <button key={r.value} type="button" role="radio" aria-checked={role === r.value} onClick={() => setRole(r.value)}
                  className={cn('rounded-xl border p-3 text-left transition', role === r.value ? 'border-accent ring-1 ring-accent' : 'border-line hover:border-line-strong')}>
                  <div className="text-[14px] font-medium">{t(r.title)}</div>
                  <div className="text-[12.5px] text-ink-3">{t(r.text)}</div>
                </button>
              ))}
            </div>
            {role === 'guest' && (
              <label className="block text-[13px] text-ink-2">{t('people.approver')}
                <select value={responsible} onChange={(e) => setResponsible(e.target.value)} className={cn(input, 'mt-1.5')}>
                  {people.filter((p) => p.role !== 'guest').map((p) => <option key={p.id} value={p.id}>{p.role === 'owner' ? t('people.you') : p.name}</option>)}
                </select>
              </label>
            )}
            {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={onDone}>{t('common.cancel')}</Button>
              <Button type="submit" variant="primary" disabled={!name.trim() || add.isPending}>{t('people.create')}</Button>
            </div>
          </form>
        )}
      </Card>
    </motion.div>
  )
}

function PersonCard({ p, people }: { p: Person; people: Person[] }) {
  const t = useT()
  const qc = useQueryClient()
  const refresh = () => qc.invalidateQueries({ queryKey: ['people'] })
  const update = useMutation({ mutationFn: (v: { role: Role; responsible: string }) => api.updatePerson(p.id, v.role, v.responsible), onSuccess: refresh })
  const remove = useMutation({ mutationFn: () => api.removePerson(p.id), onSuccess: refresh })
  const [open, setOpen] = useState<'mail' | 'calendar' | 'limits' | null>(null)
  const [confirming, setConfirming] = useState(false)
  const responsible = people.find((x) => x.id === (p.responsible || 'owner'))

  return (
    <Card className="p-4">
      <div className="flex items-center gap-4">
        <Avatar name={p.name} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[15px] font-medium">{p.name}</div>
          <div className="text-[13px] text-ink-3">
            {t('people.summary', { role: t(p.role === 'guest' ? 'people.guest' : 'people.member'), who: responsible?.role === 'owner' ? t('people.youLower') : (responsible?.name ?? '') })}
          </div>
        </div>
        <Telegram p={p} />
      </div>
      {!!p.daily_limit && (
        <p className="mt-2 flex flex-wrap items-center gap-2 text-[12.5px] text-ink-2">
          <Gauge size={13} className="text-ink-3" /> {t('lim.perDay', { limit: usd(p.daily_limit) })}
          {p.limit_reached && <span className="rounded-full bg-danger-soft px-2 py-0.5 text-[11.5px] font-medium text-danger">{t('lim.reached')}</span>}
        </p>
      )}
      {!p.chat && p.invite && <p className="mt-3 rounded-lg bg-sunken px-3 py-2 text-[12.5px] text-ink-2">{fill(t('people.pending'), { code: <span className="font-mono">/start {p.invite}</span> })}</p>}
      <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-line pt-3">
        <select aria-label={t('people.roleOf', { name: p.name })} value={p.role} onChange={(e) => update.mutate({ role: e.target.value as Role, responsible: e.target.value === 'guest' ? p.responsible || 'owner' : '' })} className="h-8 rounded-lg border border-line bg-bg px-2 text-[13px]">
          <option value="member">{t('people.member')}</option>
          <option value="guest">{t('people.guest')}</option>
        </select>
        {p.role === 'guest' && (
          <select aria-label={t('people.approverOf', { name: p.name })} value={p.responsible || 'owner'} onChange={(e) => update.mutate({ role: p.role, responsible: e.target.value })} className="h-8 rounded-lg border border-line bg-bg px-2 text-[13px]">
            {people.filter((x) => x.id !== p.id && x.role !== 'guest').map((x) => <option key={x.id} value={x.id}>{t('people.approves', { who: x.role === 'owner' ? t('people.youLower') : x.name })}</option>)}
          </select>
        )}
        <Button size="sm" variant="ghost" onClick={() => setOpen(open === 'mail' ? null : 'mail')}><Mail size={14} /> {t('people.mail')}{p.mail && ' ✓'}</Button>
        <Button size="sm" variant="ghost" onClick={() => setOpen(open === 'calendar' ? null : 'calendar')}><CalendarDays size={14} /> {t('people.calendar')}{p.calendar && ' ✓'}</Button>
        <Button size="sm" variant="ghost" aria-expanded={open === 'limits'} onClick={() => setOpen(open === 'limits' ? null : 'limits')}><Gauge size={14} /> {t('lim.button')}</Button>
        <div className="flex-1" />
        {confirming ? (
          <span className="flex items-center gap-2 text-[13px]">{t('people.confirmRemove', { name: p.name })}
            <Button size="sm" variant="danger" onClick={() => remove.mutate()}>{t('common.remove')}</Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>{t('people.no')}</Button>
          </span>
        ) : (
          <Button size="sm" variant="ghost" aria-label={t('people.remove', { name: p.name })} onClick={() => setConfirming(true)}><Trash2 size={14} /></Button>
        )}
      </div>
      {open === 'limits' && <Limits p={p} onDone={() => { setOpen(null); refresh() }} />}
      {(open === 'mail' || open === 'calendar') && <Accounts p={p} kind={open} onDone={() => { setOpen(null); refresh() }} />}
    </Card>
  )
}

function Accounts({ p, kind, onDone }: { p: Person; kind: 'mail' | 'calendar'; onDone: () => void }) {
  const t = useT()
  const [v, setV] = useState<Record<string, string>>({})
  const save = useMutation({ mutationFn: () => api.personConnection(p.id, kind, v), onSuccess: onDone })
  return (
    <form className="mt-3 space-y-2 rounded-xl bg-sunken p-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
      <p className="flex items-center gap-1.5 text-[12.5px] text-ink-2"><UserRound size={13} /> {t('people.accountsNote', { name: p.name })}</p>
      {kind === 'mail' ? (
        <>
          <input className={input} placeholder={t('people.mailOf', { name: p.name })} aria-label={t('people.mail')} onChange={(e) => setV({ ...v, user: e.target.value })} />
          <input className={input} type="password" placeholder={t('people.appPassword')} aria-label={t('people.appPassword')} onChange={(e) => setV({ ...v, password: e.target.value })} />
        </>
      ) : (
        <textarea className={cn(input, 'h-20 py-2')} placeholder={t('people.feeds')} aria-label={t('people.feedsLabel')} onChange={(e) => setV({ feeds: e.target.value })} />
      )}
      {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
      <div className="flex justify-end"><Button size="sm" variant="primary" type="submit" disabled={save.isPending}>{t('common.save')}</Button></div>
    </form>
  )
}

// Limits is the owner choosing which models a person may use and what they
// may spend a day, inside the house's limit. The owner learns whether the
// limit was reached, never how much the person spent.
function Limits({ p, onDone }: { p: Person; onDone: () => void }) {
  const t = useT()
  const [models, setModels] = useState<string[] | null>(p.models?.length ? p.models : null)
  const [daily, setDaily] = useState(p.daily_usd ? String(p.daily_usd) : '')
  const amount = daily.trim() === '' ? 0 : Number(daily.replace(',', '.'))
  const bad = Number.isNaN(amount) || amount < 0
  const save = useMutation({ mutationFn: () => api.setPersonLimits(p.id, models ?? [], amount), onSuccess: onDone })
  return (
    <form className="mt-3 space-y-3 rounded-xl bg-sunken p-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
      <ModelChoice value={models} onChange={setModels} legend={t('lim.modelsOf', { name: p.name })} />
      <label className="block text-[13px] text-ink-2">{t('lim.daily')}
        <input inputMode="decimal" value={daily} onChange={(e) => setDaily(e.target.value)} className={cn(input, 'mt-1.5 max-w-40')}
          placeholder={p.role === 'guest' ? usd(0.25) : t('lim.noLimit')} />
      </label>
      <p className="text-[12.5px] text-ink-3">{p.role === 'guest' ? t('lim.guestHint', { limit: usd(0.25) }) : t('lim.memberHint')} {t('lim.privacy')}</p>
      {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
      <div className="flex justify-end"><Button size="sm" variant="primary" type="submit" disabled={bad || (models !== null && models.length === 0) || save.isPending}>{t('common.save')}</Button></div>
    </form>
  )
}
