import { useMutation } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeft, ArrowRight, Brain, CalendarClock, Check, FolderOpen, Puzzle, ScrollText, Send } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card } from '../components/ui'
import { api, type ImportOptions, type MigrationPlan, type MigrationSource } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

const sources: { id: MigrationSource; name: string; path: string }[] = [
  { id: 'openclaw', name: 'OpenClaw', path: '~/.openclaw' },
  { id: 'hermes', name: 'Hermes Agent', path: '~/.hermes' },
]

const verdicts = {
  works: { label: 'import.works', cls: 'bg-read-soft text-read' },
  partial: { label: 'import.partial', cls: 'bg-change-soft text-change' },
  no: { label: 'import.no', cls: 'bg-sunken text-ink-3' },
} as const

export function Import() {
  const t = useT()
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
        <h1 className="text-[22px] font-semibold tracking-tight">{t('import.title')}</h1>
        <p className="mt-1 text-sm text-ink-2">{t('import.subtitle')}</p>
      </div>

      <AnimatePresence mode="wait">
        {apply.data ? (
          <motion.div key="done" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}>
            <Card className="p-8 text-center">
              <div className="mx-auto mb-4 grid size-12 place-items-center rounded-full bg-read-soft text-read"><Check size={22} /></div>
              <h2 className="text-lg font-semibold">{t('import.done')}</h2>
              <p className="mx-auto mt-2 max-w-md text-sm text-ink-2">
                {t('import.result', { memories: apply.data.memories, rules: apply.data.rules, tasks: apply.data.tasks, from: sources.find((s) => s.id === from)?.name ?? '' })}
                {apply.data.tasks > 0 && ` ${t('import.resultTasks')}`}
              </p>
              <div className="mt-6 flex justify-center gap-2">
                {!opts.trust && apply.data.memories > 0 && <Button onClick={() => nav('/memory')}>{t('import.reviewMemories')}</Button>}
                <Button variant="primary" onClick={() => nav('/')}>{t('import.seeTasks')} <ArrowRight size={15} /></Button>
              </div>
            </Card>
          </motion.div>
        ) : !plan ? (
          <motion.div key="pick" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}>
            <Card className="p-5">
              <div role="radiogroup" aria-label={t('import.from')} className="grid gap-3 sm:grid-cols-2">
                {sources.map((s) => (
                  <button key={s.id} type="button" role="radio" aria-checked={from === s.id} onClick={() => setFrom(s.id)}
                    className={cn('rounded-xl border p-4 text-left transition', from === s.id ? 'border-accent bg-accent/5 ring-1 ring-accent' : 'border-line hover:border-line-strong')}>
                    <div className="text-[15px] font-medium">{s.name}</div>
                    <div className="mt-0.5 font-mono text-[12px] text-ink-3">{s.path}</div>
                  </button>
                ))}
              </div>
              <label className="mt-4 block text-[13px] text-ink-2">
                {t('import.folder')}
                <div className="mt-1.5 flex items-center gap-2 rounded-[10px] border border-line bg-bg px-3 focus-within:border-accent">
                  <FolderOpen size={15} className="text-ink-3" />
                  <input value={home} onChange={(e) => setHome(e.target.value)} placeholder={sources.find((s) => s.id === from)?.path} className="h-10 flex-1 bg-transparent text-sm outline-none" />
                </div>
              </label>
              {preview.error && <p className="mt-3 text-[13px] text-danger">{preview.error.message}</p>}
              <div className="mt-5 flex justify-end">
                <Button variant="primary" onClick={() => preview.mutate()} disabled={preview.isPending}>{t('import.preview')} <ArrowRight size={15} /></Button>
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
              <div className="mb-3 text-[15px] font-medium">{t('import.what')}</div>
              <div className="space-y-1">
                <Toggle on={opts.memories} set={(v) => setOpts({ ...opts, memories: v })} title={t('import.memories', { n: plan.memories.length })} />
                <Toggle on={opts.rules} set={(v) => setOpts({ ...opts, rules: v })} title={t('import.rules', { n: plan.rules.length })} text={plan.rules.map((r) => r.file).join(', ')} />
                <Toggle on={opts.tasks} set={(v) => setOpts({ ...opts, tasks: v })} title={t('import.tasks', { n: plan.tasks.length })} text={t('import.tasksText')} />
                <Toggle on={opts.trust} set={(v) => setOpts({ ...opts, trust: v })} title={t('import.trust')} text={t('import.trustText')} />
                {(plan.telegram.has_bot || plan.mail.address) && (
                  <Toggle on={opts.secrets} set={(v) => setOpts({ ...opts, secrets: v })} title={t('import.secrets')} text={[plan.telegram.has_bot && t('import.telegramBot'), plan.mail.address && t('import.mailPassword', { address: plan.mail.address })].filter(Boolean).join(` ${t('common.and')} `)} />
                )}
              </div>
              {apply.error && <p className="mt-3 text-[13px] text-danger">{apply.error.message}</p>}
              <div className="mt-5 flex justify-between">
                <Button variant="ghost" onClick={() => preview.reset()}><ArrowLeft size={15} /> {t('common.back')}</Button>
                <Button variant="primary" onClick={() => apply.mutate()} disabled={apply.isPending || !(opts.memories || opts.rules || opts.tasks || opts.secrets)}>{t('import.import')}</Button>
              </div>
            </Card>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function Summary({ plan }: { plan: MigrationPlan }) {
  const t = useT()
  const works = plan.skills.filter((s) => s.verdict === 'works').length
  const tiles: { icon: ReactNode; n: number; label: string }[] = [
    { icon: <Brain size={16} />, n: plan.memories.length, label: t('import.tileMemories') },
    { icon: <CalendarClock size={16} />, n: plan.tasks.length, label: t('import.tileTasks') },
    { icon: <ScrollText size={16} />, n: plan.rules.length, label: t('import.tileRules') },
    { icon: <Puzzle size={16} />, n: works, label: t('import.tileSkills', { total: plan.skills.length }) },
  ]
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {tiles.map((tile) => (
        <Card key={tile.label} className="p-4">
          <div className="text-ink-3">{tile.icon}</div>
          <div className="mt-2 text-2xl font-semibold tabular-nums">{tile.n}</div>
          <div className="text-[12.5px] text-ink-3">{tile.label}</div>
        </Card>
      ))}
    </div>
  )
}

