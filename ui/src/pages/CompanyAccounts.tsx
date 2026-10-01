import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, KeyRound, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Field } from '../components/Modal'
import { Button, Card } from '../components/ui'
import { api, type MemberAccount, type Org, type SharedAccount } from '../lib/api'
import { useT } from '../lib/i18n'
import { field } from './Companies'

const accountKinds = ['service', 'machine', 'brand', 'person'] as const

function ConnectForm({ account, onSave, busy, warning, error }: { account: MemberAccount; onSave: (f: Record<string, string>) => void; busy: boolean; warning?: string; error?: string }) {
  const t = useT()
  const [values, setValues] = useState<Record<string, string>>({ ...account.values, account_kind: account.account_kind ?? 'service' })
  return (
    <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); onSave(values) }}>
      {account.fields.map((f) => (
        <Field key={f.name} label={f.label}>
          <input className={field} type={f.secret ? 'password' : 'text'} autoComplete="off" placeholder={f.secret && account.configured ? t('co.keepSecret') : f.placeholder}
            value={values[f.name] ?? ''} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })} />
        </Field>
      ))}
      <Field label={t('co.accountKind')}>
        <select className={field} value={values.account_kind} onChange={(e) => setValues({ ...values, account_kind: e.target.value })}>
          {accountKinds.map((k) => <option key={k} value={k}>{t(`co.accountKind.${k}` as 'co.accountKind.service')}</option>)}
        </select>
      </Field>
      {warning && <p role="alert" className="text-[12.5px] text-change">{warning}</p>}
      {error && <p className="text-[12.5px] text-danger">{error}</p>}
      <Button type="submit" size="sm" disabled={busy}>{t('co.connect')}</Button>
    </form>
  )
}

// MemberAccounts are a member's own accounts: what its role needs, what
// it has, and what the company shares with it.
export function MemberAccounts({ org, member }: { org: Org; member: string }) {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['member-accounts', org.id, member], queryFn: () => api.memberAccounts(org.id, member) })
  const [open, setOpen] = useState('')
  const [warning, setWarning] = useState('')
  const refresh = () => { qc.invalidateQueries({ queryKey: ['member-accounts', org.id, member] }); qc.invalidateQueries({ queryKey: ['company', org.id] }) }
  const save = useMutation({ mutationFn: ({ kind, f }: { kind: string; f: Record<string, string> }) => api.saveMemberAccount(org.id, member, kind, f), onSuccess: (r) => { setWarning(r.warning); if (!r.warning) setOpen(''); refresh() } })
  const remove = useMutation({ mutationFn: (kind: string) => api.deleteMemberAccount(org.id, member, kind), onSuccess: refresh })
  const missing = list.data?.missing ?? []
  const shown = (list.data?.accounts ?? []).filter((a) => a.configured || a.shared || missing.includes(a.kind) || a.kind === open)
  const others = (list.data?.accounts ?? []).filter((a) => !shown.includes(a))
  return (
    <div className="space-y-2">
      <p className="text-[12px] text-ink-3">{t('co.accountsHint')}</p>
      {missing.length > 0 && <p className="text-[12.5px] text-change">{t('co.missingAccounts', { list: missing.join(', ') })}</p>}
      {shown.map((a) => (
        <div key={a.kind} className="rounded-xl border border-line p-3">
          <div className="flex items-center gap-2">
            {a.configured ? <Check size={14} className="text-read" aria-hidden /> : <KeyRound size={14} className="text-ink-3" aria-hidden />}
            <span className="flex-1 text-[13px] font-medium">{a.name}</span>
            {a.shared && <span className="text-[11.5px] text-ink-3">{t('co.sharedWith', { grant: t(`co.grant2.${a.shared}` as 'co.grant2.read') })}</span>}
            {a.configured && <Button type="button" size="sm" variant="ghost" aria-label={t('co.disconnect', { name: a.name })} onClick={() => remove.mutate(a.kind)}><Trash2 size={13} /></Button>}
            {!a.configured && open !== a.kind && <Button type="button" size="sm" onClick={() => { setOpen(a.kind); setWarning('') }}>{t('co.connect')}</Button>}
          </div>
          {open === a.kind && <div className="mt-2"><ConnectForm account={a} busy={save.isPending} warning={warning} error={save.error?.message} onSave={(f) => save.mutate({ kind: a.kind, f })} /></div>}
        </div>
      ))}
      {others.length > 0 && (
        <select className={field} value="" aria-label={t('co.addAccount')} onChange={(e) => { setOpen(e.target.value); setWarning('') }}>
          <option value="">{t('co.addAccount')}</option>
          {others.map((a) => <option key={a.kind} value={a.kind}>{a.name}</option>)}
        </select>
      )}
    </div>
  )
}

