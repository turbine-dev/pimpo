import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Check, Code2, FlaskConical, Loader2, ShieldCheck, Sparkles, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CapabilityChip, RoutineCard } from '../components/RoutineCard'
import { Button, Card, RiskBadge } from '../components/ui'
import { describe } from '../lib/actions'
import { api, type ActionRecord, type VEvent } from '../lib/api'
import { cn } from '../lib/cn'
import { usd } from '../lib/format'

export function ExplorationPage() {
  const { id = '' } = useParams()
  const qc = useQueryClient()
  const nav = useNavigate()
  const q = useQuery({ queryKey: ['exploration', id], queryFn: () => api.exploration(id), refetchInterval: (d) => (d.state.data?.exploration.state === 'running' ? 1500 : false) })
  const compile = useMutation({ mutationFn: () => api.compile(id), onSuccess: () => qc.invalidateQueries({ queryKey: ['routines'] }) })
  const discard = useMutation({ mutationFn: () => api.discard(id), onSuccess: () => nav('/') })

  if (!q.data) return <div className="mx-auto max-w-3xl text-sm text-ink-3">{q.error ? q.error.message : 'Carregando…'}</div>
  const { exploration: e, actions } = q.data
  const running = e.state === 'running'

  return (
    <div className="mx-auto max-w-3xl">
      <Link to="/" className="mb-5 inline-flex items-center gap-1.5 text-[13px] text-ink-3 hover:text-ink">
        <ArrowLeft size={14} /> Rotinas
      </Link>
      <div className="mb-6">
        <div className="mb-2 flex items-center gap-2 text-[12px] font-medium">
          {running ? (
            <span className="flex items-center gap-1.5 text-explore">
              <Loader2 size={13} className="animate-spin" /> Fazendo agora, com você olhando
            </span>
          ) : e.state === 'failed' ? (
            <span className="text-danger">Não consegui terminar</span>
          ) : (
            <span className="text-read">Feito · {usd(e.cost_usd)} com modelo</span>
          )}
        </div>
        <h1 className="text-[21px] font-semibold leading-snug tracking-tight">{e.request}</h1>
      </div>

      <Card className="mb-6 overflow-hidden">
        <div className="border-b border-line px-5 py-3 text-[12.5px] font-medium text-ink-3">O que eu fiz · {actions.length} passo{actions.length === 1 ? '' : 's'}</div>
        <ol className="divide-y divide-line">
          <AnimatePresence initial={false}>
            {[...actions].reverse().map((ev) => (
              <Step key={ev.id} ev={ev} />
            ))}
          </AnimatePresence>
          {running && (
            <li className="flex items-center gap-3 px-5 py-3.5 text-[13px] text-ink-3">
              <span className="size-2 animate-pulse-soft rounded-full bg-explore" /> pensando no próximo passo…
            </li>
          )}
          {!running && actions.length === 0 && <li className="px-5 py-4 text-[13px] text-ink-3">Nenhuma ação registrada.</li>}
        </ol>
      </Card>

      {e.state === 'failed' && <p className="rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger">{e.error}</p>}

      {(e.state === 'ready' || e.state === 'compiling' || e.state === 'done') && (
        <>
          <Card className="mb-6 p-5">
            <div className="mb-2 text-[12.5px] font-medium text-ink-3">Resultado</div>
            <p className="whitespace-pre-wrap text-[14.5px] leading-relaxed">{e.summary}</p>
          </Card>
          <CompileMoment
            state={compile.isPending || e.state === 'compiling' ? 'compiling' : compile.data || e.state === 'done' ? 'done' : 'ready'}
            error={compile.error?.message ?? (e.state === 'ready' ? e.error : undefined)}
            exploreCost={e.cost_usd}
            routine={compile.data}
            routineId={e.routine}
            onCompile={() => compile.mutate()}
            onDiscard={() => discard.mutate()}
          />
        </>
      )}
    </div>
  )
}

function Step({ ev }: { ev: VEvent<ActionRecord> }) {
  const a = ev.data
  return (
    <motion.li layout initial={{ opacity: 0, x: -8 }} animate={{ opacity: 1, x: 0 }} className="flex items-start gap-3 px-5 py-3.5">
      <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', a.error ? 'bg-danger' : a.risk === 'read' ? 'bg-read' : a.risk === 'notify' ? 'bg-read' : a.risk === 'reversible' ? 'bg-change' : 'bg-danger')} />
      <div className="min-w-0 flex-1">
        <div className="text-[13.5px]">{describe(a)}</div>
        {a.error && <div className="mt-0.5 text-[12.5px] text-danger">{a.error}</div>}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {a.dry_run && <span className="rounded-full bg-sunken px-2 py-0.5 text-[11px] text-ink-3">simulado</span>}
        {a.risk !== 'read' && <RiskBadge risk={a.risk} />}
      </div>
    </motion.li>
  )
}