function Tasks({ plan }: { plan: MigrationPlan }) {
  const t = useT()
  return (
    <Card className="divide-y divide-line">
      {plan.tasks.map((task, i) => (
        <div key={i} className="flex items-start gap-3 p-4">
          <CalendarClock size={16} className="mt-0.5 shrink-0 text-explore" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-[14px] font-medium">{task.name}{!task.enabled && <span className="rounded-full bg-sunken px-2 text-[11px] font-normal text-ink-3">{t('import.paused')}</span>}</div>
            <p className="line-clamp-2 text-[13px] text-ink-2">{task.prompt}</p>
            <div className="mt-1 flex flex-wrap gap-x-3 font-mono text-[11.5px] text-ink-3">
              <span>{task.schedule}{task.timezone && ` · ${task.timezone}`}</span>
              {task.deliver && <span className="flex items-center gap-1"><Send size={11} />{task.deliver}</span>}
            </div>
          </div>
        </div>
      ))}
    </Card>
  )
}

function Skills({ plan }: { plan: MigrationPlan }) {
  const t = useT()
  return (
    <Card className="p-5">
      <div className="text-[15px] font-medium">{t('import.skills')}</div>
      <p className="mb-3 text-[13px] text-ink-3">{t('import.skillsText')}</p>
      <ul className="space-y-2">
        {plan.skills.map((s) => (
          <li key={s.name} className="flex items-start gap-3">
            <span className={cn('mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', verdicts[s.verdict].cls)}>{t(verdicts[s.verdict].label)}</span>
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
