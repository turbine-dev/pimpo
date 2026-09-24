import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Button, Card } from '../components/ui'
import { api, type Settings as S } from '../lib/api'
import { cn } from '../lib/cn'

const judges: { value: S['judge_backend']; title: string; text: string }[] = [
  { value: 'local', title: 'Modelo local', text: 'Grátis e offline, via Ollama. Menos preciso.' },
  { value: 'jev', title: 'Jev', text: 'Probabilidades calibradas. Precisa da chave da TypeSafe.' },
  { value: 'llm', title: 'Seu modelo', text: 'Usa o mesmo modelo da exploração, em versão barata.' },
]

export function Settings() {
  const qc = useQueryClient()
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const [s, setS] = useState<S>()
  const [budget, setBudget] = useState('')
  useEffect(() => { if (settings.data) setS(settings.data) }, [settings.data])
  useEffect(() => { if (state.data) setBudget(String(state.data.budget.limit)) }, [state.data])
  const save = useMutation({
    mutationFn: async () => {
      if (s) await api.saveSettings(s)
      await api.setBudget(Number(budget))
    },
    onSuccess: () => qc.invalidateQueries(),
  })
  if (!s) return null
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Ajustes</h1>
        <p className="text-sm text-ink-2">Quanto posso gastar, como decido o que é subjetivo, e onde você está.</p>
      </div>
      <Card className="p-5">
        <div className="text-[15px] font-medium">Limite de gasto por dia</div>
        <p className="mb-3 text-[13px] text-ink-3">Conferido antes de cada chamada a um modelo. Rotinas compiladas quase não gastam.</p>
        <div className="flex items-center gap-2">
          <span className="text-ink-3">$</span>
          <input value={budget} onChange={(e) => setBudget(e.target.value)} type="number" min="0" step="0.5" aria-label="Limite diário em dólares" className="h-10 w-32 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <span className="text-[12.5px] text-ink-3">0 = sem limite</span>
        </div>
      </Card>
      <Card className="p-5">
        <div className="mb-3 text-[15px] font-medium">Quem responde os julgamentos</div>
        <div className="grid gap-2 sm:grid-cols-3">
          {judges.map((j) => (
            <button key={j.value} onClick={() => setS({ ...s, judge_backend: j.value })} className={cn('rounded-xl border p-3 text-left transition-colors', s.judge_backend === j.value ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
              <div className="text-[14px] font-medium">{j.title}</div>
              <div className="mt-0.5 text-[12.5px] text-ink-3">{j.text}</div>
            </button>
          ))}
        </div>
      </Card>
      <Card className="grid gap-4 p-5 sm:grid-cols-2">
        <label>
          <span className="mb-1 block text-[12.5px] font-medium text-ink-2">Fuso horário</span>
          <input value={s.zone} onChange={(e) => setS({ ...s, zone: e.target.value })} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        </label>
        <label>
          <span className="mb-1 block text-[12.5px] font-medium text-ink-2">Idioma das mensagens</span>
          <select value={s.locale} onChange={(e) => setS({ ...s, locale: e.target.value })} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent">
            <option value="pt-BR">Português</option>
            <option value="en-US">English</option>
          </select>
        </label>
      </Card>
      {save.error && <p className="text-sm text-danger">{save.error.message}</p>}
      <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{save.isSuccess ? 'Salvo' : 'Salvar'}</Button>
    </div>
  )
}
