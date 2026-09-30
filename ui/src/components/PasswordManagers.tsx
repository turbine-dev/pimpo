import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, KeyRound, Loader2 } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, type PasswordManagerInput, type PasswordManagers as View } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { Button, Card } from './ui'

type Kind = 'onepassword' | 'hashicorp'

const field = 'h-9 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// PasswordManagers sets up 1Password and HashiCorp Vault for whoever is
// signed in: the house's for the owner, a person's own for everyone else.
// Tokens go to the server and never come back.
export function PasswordManagers() {
  const t = useT()
  const q = useQuery({ queryKey: ['password-managers'], queryFn: api.passwordManagers })
  const v = q.data
  if (!v) return null
  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><KeyRound size={17} /> {t('pm.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t(v.house ? 'pm.textHouse' : 'pm.textOwn')}</p>
      <div className="divide-y divide-line rounded-xl border border-line">
        <Manager kind="onepassword" v={v} title="1Password" status={v.onepassword.mode && t(`pm.mode.${v.onepassword.mode}`)}>
          {(done) => <OnePasswordForm v={v} done={done} />}
        </Manager>
        <Manager kind="hashicorp" v={v} title="HashiCorp Vault" status={v.hashicorp.auth && `${v.hashicorp.addr} · ${t(`pm.auth.${v.hashicorp.auth}`)}`}>
          {(done) => <HashiCorpForm v={v} done={done} />}
        </Manager>
      </div>
    </Card>
  )
}

function Manager({ kind, v, title, status, children }: { kind: Kind; v: View; title: string; status: string; children: (done: () => void) => ReactNode }) {
  const t = useT()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const test = useMutation({ mutationFn: () => api.testPasswordManager(kind) })
  const remove = useMutation({ mutationFn: () => api.deletePasswordManager(kind), onSuccess: (d) => { qc.setQueryData(['password-managers'], d); test.reset() } })
  const set = kind === 'onepassword' ? !!v.onepassword.mode : !!v.hashicorp.auth
  return (
    <div className="p-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="min-w-0 flex-1">
          <div className="text-[14px] font-medium">{title}</div>
          <div className="truncate text-[12.5px] text-ink-3">{set ? status : t('pm.notSet')}</div>
        </div>
        {set && <Button size="sm" variant="ghost" onClick={() => test.mutate()} disabled={test.isPending}>{test.isPending && <Loader2 size={13} className="animate-spin" />} {t('common.test')}</Button>}
        <Button size="sm" variant={set ? 'ghost' : 'secondary'} onClick={() => setOpen(!open)} aria-expanded={open}>{set ? t('conn.change') : t('conn.setUp')}</Button>
        {set && <Button size="sm" variant="ghost" onClick={() => remove.mutate()} aria-label={t('pm.remove', { name: title })}>{t('common.remove')}</Button>}
      </div>
      {test.isSuccess && <p role="status" className="mt-2 flex items-center gap-1 text-[12.5px] text-read"><Check size={13} /> {t('pm.works')}</p>}
      {test.error && <p role="alert" className="mt-2 text-[12.5px] text-danger">{test.error.message}</p>}
      {open && children(() => { setOpen(false); test.reset() })}
    </div>
  )
}

function useSave(kind: Kind, done: () => void) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: PasswordManagerInput) => api.putPasswordManager(kind, body),
    onSuccess: (d) => { qc.setQueryData(['password-managers'], d); done() },
  })
}

function Choice<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div role="radiogroup" aria-label={label} className="flex flex-wrap gap-1.5">
      {options.map(([o, text]) => (
        <button key={o} type="button" role="radio" aria-checked={value === o} onClick={() => onChange(o)}
          className={cn('rounded-lg border px-2.5 py-1 text-[12.5px]', value === o ? 'border-ink bg-ink text-bg' : 'border-line hover:bg-sunken')}>{text}</button>
      ))}
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[12.5px] text-ink-2">{label}</span>
      {children}
    </label>
  )
}

