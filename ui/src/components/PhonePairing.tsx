import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Smartphone, Trash2 } from 'lucide-react'
import QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { Button, Card } from './ui'

// Each phone or computer gets its own link. The link is shown once, as a
// QR code; revoking a device cuts off only that one.
export function PhonePairing() {
  const qc = useQueryClient()
  const pairing = useQuery({ queryKey: ['pairing'], queryFn: api.pairing })
  const [base, setBase] = useState('')
  const [name, setName] = useState('')
  const [qr, setQr] = useState('')
  useEffect(() => { if (pairing.data) setBase(pairing.data.base) }, [pairing.data])
  const pair = useMutation({
    mutationFn: () => api.setPairing(base, name.trim() || 'Celular'),
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
          <div className="text-[15px] font-medium">Abrir no celular</div>
          <div className="text-[13px] text-ink-3">O app do celular conversa com este Vigia. Exponha-o com segurança, por exemplo com <code className="font-mono">tailscale serve 7788</code>, e cole o endereço.</div>
        </div>
      </div>
      <form className="mt-4 grid gap-2 sm:grid-cols-[1.4fr_1fr_auto]" onSubmit={(e) => { e.preventDefault(); pair.mutate() }}>
        <input value={base} onChange={(e) => setBase(e.target.value)} placeholder="https://vigia.sua-rede.ts.net" aria-label="Endereço do Vigia fora deste computador" className="h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nome do aparelho" aria-label="Nome do aparelho" className="h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <Button type="submit" disabled={!base || pair.isPending}>Gerar código</Button>
      </form>
      {pair.error && <p className="mt-2 text-[13px] text-danger">{pair.error.message}</p>}
      {qr && (
        <div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-line bg-white p-4 sm:flex-row sm:items-start">
          <img src={qr} alt="Código QR para parear o celular" className="size-44" />
          <div className="text-[13px] text-[#4a4d55]">
            <p className="mb-2">Leia com a câmera do celular, ou abra o app Vigia e cole o link. Este código só aparece agora.</p>
            <p className="mb-3 font-medium text-[#b8243c]">Quem tiver este código entra no seu Vigia. Não mostre a ninguém.</p>
            <Button size="sm" onClick={() => setQr('')}>Pronto</Button>
          </div>
        </div>
      )}
      {devices.length > 0 && (
        <ul className="mt-4 divide-y divide-line rounded-xl border border-line" aria-label="Aparelhos pareados">
          {devices.map((d) => (
            <li key={d.id} className="flex items-center gap-3 px-3 py-2.5 text-[13px]">
              <Smartphone size={14} className="text-ink-3" />
              <span className="flex-1">{d.name}</span>
              <span className="text-[12px] text-ink-3">{d.last_seen ? `visto ${relative(d.last_seen)}` : 'ainda não usado'}</span>
              <Button size="sm" variant="ghost" aria-label={`Desconectar ${d.name}`} onClick={() => revoke.mutate(d.id)}><Trash2 size={14} /></Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}
