import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Globe, Home, Loader2, Smartphone, Trash2 } from 'lucide-react'
import QRCode from 'qrcode'
import { useEffect, useState, type ReactNode } from 'react'
import { api, type RemoteState } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { Button, Card, Switch } from './ui'

const field = 'h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// How the phone reaches this Pimpo (home Wi-Fi, Tailscale built in, or an
// address of the owner's), and the devices paired to it. Each device gets
// its own link, shown once as a QR code; revoking one cuts off only it.
export function PhonePairing() {
  const t = useT()
  const qc = useQueryClient()
  const pairing = useQuery({ queryKey: ['pairing'], queryFn: api.pairing })
  const remote = useQuery({
    queryKey: ['remote'], queryFn: api.remote,
    refetchInterval: (q) => (['starting', 'needs_login', 'needs_funnel'].includes(q.state.data?.tailscale.state ?? '') ? 2000 : false),
  })
  const toggle = useMutation({
    mutationFn: ({ kind, on }: { kind: 'tailscale' | 'lan'; on: boolean }) => api.switchRemote(kind, on),
    onSuccess: (d) => { qc.setQueryData(['remote'], d); qc.invalidateQueries({ queryKey: ['pairing'] }) },
  })
  const [manual, setManual] = useState('')
  const [showManual, setShowManual] = useState(false)
  const [name, setName] = useState('')
  // Whose device: it signs in as that person and sees only their things.
  const people = useQuery({ queryKey: ['people'], queryFn: api.people })
  const [whose, setWhose] = useState('')
  const nameOf = (id: string) => (id === 'owner' || !id ? t('pair.me') : people.data?.find((p) => p.id === id)?.name ?? id)
  const [qr, setQr] = useState('')
  useEffect(() => {
    const b = pairing.data?.base ?? ''
    if (b && b !== remote.data?.tailscale.url) { setManual(b); setShowManual(true) }
  }, [pairing.data, remote.data])
  const r = remote.data
  const tsURL = r?.tailscale.state === 'running' ? r.tailscale.url : undefined
  const base = tsURL ?? (showManual ? manual.trim() : '')
  const reachable = !!base || !!r?.lan.on
  const pair = useMutation({
    mutationFn: () => api.setPairing(base, name.trim() || t('phone.defaultName'), whose),
    onSuccess: async (d) => {
      qc.invalidateQueries({ queryKey: ['pairing'] })
      setName('')
      setQr(d.link ? await QRCode.toDataURL(d.link, { margin: 1, width: 360, color: { dark: '#17181b', light: '#ffffff' } }) : '')
    },
  })
  const revoke = useMutation({ mutationFn: api.revokeDevice, onSuccess: () => qc.invalidateQueries({ queryKey: ['pairing'] }) })
  const devices = pairing.data?.devices ?? []

  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore"><Smartphone size={18} /></div>
        <div className="flex-1">
          <div className="text-[15px] font-medium">{t('phone.title')}</div>
          <div className="text-[13px] text-ink-3">{t('phone.text')}</div>
        </div>
      </div>

      <div className="mt-4 space-y-2">
        <Access icon={<Home size={16} />} title={t('phone.homeTitle')} text={t('phone.homeText')} on={!!r?.lan.on} busy={toggle.isPending}
          onToggle={() => toggle.mutate({ kind: 'lan', on: !r?.lan.on })}>
          {r?.lan.url && <code className="text-[12px] text-ink-2">{r.lan.url}</code>}
        </Access>
        <Access icon={<Globe size={16} />} title={t('phone.tsTitle')} text={t('phone.tsText')} on={!!r && r.tailscale.state !== 'off'} busy={toggle.isPending}
          onToggle={() => toggle.mutate({ kind: 'tailscale', on: r?.tailscale.state === 'off' })}>
          <TailscaleState s={r?.tailscale} />
        </Access>
        {toggle.error && <p className="text-[13px] text-danger">{toggle.error.message}</p>}
      </div>

      <button type="button" className="mt-3 flex items-center gap-1 text-[12.5px] text-ink-3 hover:text-ink" aria-expanded={showManual} onClick={() => setShowManual(!showManual)}>
        <ChevronDown size={14} className={cn('transition', showManual && 'rotate-180')} /> {t('phone.other')}
      </button>
      {showManual && (
        <input value={manual} onChange={(e) => setManual(e.target.value)} placeholder={t('phone.basePlaceholder')} aria-label={t('phone.base')} className={cn(field, 'mt-2 w-full')} />
      )}

      <form className="mt-4 flex flex-wrap gap-2 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); pair.mutate() }}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('phone.name')} aria-label={t('phone.name')} className={cn(field, 'min-w-[180px] flex-1')} />
        {(people.data ?? []).length > 0 && (
          <select value={whose} onChange={(e) => setWhose(e.target.value)} aria-label={t('pair.whose')} className={field}>
            <option value="">{t('pair.me')}</option>
            {people.data!.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        )}
        <Button type="submit" disabled={!reachable || pair.isPending}>{t('phone.generate')}</Button>
      </form>
      {!reachable && <p className="mt-2 text-[12.5px] text-ink-3">{t('phone.needAccess')}</p>}
      {base && r?.lan.on && <p className="mt-2 text-[12.5px] text-ink-3">{t('phone.both')}</p>}
      {pair.error && <p className="mt-2 text-[13px] text-danger">{pair.error.message}</p>}
      {qr && (
        <div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-line bg-white p-4 sm:flex-row sm:items-start">
          <img src={qr} alt={t('phone.qr')} className="size-44" />
          <div className="text-[13px] text-[#4a4d55]">
            <p className="mb-2">{t('phone.scan')}</p>
            {whose && <p className="mb-2">{t('pair.forPerson', { name: nameOf(whose) })}</p>}
            <p className="mb-3 font-medium text-[#b8243c]">{t('phone.warning')}</p>
            <Button size="sm" onClick={() => setQr('')}>{t('phone.done')}</Button>
          </div>
        </div>
      )}
      {devices.length > 0 && (
        <ul className="mt-4 divide-y divide-line rounded-xl border border-line" aria-label={t('phone.devices')}>
          {devices.map((d) => (
            <li key={d.id} className="flex items-center gap-3 px-3 py-2.5 text-[13px]">
              <Smartphone size={14} className="text-ink-3" />
              <span className="flex-1">{d.name} <span className="text-[12px] text-ink-3">· {nameOf(d.person)}</span></span>
              <span className="text-[12px] text-ink-3">{d.last_seen ? t('phone.seen', { when: relative(d.last_seen) }) : t('phone.unused')}</span>
              <Button size="sm" variant="ghost" aria-label={t('phone.revoke', { name: d.name })} onClick={() => revoke.mutate(d.id)}><Trash2 size={14} /></Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

function Access({ icon, title, text, on, busy, onToggle, children }: { icon: ReactNode; title: string; text: string; on: boolean; busy: boolean; onToggle: () => void; children?: ReactNode }) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-line p-3">
      <div className={cn('mt-0.5', on ? 'text-read' : 'text-ink-3')}>{icon}</div>
      <div className="min-w-0 flex-1">
        <div className="text-[14px] font-medium">{title}</div>
        <div className="text-[12.5px] text-ink-3">{text}</div>
        {on && children && <div className="mt-1.5">{children}</div>}
      </div>
      <Switch on={on} label={title} disabled={busy} onChange={() => onToggle()} className="mt-0.5" />
    </div>
  )
}

function TailscaleState({ s }: { s?: RemoteState['tailscale'] }) {
  const t = useT()
  if (!s) return null
  switch (s.state) {
    case 'starting':
      return <span className="flex items-center gap-1.5 text-[12.5px] text-ink-2"><Loader2 size={13} className="animate-spin" /> {t('phone.tsStarting')}</span>
    case 'needs_login':
      return (
        <div className="space-y-1">
          <a href={s.auth_url} target="_blank" rel="noreferrer" className="inline-flex h-8 items-center rounded-[10px] bg-ink px-3 text-[13px] font-medium text-bg shadow-[var(--shadow-card)] hover:opacity-90">{t('phone.tsLogin')}</a>
          <p className="text-[12px] text-ink-3">{t('phone.tsLoginHint')}</p>
        </div>
      )
    case 'needs_funnel':
      return (
        <div className="space-y-1">
          <p className="text-[12px] text-ink-3">{t('phone.tsFunnelHint')}</p>
          <a href={s.auth_url} target="_blank" rel="noreferrer" className="inline-flex h-8 items-center rounded-[10px] bg-ink px-3 text-[13px] font-medium text-bg shadow-[var(--shadow-card)] hover:opacity-90">{t('phone.tsFunnel')}</a>
        </div>
      )
    case 'running':
      return <code className="break-all text-[12px] text-read">{s.url}</code>
    case 'error':
      return <p className="text-[12.5px] text-danger">{s.error}</p>
  }
  return null
}
