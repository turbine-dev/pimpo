import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Smartphone, Trash2 } from 'lucide-react'
import QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { fill, useT } from '../lib/i18n'
import { Button, Card } from './ui'

// Each phone or computer gets its own link. The link is shown once, as a
// QR code; revoking a device cuts off only that one.
export function PhonePairing() {
  const t = useT()
  const qc = useQueryClient()
  const pairing = useQuery({ queryKey: ['pairing'], queryFn: api.pairing })
  const [base, setBase] = useState('')
  const [name, setName] = useState('')
  const [qr, setQr] = useState('')
  useEffect(() => { if (pairing.data) setBase(pairing.data.base) }, [pairing.data])
  const pair = useMutation({
    mutationFn: () => api.setPairing(base, name.trim() || t('phone.defaultName')),
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
          <div className="text-[13px] text-ink-3">{fill(t('phone.text'), { cmd: <code className="font-mono">tailscale serve 7788</code> })}</div>
        </div>
      </div>
      <form className="mt-4 grid gap-2 sm:grid-cols-[1.4fr_1fr_auto]" onSubmit={(e) => { e.preventDefault(); pair.mutate() }}>
        <input value={base} onChange={(e) => setBase(e.target.value)} placeholder={t('phone.basePlaceholder')} aria-label={t('phone.base')} className="h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('phone.name')} aria-label={t('phone.name')} className="h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <Button type="submit" disabled={!base || pair.isPending}>{t('phone.generate')}</Button>
      </form>
      {pair.error && <p className="mt-2 text-[13px] text-danger">{pair.error.message}</p>}
      {qr && (
        <div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-line bg-white p-4 sm:flex-row sm:items-start">
          <img src={qr} alt={t('phone.qr')} className="size-44" />
          <div className="text-[13px] text-[#4a4d55]">
            <p className="mb-2">{t('phone.scan')}</p>
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
              <span className="flex-1">{d.name}</span>
              <span className="text-[12px] text-ink-3">{d.last_seen ? t('phone.seen', { when: relative(d.last_seen) }) : t('phone.unused')}</span>
              <Button size="sm" variant="ghost" aria-label={t('phone.revoke', { name: d.name })} onClick={() => revoke.mutate(d.id)}><Trash2 size={14} /></Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}
