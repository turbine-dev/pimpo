import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Check, Coins, Mail, MessageCircle, Repeat, ShieldCheck, Sparkles } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Logo } from '../components/Shell'
import { Button, Card } from '../components/ui'
import { api } from '../lib/api'
import { cn } from '../lib/cn'

const presets = [
  { value: 'conservative', title: 'Conservador', text: 'Pergunto antes de qualquer mudança, até arquivar.' },
  { value: 'balanced', title: 'Equilibrado', text: 'Leio e te aviso à vontade; mudanças reversíveis eu faço e deixo o desfazer; o irreversível sempre pergunto.', recommended: true },
  { value: 'liberal', title: 'Liberal', text: 'Só pergunto antes de mandar algo para outras pessoas. Apagar vira lixeira.' },
] as const

export function Welcome({ onFirstTask }: { onFirstTask: () => void }) {
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
      title: 'Oi, eu sou o Vigia.',
      body: (
        <div className="space-y-4 text-[14.5px] leading-relaxed text-ink-2">
          <p>Você me pede algo que faz toda semana. Na primeira vez eu faço com um modelo de linguagem, com você olhando.</p>
          <div className="grid gap-3 sm:grid-cols-3">
            {[
              [<Repeat size={16} />, 'Rotinas', 'O que deu certo vira código com testes, que roda sozinho e quase de graça.'],
              [<ShieldCheck size={16} />, 'Regras', 'Suas regras ficam fora do modelo. Nada irreversível sem você.'],
              [<Check size={16} />, 'Recibos', 'Tudo o que eu fizer fica registrado, com desfazer.'],
            ].map(([icon, title, text]) => (
              <div key={String(title)} className="rounded-xl border border-line bg-bg p-3.5">
                <div className="mb-1.5 flex items-center gap-2 text-[13.5px] font-medium text-ink">
                  {icon} {title}
                </div>
                <div className="text-[12.5px]">{text}</div>
              </div>
            ))}
          </div>
          {s?.demo && <p className="rounded-lg bg-explore-soft px-3 py-2 text-[13px] text-explore">Modo demonstração: uma caixa de e-mail e uma agenda de exemplo, sem contas e sem custo.</p>}
        </div>
      ),
    },
    {
      icon: <MessageCircle size={20} />,
      title: 'Onde eu falo com você',
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>Pelo Telegram eu mando resultados, peço aprovações e aviso quando algo falha. Leva 1 minuto.</p>
          <Status ok={!!s?.telegram || !!s?.demo} label={s?.demo ? 'No demo as mensagens aparecem aqui mesmo' : s?.telegram ? 'Telegram conectado' : 'Ainda não conectado'} />
          {!s?.demo && !s?.telegram && <Button onClick={() => nav('/connections')}>Conectar o Telegram</Button>}
        </div>
      ),
    },
    {
      icon: <Mail size={20} />,
      title: 'O que eu posso ler',
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>Seu e-mail (senha de app, revogável) e sua agenda (link iCal, só leitura). As chaves ficam criptografadas no seu computador.</p>
          <Status ok={!!s?.mail || !!s?.demo} label={s?.demo ? 'E-mail de exemplo' : s?.mail ? 'E-mail conectado' : 'E-mail ainda não conectado'} />
          <Status ok={!!s?.calendar || !!s?.demo} label={s?.demo ? 'Agenda de exemplo' : s?.calendar ? 'Agenda conectada' : 'Agenda ainda não conectada'} />
          {!s?.demo && !(s?.mail && s?.calendar) && <Button onClick={() => nav('/connections')}>Abrir Conexões</Button>}
        </div>
      ),
    },
    {
      icon: <ShieldCheck size={20} />,
      title: 'O que eu posso fazer sem te perguntar?',
      body: (
        <div className="grid gap-2">
          {presets.map((p) => (
            <button key={p.value} onClick={() => setPreset(p.value)} className={cn('rounded-xl border p-4 text-left transition-colors', preset === p.value ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
              <div className="flex items-center gap-2 text-[14.5px] font-medium">
                {p.title}
                {'recommended' in p && <span className="rounded-full bg-accent/15 px-2 py-0.5 text-[11px] text-accent">recomendado</span>}
              </div>
              <div className="mt-1 text-[13px] text-ink-2">{p.text}</div>
            </button>
          ))}
          <p className="pt-1 text-[12.5px] text-ink-3">Você pode mudar e escrever suas próprias regras depois, em Regras.</p>
        </div>
      ),
    },
    {
      icon: <Coins size={20} />,
      title: 'Quanto posso gastar por dia com modelos?',
      body: (
        <div className="space-y-3 text-[14.5px] text-ink-2">
          <p>Conferido antes de cada chamada. Rotinas compiladas quase não gastam; o custo está em aprender tarefas novas.</p>
          <div className="flex items-center gap-2">
            <span className="text-ink-3">$</span>
            <input value={budget} onChange={(e) => setBudget(e.target.value)} type="number" min="0" step="0.5" aria-label="Limite diário em dólares" className="h-11 w-32 rounded-[10px] border border-line bg-bg px-3 text-[15px] outline-none focus:border-accent" />
            <span className="text-[13px] text-ink-3">por dia</span>
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
        <div className="flex gap-1.5" role="img" aria-label={`Passo ${step + 1} de ${steps.length}`}>
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
          Voltar
        </Button>
        {last ? (
          <Button variant="primary" onClick={() => finish.mutate()} disabled={finish.isPending}>
            Pedir a primeira tarefa <ArrowRight size={15} />
          </Button>
        ) : (
          <Button variant="primary" onClick={() => setStep(step + 1)}>
            Continuar <ArrowRight size={15} />
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
