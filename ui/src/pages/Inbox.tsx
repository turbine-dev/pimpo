import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, BellOff, Sparkles } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'
import { api } from '../lib/api'
import { relative } from '../lib/format'

export function Inbox() {
  const nav = useNavigate()
  const qc = useQueryClient()
  const ready = useQuery({ queryKey: ['explorations', 'ready'], queryFn: () => api.explorations('ready') })
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const repair = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'repair'), onSuccess: (r) => r.exploration && nav(`/explorations/${r.exploration}`) })
  const run = useMutation({ mutationFn: (id: string) => api.routineAction(id, 'run'), onSettled: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  const broken = (routines.data ?? []).filter((r) => r.state === 'broken')
  const items = (ready.data?.length ?? 0) + broken.length

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Precisa de você</h1>
      <p className="mb-6 text-sm text-ink-2">O que está esperando uma decisão sua. O mesmo aparece no Telegram.</p>
      {items === 0 && (
        <EmptyState icon={<BellOff size={22} />} title="Nada esperando por você">
          Quando uma rotina falhar ou uma tarefa nova estiver pronta para virar rotina, ela aparece aqui.
        </EmptyState>
      )}
      <div className="space-y-3">
        {broken.map((r) => (
          <Card key={r.id} className="flex flex-wrap items-center gap-4 border-danger/40 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-danger-soft text-danger">
              <AlertTriangle size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{r.name} não rodou</div>
              <div className="text-[12.5px] text-ink-3">Pausei a rotina até você decidir.</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => run.mutate(r.id)}>Rodar de novo</Button>
              <Button size="sm" variant="primary" onClick={() => repair.mutate(r.id)}>Refazer com o agente</Button>
            </div>
          </Card>
        ))}
        {(ready.data ?? []).map((e) => (
          <Card key={e.id} className="flex flex-wrap items-center gap-4 p-4">
            <div className="grid size-10 place-items-center rounded-xl bg-accent/12 text-accent">
              <Sparkles size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px] font-medium">{e.request}</div>
              <div className="text-[12.5px] text-ink-3">Pronto para virar rotina · {relative(e.updated_at)}</div>
            </div>
            <Button size="sm" variant="primary" onClick={() => nav(`/explorations/${e.id}`)}>Revisar</Button>
          </Card>
        ))}
      </div>
    </div>
  )
}
