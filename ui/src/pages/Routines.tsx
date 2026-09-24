import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, Loader2, Plus, Repeat, Sparkles, X } from 'lucide-react'
import { motion } from 'motion/react'
import { useNavigate } from 'react-router-dom'
import { RoutineCard } from '../components/RoutineCard'
import { SinceYesterday } from '../components/SinceYesterday'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Exploration } from '../lib/api'
import { relative } from '../lib/format'

export function Routines({ onNew }: { onNew: () => void }) {
  const nav = useNavigate()
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines })
  const pending = useQuery({ queryKey: ['explorations', 'pending'], queryFn: () => api.explorations('running,ready,compiling') })
  const imported = useQuery({ queryKey: ['explorations', 'imported'], queryFn: () => api.explorations('imported') })
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

      {list.length > 0 && <SinceYesterday />}

      {open.length > 0 && (
        <div className="mb-6 grid gap-3 sm:grid-cols-2">
          {open.map((e) => (
            <ExplorationCard key={e.id} e={e} onOpen={() => nav(`/explorations/${e.id}`)} />
          ))}
        </div>
      )}

      {(imported.data?.length ?? 0) > 0 && <ImportedTasks items={imported.data!} />}

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

function ImportedTasks({ items }: { items: Exploration[] }) {
  const qc = useQueryClient()
  const nav = useNavigate()
  const explore = useMutation({ mutationFn: api.exploreImported, onSuccess: (r) => { qc.invalidateQueries({ queryKey: ['explorations'] }); nav(`/explorations/${r.id}`) } })
  const discard = useMutation({ mutationFn: api.discard, onSuccess: () => qc.invalidateQueries({ queryKey: ['explorations'] }) })
  return (
    <section className="mb-6" aria-label="Tarefas trazidas de outro agente">
      <h2 className="mb-2 flex items-center gap-2 text-[13px] font-medium text-ink-2"><ArrowDownToLine size={14} /> Trazidas de outro agente · explore uma vez e vira rotina</h2>
      <Card className="divide-y divide-line">
        {items.map((e) => (
          <div key={e.id} className="flex items-center gap-3 p-3.5">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[14px]">{e.request.split('\n')[0]}</div>
              <div className="truncate text-[12px] text-ink-3">{e.summary}</div>
            </div>
            <Button size="sm" variant="primary" disabled={explore.isPending} onClick={() => explore.mutate(e.id)}>Explorar</Button>
            <Button size="sm" variant="ghost" aria-label="Descartar" onClick={() => discard.mutate(e.id)}><X size={15} /></Button>
          </div>
        ))}
      </Card>
    </section>
  )
}