// SharedAccounts are the company's own accounts and who may use them.
export function SharedAccounts({ org, can, onSaved }: { org: Org; can: boolean; onSaved: (o: Org) => void }) {
  const t = useT()
  const agents = org.members.filter((m) => m.kind === 'agent')
  const kinds = useQuery({ queryKey: ['member-accounts', org.id, agents[0]?.id], queryFn: () => api.memberAccounts(org.id, agents[0].id), enabled: agents.length > 0 && can })
  const [accounts, setAccounts] = useState<SharedAccount[]>(org.accounts ?? [])
  const [adding, setAdding] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({ account_kind: 'brand' })
  const connect = useMutation({ mutationFn: () => api.saveCompanyAccount(org.id, adding, fields) })
  const save = useMutation({ mutationFn: () => api.saveCompany(org.id, { ...org, accounts }), onSuccess: onSaved })
  const kindsList = kinds.data?.accounts ?? []
  const add = async () => {
    const r = await connect.mutateAsync()
    if (!accounts.some((a) => a.kind === adding)) setAccounts([...accounts, { kind: adding, grants: {} }])
    setAdding('')
    setFields({ account_kind: 'brand' })
    return r
  }
  const grant = (kind: string, member: string, g: string) => setAccounts(accounts.map((a) => (a.kind !== kind ? a : { ...a, grants: Object.fromEntries(Object.entries({ ...a.grants, [member]: g }).filter(([, v]) => v)) as SharedAccount['grants'] })))
  return (
    <section className="space-y-3">
      <h2 className="text-[15px] font-semibold">{t('co.sharedAccounts')}</h2>
      <p className="text-[12.5px] text-ink-3">{t('co.sharedHint')}</p>
      {accounts.map((a) => (
        <Card key={a.kind} className="space-y-2 p-3">
          <div className="flex items-center gap-2">
            <span className="flex-1 text-[13.5px] font-medium">{kindsList.find((k) => k.kind === a.kind)?.name ?? a.kind}</span>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.disconnect', { name: a.kind })} onClick={async () => { await api.deleteCompanyAccount(org.id, a.kind); setAccounts(accounts.filter((x) => x.kind !== a.kind)) }}><Trash2 size={13} /></Button>}
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            {agents.map((m) => (
              <label key={m.id} className="flex items-center justify-between gap-2 text-[12.5px]">{m.name}
                <select className={field + ' h-8 w-36'} disabled={!can} value={a.grants?.[m.id] ?? ''} onChange={(e) => grant(a.kind, m.id, e.target.value)}>
                  <option value="">{t('co.grant2.none')}</option>
                  <option value="read">{t('co.grant2.read')}</option>
                  <option value="act">{t('co.grant2.act')}</option>
                </select>
              </label>
            ))}
          </div>
        </Card>
      ))}
      {can && (
        <div className="space-y-2">
          <select className={field} value={adding} aria-label={t('co.addShared')} onChange={(e) => setAdding(e.target.value)}>
            <option value="">{t('co.addShared')}</option>
            {kindsList.filter((k) => !accounts.some((a) => a.kind === k.kind)).map((k) => <option key={k.kind} value={k.kind}>{k.name}</option>)}
          </select>
          {adding && (
            <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); add() }}>
              {kindsList.find((k) => k.kind === adding)?.fields.map((f) => (
                <Field key={f.name} label={f.label}><input className={field} type={f.secret ? 'password' : 'text'} autoComplete="off" placeholder={f.placeholder} value={fields[f.name] ?? ''} onChange={(e) => setFields({ ...fields, [f.name]: e.target.value })} /></Field>
              ))}
              {connect.data?.warning && <p role="alert" className="text-[12.5px] text-change">{connect.data.warning}</p>}
              <Button type="submit" size="sm" disabled={connect.isPending}>{t('co.connect')}</Button>
            </form>
          )}
          <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        </div>
      )}
    </section>
  )
}
