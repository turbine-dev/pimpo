import { useMutation } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeft, ArrowRight, Brain, CalendarClock, Check, FolderOpen, Puzzle, ScrollText, Send } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card } from '../components/ui'
import { api, type ImportOptions, type MigrationPlan, type MigrationSource } from '../lib/api'
import { cn } from '../lib/cn'

const sources: { id: MigrationSource; name: string; path: string }[] = [
  { id: 'openclaw', name: 'OpenClaw', path: '~/.openclaw' },
  { id: 'hermes', name: 'Hermes Agent', path: '~/.hermes' },
]

const verdicts = {
  works: { label: 'Vira rotina', cls: 'bg-read-soft text-read' },
  partial: { label: 'Em parte', cls: 'bg-change-soft text-change' },
  no: { label: 'Não dá', cls: 'bg-sunken text-ink-3' },
}

export function Import() {
  const nav = useNavigate()
  const [from, setFrom] = useState<MigrationSource>('openclaw')
  const [home, setHome] = useState('')
  const [opts, setOpts] = useState<ImportOptions>({ memories: true, rules: true, tasks: true, secrets: false, trust: false })
  const preview = useMutation({ mutationFn: () => api.migratePreview(from, home.trim()) })
  const apply = useMutation({ mutationFn: () => api.migrateApply(from, home.trim(), opts) })
  const plan = preview.data

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6">
        <h1 className="text-[22px] font-semibold tracking-tight">Trazer de outro agente</h1>
        <p className="mt-1 text-sm text-ink-2">Memórias, tarefas agendadas e regras do OpenClaw ou do Hermes. Primeiro você vê tudo; nada muda até você confirmar.</p>
      </div>

      <AnimatePresence mode="wait">
        {apply.data ? (
          <motion.div key="done" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
            <Card className="p-8 text-center">
              <div className="mx-auto mb-4 grid size-12 place-items-center rounded-full bg-read-soft text-read"><Check size={22} /></div>
              <h2 className="text-lg font-semibold">Pronto</h2>
              <p className="mx-auto mt-2 max-w-md text-sm text-ink-2">
                {apply.data.memories} memórias, {apply.data.rules} regras e {apply.data.tasks} tarefas vieram do {sources.find((s) => s.id === from)?.name}.
                {apply.data.tasks > 0 && ' As tarefas esperam em Rotinas: explore cada uma uma vez e ela passa a rodar sozinha, sem modelo.'}
              </p>
              <div className="mt-6 flex justify-center gap-2">
                {!opts.trust && apply.data.memories > 0 && <Button onClick={() => nav('/memory')}>Revisar memórias</Button>}
                <Button variant="primary" onClick={() => nav('/')}>Ver as tarefas <ArrowRight size={15} /></Button>
              </div>
            </Card>
          </motion.div>
        ) : !plan ? (
          <motion.div key="pick" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}>
            <Card className="p-5">
              <div role="radiogroup" aria-label="De onde" className="grid gap-3 sm:grid-cols-2">
                {sources.map((s) => (
                  <button key={s.id} type="button" role="radio" aria-checked={from === s.id} onClick={() => setFrom(s.id)}
                    className={cn('rounded-xl border p-4 text-left transition', from === s.id ? 'border-accent bg-accent/5 ring-1 ring-accent' : 'border-line hover:border-line-strong')}>
                    <div className="text-[15px] font-medium">{s.name}</div>
                    <div className="mt-0.5 font-mono text-[12px] text-ink-3">{s.path}</div>
                  </button>
                ))}
              </div>
              <label className="mt-4 block text-[13px] text-ink-2">
                Outra pasta (opcional)
                <div className="mt-1.5 flex items-center gap-2 rounded-[10px] border border-line bg-bg px-3 focus-within:border-accent">
                  <FolderOpen size={15} className="text-ink-3" />
                  <input value={home} onChange={(e) => setHome(e.target.value)} placeholder={sources.find((s) => s.id === from)?.path} className="h-10 flex-1 bg-transparent text-sm outline-none" />
                </div>
              </label>
              {preview.error && <p className="mt-3 text-[13px] text-danger">{preview.error.message}</p>}
              <div className="mt-5 flex justify-end">
                <Button variant="primary" onClick={() => preview.mutate()} disabled={preview.isPending}>Ver o que vem <ArrowRight size={15} /></Button>
              </div>
            </Card>
          </motion.div>
        ) : (
          <motion.div key="plan" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} className="space-y-4">
            <Summary plan={plan} />
            {plan.tasks.length > 0 && <Tasks plan={plan} />}
            {plan.skills.length > 0 && <Skills plan={plan} />}
            {plan.warnings.length > 0 && (
              <Card className="space-y-2 p-5">
                {plan.warnings.map((w) => (
                  <p key={w} className="flex gap-2 text-[13px] text-ink-2"><AlertTriangle size={15} className="mt-0.5 shrink-0 text-change" />{w}</p>
                ))}
              </Card>
            )}
            <Card className="p-5">
              <div className="mb-3 text-[15px] font-medium">O que trazer</div>
              <div className="space-y-1">
                <Toggle on={opts.memories} set={(v) => setOpts({ ...opts, memories: v })} title={`Memórias (${plan.memories.length})`} />
                <Toggle on={opts.rules} set={(v) => setOpts({ ...opts, rules: v })} title={`Regras permanentes (${plan.rules.length})`} text={plan.rules.map((r) => r.file).join(', ')} />
                <Toggle on={opts.tasks} set={(v) => setOpts({ ...opts, tasks: v })} title={`Tarefas agendadas (${plan.tasks.length})`} text="Entram como propostas. Nada roda antes de você explorar." />
                <Toggle on={opts.trust} set={(v) => setOpts({ ...opts, trust: v })} title="Confiar no que veio" text="Deixe desligado se o agente anotava sozinho. Fatos não confirmados não guiam as explorações." />
                {(plan.telegram.has_bot || plan.mail.address) && (
                  <Toggle on={opts.secrets} set={(v) => setOpts({ ...opts, secrets: v })} title="Copiar tokens para o cofre" text={[plan.telegram.has_bot && 'bot do Telegram', plan.mail.address && `senha de ${plan.mail.address}`].filter(Boolean).join(' e ')} />
                )}
              </div>
              {apply.error && <p className="mt-3 text-[13px] text-danger">{apply.error.message}</p>}
              <div className="mt-5 flex justify-between">
                <Button variant="ghost" onClick={() => preview.reset()}><ArrowLeft size={15} /> Voltar</Button>
                <Button variant="primary" onClick={() => apply.mutate()} disabled={apply.isPending || !(opts.memories || opts.rules || opts.tasks || opts.secrets)}>Importar</Button>
              </div>
            </Card>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function Summary({ plan }: { plan: MigrationPlan }) {
  const works = plan.skills.filter((s) => s.verdict === 'works').length
  const tiles: { icon: ReactNode; n: number; label: string }[] = [
    { icon: <Brain size={16} />, n: plan.memories.length, label: 'memórias' },
    { icon: <CalendarClock size={16} />, n: plan.tasks.length, label: 'tarefas' },
    { icon: <ScrollText size={16} />, n: plan.rules.length, label: 'regras' },
    { icon: <Puzzle size={16} />, n: works, label: `de ${plan.skills.length} skills viram rotina` },
  ]
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {tiles.map((t) => (
        <Card key={t.label} className="p-4">
          <div className="text-ink-3">{t.icon}</div>
          <div className="mt-2 text-2xl font-semibold tabular-nums">{t.n}</div>
          <div className="text-[12.5px] text-ink-3">{t.label}</div>
        </Card>
      ))}
    </div>
  )
}

