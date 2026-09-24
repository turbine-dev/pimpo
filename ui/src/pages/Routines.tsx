import { useQuery } from '@tanstack/react-query'
import { Loader2, Plus, Repeat, Sparkles } from 'lucide-react'
import { motion } from 'motion/react'
import { useNavigate } from 'react-router-dom'
import { RoutineCard } from '../components/RoutineCard'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Exploration } from '../lib/api'
import { relative } from '../lib/format'

export function Routines({ onNew }: { onNew: () => void }) {
  const nav = useNavigate()
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const pending = useQuery({ queryKey: ['explorations', 'pending'], queryFn: () => api.explorations('running,ready,compiling') })
  const list = routines.data ?? []
  const open = pending.data ?? []

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">Rotinas</h1>
          <p className="mt-1 text-sm text-ink-2">O que eu faço sozinho, sem gastar com modelo a cada vez.</p>
        </div>
        <Button variant="primary" onClick={onNew}>
          <Plus size={16} /> Nova tarefa
        </Button>
      </div>

      {open.length > 0 && (
        <div className="mb-6 grid gap-3 sm:grid-cols-2">
          {open.map((e) => (
            <ExplorationCard key={e.id} e={e} onOpen={() => nav(`/explorations/${e.id}`)} />
          ))}
        </div>
      )}

      {routines.isPending ? null : list.length === 0 ? (
        <EmptyState icon={<Repeat size={22} />} title="Nenhuma rotina ainda" action={<Button variant="primary" onClick={onNew}>Pedir a primeira tarefa</Button>}>
          Peça algo que você faz toda semana. Depois que der certo uma vez, eu faço sozinho.
        </EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {list.map((r) => (
            <RoutineCard key={r.id} r={r} onOpen={() => nav(`/routines/${r.id}`)} />
          ))}
        </div>
      )}
    </div>
  )
}

function ExplorationCard({ e, onOpen }: { e: Exploration; onOpen: () => void }) {
  const running = e.state === 'running' || e.state === 'compiling'
  return (
    <motion.div initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}>
      <Card role="button" tabIndex={0} onClick={onOpen} onKeyDown={(k) => k.key === 'Enter' && onOpen()} className="flex cursor-pointer items-center gap-4 border-dashed border-explore/50 p-4 hover:border-explore">
        <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore">{running ? <Loader2 size={18} className="animate-spin" /> : <Sparkles size={18} />}</div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[14px] font-medium">{e.request}</div>
          <div className="text-[12.5px] text-ink-3">{e.state === 'running' ? 'Fazendo agora…' : e.state === 'compiling' ? 'Virando rotina…' : `Pronto para virar rotina · ${relative(e.updated_at)}`}</div>
        </div>
      </Card>
    </motion.div>
  )
}