function OnePasswordForm({ v, done }: { v: View; done: () => void }) {
  const t = useT()
  const [mode, setMode] = useState(v.onepassword.mode || 'service')
  const [token, setToken] = useState('')
  const [url, setUrl] = useState(v.onepassword.connect_url ?? '')
  const [connectToken, setConnectToken] = useState('')
  const save = useSave('onepassword', done)
  const saved = (m: string) => v.onepassword.mode === m ? t('pm.keepSaved') : ''
  const modes: ['service' | 'desktop' | 'connect', string][] = [['service', t('pm.mode.service')], ['connect', t('pm.mode.connect')]]
  if (v.house) modes.splice(1, 0, ['desktop', t('pm.mode.desktop')])
  return (
    <form className="mt-3 space-y-3 border-t border-line pt-3" onSubmit={(e) => { e.preventDefault(); save.mutate({ mode, token, connect_url: url, connect_token: connectToken }) }}>
      <Choice label={t('pm.how')} value={mode} options={modes} onChange={setMode} />
      {mode !== 'connect' && !v.op_installed && <p className="text-[12.5px] text-danger">{t('pm.opMissing')}</p>}
      {mode === 'desktop' && <p className="text-[12.5px] text-ink-3">{t('pm.desktopHelp')}</p>}
      {mode === 'service' && <Field label={t('pm.serviceToken')}><input className={field} type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} placeholder={saved('service') || 'ops_…'} /></Field>}
      {mode === 'connect' && (
        <>
          <Field label={t('pm.connectUrl')}><input className={field} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://op-connect.example.com" inputMode="url" autoComplete="off" /></Field>
          <Field label={t('pm.connectToken')}><input className={field} type="password" autoComplete="off" value={connectToken} onChange={(e) => setConnectToken(e.target.value)} placeholder={saved('connect')} /></Field>
        </>
      )}
      {save.error && <p className="text-[12.5px] text-danger">{save.error.message}</p>}
      <Button size="sm" variant="primary" type="submit" disabled={save.isPending}>{t('common.save')}</Button>
    </form>
  )
}

function HashiCorpForm({ v, done }: { v: View; done: () => void }) {
  const t = useT()
  const h = v.hashicorp
  const [addr, setAddr] = useState(h.addr ?? '')
  const [auth, setAuth] = useState<'token' | 'approle'>(h.auth || 'token')
  const [token, setToken] = useState('')
  const [role, setRole] = useState('')
  const [secret, setSecret] = useState('')
  const [ns, setNs] = useState(h.namespace ?? '')
  const [mount, setMount] = useState(h.auth_mount ?? '')
  const save = useSave('hashicorp', done)
  const saved = h.auth === auth ? t('pm.keepSaved') : ''
  return (
    <form className="mt-3 space-y-3 border-t border-line pt-3" onSubmit={(e) => { e.preventDefault(); save.mutate({ addr, auth, token, role_id: role, secret_id: secret, namespace: ns, auth_mount: mount }) }}>
      <Field label={t('pm.addr')}><input className={field} value={addr} onChange={(e) => setAddr(e.target.value)} placeholder="https://vault.example.com:8200" inputMode="url" autoComplete="off" /></Field>
      <Choice label={t('pm.signIn')} value={auth} options={[['token', t('pm.auth.token')], ['approle', t('pm.auth.approle')]]} onChange={setAuth} />
      {auth === 'token'
        ? <Field label={t('pm.vaultToken')}><input className={field} type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} placeholder={saved || 'hvs.…'} /></Field>
        : (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('pm.roleId')}><input className={field} autoComplete="off" value={role} onChange={(e) => setRole(e.target.value)} placeholder={saved} /></Field>
            <Field label={t('pm.secretId')}><input className={field} type="password" autoComplete="off" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={saved} /></Field>
            <Field label={`${t('pm.authMount')} (${t('common.optional')})`}><input className={field} value={mount} onChange={(e) => setMount(e.target.value)} placeholder="approle" /></Field>
          </div>
        )}
      <Field label={`${t('pm.namespace')} (${t('common.optional')})`}><input className={field} value={ns} onChange={(e) => setNs(e.target.value)} /></Field>
      <p className="text-[12.5px] text-ink-3">{t('pm.hvHelp')}</p>
      {save.error && <p className="text-[12.5px] text-danger">{save.error.message}</p>}
      <Button size="sm" variant="primary" type="submit" disabled={save.isPending}>{t('common.save')}</Button>
    </form>
  )
}