function Tasks({ plan }: { plan: MigrationPlan }) {
  return (
    <Card className="divide-y divide-line">
      {plan.tasks.map((t, i) => (
        <div key={i} className="flex items-start gap-3 p-4">
          <CalendarClock size={16} className="mt-0.5 shrink-0 text-explore" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-[14px] font-medium">{t.name}{!t.enabled && <span className="rounded-full bg-sunken px-2 text-[11px] font-normal text-ink-3">pausada</span>}</div>
            <p className="line-clamp-2 text-[13px] text-ink-2">{t.prompt}</p>
            <div className="mt-1 flex flex-wrap gap-x-3 font-mono text-[11.5px] text-ink-3">
              <span>{t.schedule}{t.timezone && ` · ${t.timezone}`}</span>
              {t.deliver && <span className="flex items-center gap-1"><Send size={11} />{t.deliver}</span>}
            </div>
          </div>
        </div>
      ))}
    </Card>
  )
}

function Skills({ plan }: { plan: MigrationPlan }) {
  return (
    <Card className="p-5">
      <div className="text-[15px] font-medium">Skills</div>
      <p className="mb-3 text-[13px] text-ink-3">Rotinas do Vigia leem e agem por capacidades declaradas; não rodam programas. Assim fica cada uma:</p>
      <ul className="space-y-2">
        {plan.skills.map((s) => (
          <li key={s.name} className="flex items-start gap-3">
            <span className={cn('mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', verdicts[s.verdict].cls)}>{verdicts[s.verdict].label}</span>
            <div className="min-w-0 text-[13px]">
              <span className="font-medium">{s.name}</span>
              {s.capabilities.length > 0 && <span className="ml-2 font-mono text-[11.5px] text-ink-3">{s.capabilities.join(' ')}</span>}
              {s.missing.length > 0 && <div className="text-ink-3">{s.missing.join('; ')}</div>}
            </div>
          </li>
        ))}
      </ul>
    </Card>
  )
}

function Toggle({ on, set, title, text }: { on: boolean; set: (v: boolean) => void; title: string; text?: string }) {
  return (
    <button type="button" role="switch" aria-checked={on} onClick={() => set(!on)} className="flex w-full items-start gap-3 rounded-lg p-2 text-left hover:bg-sunken">
      <span className={cn('mt-0.5 flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition', on ? 'bg-accent' : 'bg-line-strong')}>
        <motion.span layout className={cn('size-4 rounded-full bg-white shadow', on && 'ml-auto')} />
      </span>
      <span>
        <span className="block text-[14px]">{title}</span>
        {text && <span className="block text-[12.5px] text-ink-3">{text}</span>}
      </span>
    </button>
  )
}