function CompileMoment({ state, error, exploreCost, routine, routineId, onCompile, onDiscard }: {
  state: 'ready' | 'compiling' | 'done'
  error?: string
  exploreCost: number
  routine?: import('../lib/api').RoutineSummary
  routineId?: string
  onCompile: () => void
  onDiscard: () => void
}) {
  const nav = useNavigate()
  return (
    <AnimatePresence mode="wait">
      {state === 'ready' && (
        <motion.div key="ready" exit={{ opacity: 0, scale: 0.98 }}>
          <Card className="relative overflow-hidden p-6">
            <div className="pointer-events-none absolute -right-16 -top-16 size-48 rounded-full bg-accent/10 blur-3xl" />
            <div className="flex items-start gap-4">
              <div className="grid size-11 shrink-0 place-items-center rounded-2xl bg-accent/12 text-accent">
                <Sparkles size={20} />
              </div>
              <div className="flex-1">
                <h2 className="text-[16px] font-semibold tracking-tight">Quer que eu faça isso sozinho, sem gastar com modelo a cada vez?</h2>
                <p className="mt-1 text-[13.5px] text-ink-2">Eu transformo o que acabei de fazer numa rotina: código que você pode ler, com testes e com a lista exata do que ela pode tocar.</p>
                {error && <p className="mt-3 rounded-lg bg-danger-soft px-3 py-2 text-[13px] text-danger">{error}</p>}
                <div className="mt-5 flex flex-wrap gap-2">
                  <Button variant="primary" onClick={onCompile}>
                    <Sparkles size={15} /> Transformar em rotina
                  </Button>
                  <Button variant="ghost" onClick={onDiscard}>
                    <X size={15} /> Descartar
                  </Button>
                </div>
              </div>
            </div>
          </Card>
        </motion.div>
      )}
      {state === 'compiling' && (
        <motion.div key="compiling" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
          <Card className="p-6">
            <div className="flex items-center gap-3 text-[14px] font-medium">
              <Loader2 size={17} className="animate-spin text-accent" /> Escrevendo a rotina e testando…
            </div>
            <div className="mt-4 grid gap-2 text-[13px] text-ink-2 sm:grid-cols-3">
              <Phase icon={<Code2 size={14} />} label="Escrevendo o código" delay={0} />
              <Phase icon={<FlaskConical size={14} />} label="Rodando os testes" delay={1.2} />
              <Phase icon={<ShieldCheck size={14} />} label="Conferindo capacidades" delay={2.4} />
            </div>
          </Card>
        </motion.div>
      )}
      {state === 'done' && (
        <motion.div key="done" initial={{ opacity: 0, y: 12, scale: 0.97 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: 'spring', stiffness: 260, damping: 24 }}>
          <Card className="p-6">
            <div className="mb-4 flex items-center gap-2 text-[14px] font-semibold text-read">
              <Check size={17} /> Virou rotina. A partir de agora eu faço sozinho.
            </div>
            <div className="mb-5 grid grid-cols-2 gap-3">
              <div className="rounded-xl border border-line bg-bg p-4">
                <div className="text-[12px] text-ink-3">Hoje, com modelo</div>
                <div className="mt-1 text-[22px] font-semibold tabular-nums">{usd(exploreCost)}</div>
              </div>
              <div className="rounded-xl border border-read/30 bg-read-soft p-4">
                <div className="text-[12px] text-read">Cada execução da rotina</div>
                <div className="mt-1 text-[22px] font-semibold tabular-nums text-read">~$0,00</div>
              </div>
            </div>
            {routine && (
              <>
                <div className="mb-4 flex flex-wrap gap-1.5">
                  {routine.capabilities.map((c) => (
                    <CapabilityChip key={c} entry={c} />
                  ))}
                </div>
                <div className="max-w-sm">
                  <RoutineCard r={routine} onOpen={() => nav(`/routines/${routine.id}`)} />
                </div>
              </>
            )}
            {!routine && routineId && (
              <Button onClick={() => nav(`/routines/${routineId}`)}>Ver a rotina</Button>
            )}
          </Card>
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function Phase({ icon, label, delay }: { icon: React.ReactNode; label: string; delay: number }) {
  return (
    <motion.div initial={{ opacity: 0.35 }} animate={{ opacity: 1 }} transition={{ delay, duration: 0.4 }} className="flex items-center gap-2 rounded-lg bg-sunken px-3 py-2">
      {icon} {label}
    </motion.div>
  )
}
