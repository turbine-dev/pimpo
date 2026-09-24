import { Plus, Repeat } from 'lucide-react'
import { RoutineCard, type RoutineSummary } from '../components/RoutineCard'
import { Button, EmptyState } from '../components/ui'

export function Routines({ routines, onNew }: { routines: RoutineSummary[]; onNew?: () => void }) {
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
      {routines.length === 0 ? (
        <EmptyState icon={<Repeat size={22} />} title="Nenhuma rotina ainda" action={<Button variant="primary" onClick={onNew}>Pedir a primeira tarefa</Button>}>
          Peça algo que você faz toda semana. Depois que der certo uma vez, eu faço sozinho.
        </EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {routines.map((r) => (
            <RoutineCard key={r.id} r={r} />
          ))}
        </div>
      )}
    </div>
  )
}
