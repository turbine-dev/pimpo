import { Repeat } from 'lucide-react'
import { RoutineCard } from '../components/RoutineCard'
import type { RoutineSummary } from '../lib/api'
import { Button, Card, EmptyState, RiskBadge } from '../components/ui'
import { useT } from '../lib/i18n'
import { Cat } from '../components/Mascot'

// The component catalog: every building block with realistic data, used to
// review the design and as a visual regression target.
export const sampleRoutines: RoutineSummary[] = [
  { id: 'brief', name: 'Resumo matinal', description: 'Agenda de hoje, e-mails importantes e o tempo, todo dia às 7h.', state: 'active', version: 3, schedule: '0 7 * * *', next_run: new Date(Date.now() + 36e6).toISOString(), runs: ['ok', 'ok', 'ok', 'ok', 'failed', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok'], cost_month_usd: 0.031, capabilities: ['calendar.events', 'gmail.search', 'http.getJSON:api.open-meteo.com', 'telegram.send'] },
  { id: 'triage', name: 'Triagem de newsletters', description: 'Arquiva promoções e newsletters não lidas e conta quantas foram.', state: 'active', version: 1, schedule: '0 18 * * *', next_run: new Date(Date.now() + 9e6).toISOString(), runs: ['ok', 'ok', 'ok', 'ok', 'ok', 'ok'], cost_month_usd: 0.012, capabilities: ['gmail.search', 'gmail.archive', 'telegram.send'] },
  { id: 'bills', name: 'Contas a vencer', description: 'Boletos e faturas que vencem nos próximos 7 dias.', state: 'broken', version: 2, schedule: '0 8 * * *', runs: ['ok', 'ok', 'ok', 'failed', 'failed'], cost_month_usd: 0.004, capabilities: ['gmail.search', 'telegram.send'] },
  { id: 'usd', name: 'Alerta do dólar', description: 'Avisa quando o dólar passar de R$ 5,40.', state: 'paused', version: 1, schedule: '*/30 * * * *', runs: [], cost_month_usd: 0, capabilities: ['http.getJSON:api.exchangerate.example', 'telegram.send'] },
]

export function Design() {
  const t = useT()
  return (
    <div className="mx-auto max-w-6xl space-y-10">
      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-3">Pimpo</h2>
        <div className="flex flex-wrap gap-6">
          {(['idle', 'sleep', 'alert', 'happy', 'worried', 'working'] as const).map((m) => (
            <figure key={m} className="text-center">
              <div className="h-[130px] w-[100px]"><Cat mood={m} petting={false} /></div>
              <figcaption className="mt-1 text-[12px] text-ink-3">{m}</figcaption>
            </figure>
          ))}
          {(['butterfly', 'ball', 'yawn', 'groom'] as const).map((g) => (
            <figure key={g} className="ml-10 text-center">
              <div className="h-[130px] w-[100px]"><Cat mood="idle" petting={false} play={g} /></div>
              <figcaption className="mt-1 text-[12px] text-ink-3">{g}</figcaption>
            </figure>
          ))}
        </div>
      </section>
      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-3">{t('design.cards')}</h2>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {sampleRoutines.map((r) => (
            <RoutineCard key={r.id} r={r} />
          ))}
        </div>
      </section>
      <section className="grid gap-4 md:grid-cols-2">
        <Card className="space-y-3 p-5">
          <h2 className="text-sm font-semibold text-ink-3">{t('design.risk')}</h2>
          <div className="flex flex-wrap gap-2">
            <RiskBadge risk="read" />
            <RiskBadge risk="notify" />
            <RiskBadge risk="reversible" />
            <RiskBadge risk="irreversible" />
          </div>
          <h2 className="pt-2 text-sm font-semibold text-ink-3">{t('design.buttons')}</h2>
          <div className="flex flex-wrap gap-2">
            <Button variant="primary">{t('design.activate')}</Button>
            <Button>{t('design.code')}</Button>
            <Button variant="ghost">{t('design.adjust')}</Button>
            <Button variant="danger">{t('inbox.deny')}</Button>
          </div>
        </Card>
        <EmptyState icon={<Repeat size={22} />} title={t('routines.emptyTitle')} action={<Button variant="primary">{t('common.newTask')}</Button>}>
          {t('routines.emptyText')}
        </EmptyState>
      </section>
    </div>
  )
}
