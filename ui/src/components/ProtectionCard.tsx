import { useQuery } from '@tanstack/react-query'
import { ShieldCheck } from 'lucide-react'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { Card } from './ui'

export function ProtectionCard({ on, toggle }: { on: boolean; toggle: () => void }) {
  const q = useQuery({ queryKey: ['protection'], queryFn: api.protection })
  const p = q.data
  return (
    <Card className="flex items-start gap-4 p-5">
      <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-read-soft text-read"><ShieldCheck size={18} /></div>
      <div className="flex-1">
        <div className="text-[15px] font-medium">Proteção da comunidade</div>
        <p className="text-[13px] text-ink-3">Uma lista assinada de domínios de roubo de dados, skills maliciosas e ações perigosas, revisada e atualizada todo dia. Eu só baixo a lista; nada seu sai daqui.</p>
        {p && <p className="mt-1.5 text-[12.5px] text-ink-2">Versão {p.version} · {p.entries} entradas · {p.blocked} {p.blocked === 1 ? 'bloqueio' : 'bloqueios'} até agora</p>}
      </div>
      <button type="button" role="switch" aria-checked={on} aria-label="Baixar a lista de proteção" onClick={toggle}
        className={cn('mt-1 flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition', on ? 'bg-accent' : 'bg-line-strong')}>
        <span className={cn('size-5 rounded-full bg-white shadow transition', on && 'translate-x-5')} />
      </button>
    </Card>
  )
}
