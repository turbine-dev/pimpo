import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Smartphone } from 'lucide-react'
import QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { Button, Card } from './ui'

export function PhonePairing() {
  const qc = useQueryClient()
  const pairing = useQuery({ queryKey: ['pairing'], queryFn: api.pairing })
  const [base, setBase] = useState('')
  const [qr, setQr] = useState('')
  const [show, setShow] = useState(false)
  useEffect(() => { if (pairing.data) setBase(pairing.data.base) }, [pairing.data])
  const link = pairing.data?.link ?? ''
  useEffect(() => {
    if (!link || !show) return setQr('')
    QRCode.toDataURL(link, { margin: 1, width: 360, color: { dark: '#17181b', light: '#ffffff' } }).then(setQr)
  }, [link, show])
  const save = useMutation({ mutationFn: () => api.setPairing(base), onSuccess: (d) => { qc.setQueryData(['pairing'], d); setShow(true) } })

  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore"><Smartphone size={18} /></div>
        <div className="flex-1">
          <div className="text-[15px] font-medium">Abrir no celular</div>
          <div className="text-[13px] text-ink-3">O app do celular conversa com este Vigia. Exponha-o com segurança, por exemplo com <code className="font-mono">tailscale serve 7788</code>, e cole o endereço.</div>
        </div>
      </div>
      <form className="mt-4 flex gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
        <input value={base} onChange={(e) => setBase(e.target.value)} placeholder="https://vigia.sua-rede.ts.net" aria-label="Endereço do Vigia fora deste computador" className="h-10 flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <Button type="submit" disabled={!base || save.isPending}>Gerar código</Button>
      </form>
      {save.error && <p className="mt-2 text-[13px] text-danger">{save.error.message}</p>}
      {link && !show && <button type="button" className="mt-3 text-[13px] text-accent underline" onClick={() => setShow(true)}>Mostrar o código</button>}
      {qr && (
        <div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-line bg-white p-4 sm:flex-row sm:items-start">
          <img src={qr} alt="Código QR para parear o celular" className="size-44" />
          <div className="text-[13px] text-[#4a4d55]">
            <p className="mb-2">Leia com a câmera do celular, ou abra o app Vigia e cole o link.</p>
            <p className="mb-3 font-medium text-[#b8243c]">Quem tiver este código entra no seu Vigia. Não mostre a ninguém.</p>
            <Button size="sm" onClick={() => setShow(false)}>Esconder</Button>
          </div>
        </div>
      )}
    </Card>
  )
}
