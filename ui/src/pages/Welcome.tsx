import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Check, Coins, Mail, MessageCircle, Repeat, ShieldCheck, Sparkles } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Logo } from '../components/Shell'
import { Button, Card } from '../components/ui'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

const presets = [
  { value: 'conservative', title: 'welcome.conservative', text: 'welcome.conservativeText' },
  { value: 'balanced', title: 'welcome.balanced', text: 'welcome.balancedText', recommended: true },
  { value: 'liberal', title: 'welcome.liberal', text: 'welcome.liberalText' },
] as const

export function Welcome({ onFirstTask }: { onFirstTask: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const setup = useQuery({ queryKey: ['setup'], queryFn: api.setup })
  const [step, setStep] = useState(0)
  const [preset, setPreset] = useState<'conservative' | 'balanced' | 'liberal'>('balanced')
  const [budget, setBudget] = useState('1')
  const finish = useMutation({
    mutationFn: async () => {
      await api.preset(preset)
      await api.setBudget(Number(budget) || 0)
      await api.setupDone()
    },
    onSuccess: () => {
      qc.invalidateQueries()
      nav('/')
      onFirstTask()
    },
  })
  const s = setup.data
  const steps = [
    {
      icon: <Sparkles size={20} />,
      title: t('welcome.hello'),
      body: (
        <div className="space-y-4 text-[14.5px] leading-relaxed text-ink-2">
          <p>{t('welcome.intro')}</p>
          <div className="grid gap-3 sm:grid-cols-3">
            {[
              [<Repeat size={16} />, t('nav.routines'), t('welcome.routinesText')],
              [<ShieldCheck size={16} />, t('nav.rules'), t('welcome.rulesText')],
              [<Check size={16} />, t('nav.receipts'), t('welcome.receiptsText')],
            ].map(([icon, title, text]) => (
              <div key={String(title)} className="rounded-xl border border-line bg-bg p-3.5">
                <div className="mb-1.5 flex items-center gap-2 text-[13.5px] font-medium text-ink">
                  {icon} {title}
                </div>
                <div className="text-[12.5px]">{text}</div>
              </div>
            ))}
          </div>
          {s?.demo && <p className="rounded-lg bg-explore-soft px-3 py-2 text-[13px] text-explore">{t('welcome.demo')}</p>}
        </div>
      ),
    },
    {
      icon: <MessageCircle size={20} />,
      title: t('welcome.talk'),
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>{t('welcome.talkText')}</p>
          <Status ok={!!s?.telegram || !!s?.demo} label={s?.demo ? t('welcome.demoMessages') : s?.telegram ? t('welcome.telegramOn') : t('welcome.notConnected')} />
          {!s?.demo && !s?.telegram && <Button onClick={() => nav('/connections')}>{t('welcome.connectTelegram')}</Button>}
        </div>
      ),
    },
    {
      icon: <Mail size={20} />,
      title: t('welcome.read'),
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>{t('welcome.readText')}</p>
          <Status ok={!!s?.mail || !!s?.demo} label={s?.demo ? t('welcome.sampleMail') : s?.mail ? t('welcome.mailOn') : t('welcome.mailOff')} />
          <Status ok={!!s?.calendar || !!s?.demo} label={s?.demo ? t('welcome.sampleCalendar') : s?.calendar ? t('welcome.calendarOn') : t('welcome.calendarOff')} />
          {!s?.demo && !(s?.mail && s?.calendar) && <Button onClick={() => nav('/connections')}>{t('welcome.openConnections')}</Button>}
        </div>
      ),
    },
    {
      icon: <ShieldCheck size={20} />,
      title: t('welcome.autonomy'),
      body: (
        <div className="grid gap-2">
          {presets.map((p) => (
            <button key={p.value} onClick={() => setPreset(p.value)} className={cn('rounded-xl border p-4 text-left transition-colors', preset === p.value ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
              <div className="flex items-center gap-2 text-[14.5px] font-medium">
                {t(p.title)}
                {'recommended' in p && <span className="rounded-full bg-accent/15 px-2 py-0.5 text-[11px] text-accent">{t('welcome.recommended')}</span>}
              </div>
              <div className="mt-1 text-[13px] text-ink-2">{t(p.text)}</div>
            </button>
          ))}
          <p className="pt-1 text-[12.5px] text-ink-3">{t('welcome.later')}</p>
        </div>
      ),
    },
    {
      icon: <Coins size={20} />,
      title: t('welcome.budget'),
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>{t('welcome.budgetText')}</p>
          <div className="flex items-center gap-2">
            <span className="text-ink-3">$</span>
            <input value={budget} onChange={(e) => setBudget(e.target.value)} type="number" min="0" step="0.5" aria-label={t('settings.budgetLabel')} className="h-11 w-32 rounded-[10px] border border-line bg-bg px-3 text-[15px] outline-none focus:border-accent" />
            <span className="text-[13px] text-ink-3">{t('welcome.perDay')}</span>
          </div>
        </div>
      ),
    },
  ]
  const last = step === steps.length - 1
  return (
    <div className="mx-auto flex min-h-[80vh] max-w-2xl flex-col justify-center py-10">
      <div className="mb-8 flex items-center gap-3">
        <Logo size={34} />
        <div className="flex gap-1.5" role="img" aria-label={t('welcome.step', { n: step + 1, total: steps.length })}>
          {steps.map((_, i) => (
            <span key={i} className={cn('h-1.5 w-8 rounded-full transition-colors', i <= step ? 'bg-accent' : 'bg-line')} />
          ))}
        </div>
      </div>
      <AnimatePresence mode="wait">
        <motion.div key={step} initial={{ opacity: 0, x: 16 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -16 }} transition={{ duration: 0.2 }}>
          <Card className="p-7">
            <div className="mb-4 grid size-11 place-items-center rounded-2xl bg-accent/12 text-accent">{steps[step].icon}</div>
            <h1 className="mb-4 text-[21px] font-semibold tracking-tight">{steps[step].title}</h1>
            {steps[step].body}
          </Card>
        </motion.div>
      </AnimatePresence>
      <div className="mt-5 flex items-center justify-between">
        <Button variant="ghost" onClick={() => setStep(Math.max(0, step - 1))} disabled={step === 0}>
          {t('common.back')}
        </Button>
        {last ? (
          <Button variant="primary" onClick={() => finish.mutate()} disabled={finish.isPending}>
            {t('routines.emptyAction')} <ArrowRight size={15} />
          </Button>
        ) : (
          <Button variant="primary" onClick={() => setStep(step + 1)}>
            {t('welcome.continue')} <ArrowRight size={15} />
          </Button>
        )}
      </div>
    </div>
  )
}

function Status({ ok, label }: { ok: boolean; label: string }) {
  return (
    <div className={cn('flex items-center gap-2 text-[13.5px]', ok ? 'text-read' : 'text-ink-3')}>
      <span className={cn('grid size-5 place-items-center rounded-full', ok ? 'bg-read-soft' : 'bg-sunken')}>{ok && <Check size={12} />}</span>
      {label}
    </div>
  )
}
