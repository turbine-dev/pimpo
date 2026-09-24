import { Repeat } from 'lucide-react'
import { RoutineCard, type RoutineSummary } from '../components/RoutineCard'
import { Button, Card, EmptyState, RiskBadge } from '../components/ui'

// The component catalog: every building block with realistic data, used to
// review the design and as a visual regression target.
export const sampleRoutines: RoutineSummary[] = [
  { id: 'brief', name: 'Resumo matinal', description: 'Agenda de hoje, e-mails importantes e o tempo, todo dia às 7h.', state: 'active', next_run: 'amanhã 07:00', runs: ['ok', 'ok', 'ok', 'ok', 'failed', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok'], cost_month_usd: 0.031, capabilities: ['calendar.events', 'gmail.search', 'http.getJSON:api.open-meteo.com', 'telegram.send'] },
  { id: 'triage', name: 'Triagem de newsletters', description: 'Arquiva promoções e newsletters não lidas e conta quantas foram.', state: 'active', next_run: 'hoje 18:00', runs: ['ok', 'ok', 'ok', 'ok', 'ok', 'ok'], cost_month_usd: 0.012, capabilities: ['gmail.search', 'gmail.archive', 'telegram.send'] },
  { id: 'bills', name: 'Contas a vencer', description: 'Boletos e faturas que vencem nos próximos 7 dias.', state: 'broken', next_run: 'pausada', runs: ['ok', 'ok', 'ok', 'failed', 'failed'], cost_month_usd: 0.004, capabilities: ['gmail.search', 'telegram.send'] },
  { id: 'new', name: 'Alerta do dólar', description: 'Avisa quando o dólar passar de R$ 5,40.', state: 'exploring', runs: [], cost_month_usd: 0.21, capabilities: ['http.getJSON:api.exchangerate.example', 'telegram.send'], uses_llm: true },
]

export function Design() {
  return (
    <div className="mx-auto max-w-6xl space-y-10">
      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-3">Cartões de rotina</h2>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {sampleRoutines.map((r) => (
            <RoutineCard key={r.id} r={r} />
          ))}
        </div>
      </section>
      <section className="grid gap-4 md:grid-cols-2">
        <Card className="space-y-3 p-5">
          <h2 className="text-sm font-semibold text-ink-3">Risco</h2>
          <div className="flex flex-wrap gap-2">
            <RiskBadge risk="read" />
            <RiskBadge risk="notify" />
            <RiskBadge risk="reversible" />
            <RiskBadge risk="irreversible" />
          </div>
          <h2 className="pt-2 text-sm font-semibold text-ink-3">Botões</h2>
          <div className="flex flex-wrap gap-2">
            <Button variant="primary">Ativar rotina</Button>
            <Button>Ver código</Button>
            <Button variant="ghost">Ajustar</Button>
            <Button variant="danger">Negar</Button>
          </div>
        </Card>
        <EmptyState icon={<Repeat size={22} />} title="Nenhuma rotina ainda" action={<Button variant="primary">Nova tarefa</Button>}>
          Peça algo que você faz toda semana. Depois que der certo uma vez, eu faço sozinho.
        </EmptyState>
      </section>
    </div>
  )
}
