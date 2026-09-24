import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FlaskConical, Plus, ShieldCheck, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { capabilityLabel } from '../components/RoutineCard'
import { Button, Card } from '../components/ui'
import { api, type Rule } from '../lib/api'
import { cn } from '../lib/cn'

const verdictText: Record<Rule['then'], { label: string; cls: string }> = {
  allow: { label: 'Pode fazer', cls: 'bg-read-soft text-read' },
  reversible: { label: 'Faz de forma reversível', cls: 'bg-change-soft text-change' },
  ask: { label: 'Pergunta antes', cls: 'bg-change-soft text-change' },
  block: { label: 'Nunca', cls: 'bg-danger-soft text-danger' },
}

const riskText: Record<string, string> = { read: 'leituras', notify: 'avisos para você', reversible: 'mudanças reversíveis', irreversible: 'coisas irreversíveis' }

export function describeRule(r: Rule) {
  const parts: string[] = []
  if (r.when.capabilities?.length) parts.push(r.when.capabilities.map(capabilityLabel).join(', '))
  if (r.when.min_risk) parts.push(`${riskText[r.when.min_risk] ?? r.when.min_risk} ou mais`)
  if (r.when.source) parts.push(`em ${r.when.source.replace('routine:', 'rotina ')}`)
  if (r.when.hosts?.length) parts.push(`sites ${r.when.hosts.join(', ')}`)
  if (r.when.args_contain?.length) parts.push(`quando menciona ${r.when.args_contain.map((s) => `“${s}”`).join(' ou ')}`)
  return parts.length ? parts.join(' · ') : 'qualquer ação'
}

export function Rules() {
  const qc = useQueryClient()
  const rules = useQuery({ queryKey: ['rules'], queryFn: api.rules })
  const [text, setText] = useState('')
  const compile = useMutation({ mutationFn: () => api.compileRule(text) })
  const test = useMutation({ mutationFn: (r: Rule) => api.testRule(r) })
  const save = useMutation({
    mutationFn: (list: Rule[]) => api.saveRules(list),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rules'] })
      compile.reset()
      test.reset()
      setText('')
    },
  })
  const list = rules.data ?? []
  const draft = compile.data?.rule

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Regras</h1>
      <p className="mb-6 text-sm text-ink-2">Suas regras ficam fora do modelo: são conferidas antes de cada ação e nenhum agente consegue esquecê-las ou mudá-las.</p>

      <Card className="mb-6 p-5">
        <form onSubmit={(e) => { e.preventDefault(); if (text.trim()) compile.mutate() }} className="flex gap-2">
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Ex.: Nunca apague e-mail sem me perguntar" aria-label="Nova regra" className="h-10 flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <Button variant="primary" type="submit" disabled={!text.trim() || compile.isPending}>
            <Plus size={15} /> {compile.isPending ? 'Entendendo…' : 'Criar'}
          </Button>
        </form>
        {compile.error && <p className="mt-3 text-[13px] text-danger">{compile.error.message}</p>}
        {draft && (
          <div className="mt-4 rounded-xl border border-accent/30 bg-accent/5 p-4">
            <div className="text-[12.5px] font-medium text-ink-3">Entendi assim — confira antes de salvar:</div>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-[14px]">
              <span className={cn('rounded-full px-2.5 py-0.5 text-[12.5px] font-medium', verdictText[draft.then].cls)}>{verdictText[draft.then].label}</span>
              <span>{describeRule(draft)}</span>
            </div>
            {compile.data?.summary && <p className="mt-2 text-[13px] text-ink-2">{compile.data.summary}</p>}
            {test.data && (
              <p className="mt-3 text-[13px] text-ink-2">
                {test.data.matches.length === 0 ? 'Na última semana esta regra não teria mudado nada.' : `Na última semana esta regra teria se aplicado a ${test.data.matches.length} ${test.data.matches.length === 1 ? 'ação' : 'ações'}.`}
              </p>
            )}
            <div className="mt-4 flex flex-wrap gap-2">
              <Button variant="primary" onClick={() => save.mutate([...list, draft])}>
                <ShieldCheck size={15} /> Salvar regra
              </Button>
              <Button onClick={() => test.mutate(draft)} disabled={test.isPending}>
                <FlaskConical size={15} /> Testar na última semana
              </Button>
              <Button variant="ghost" onClick={() => compile.reset()}>Cancelar</Button>
            </div>
          </div>
        )}
      </Card>

      <div className="space-y-2">
        {list.map((r) => (
          <Card key={r.id} className={cn('flex items-start gap-4 p-4', r.off && 'opacity-50')}>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-medium">{r.text}</div>
              <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[12.5px] text-ink-3">
                <span className={cn('rounded-full px-2 py-0.5 font-medium', verdictText[r.then].cls)}>{verdictText[r.then].label}</span>
                {describeRule(r)}
              </div>
            </div>
            <div className="flex shrink-0 gap-1">
              <Button size="sm" variant="ghost" onClick={() => save.mutate(list.map((x) => (x.id === r.id ? { ...x, off: !x.off } : x)))}>{r.off ? 'Ligar' : 'Pausar'}</Button>
              <Button size="sm" variant="ghost" aria-label={`Remover ${r.text}`} onClick={() => save.mutate(list.filter((x) => x.id !== r.id))}>
                <Trash2 size={14} />
              </Button>
            </div>
          </Card>
        ))}
      </div>
      <p className="mt-4 text-[12.5px] text-ink-3">Sem regra específica: leituras e avisos para você passam; mudanças reversíveis passam e ficam nos recibos com desfazer; o irreversível pergunta. Quando várias regras se aplicam, vale a mais rígida — e “Nunca” sempre vence.</p>
    </div>
  )
}
