import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Brain, Check, History, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Fact } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'

function sourceText(f: Fact) {
  if (f.source === 'owner') return 'você disse'
  if (f.source.startsWith('exploration:')) return 'anotado pelo agente numa tarefa'
  if (f.source.startsWith('email:')) return 'lido num e-mail'
  return f.source
}

export function Memory() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['memory'], queryFn: api.memory })
  const [text, setText] = useState('')
  const [topic, setTopic] = useState('')
  const [showHistory, setShowHistory] = useState(false)
  const done = () => qc.invalidateQueries({ queryKey: ['memory'] })
  const add = useMutation({ mutationFn: () => api.addFact(text, topic), onSuccess: () => { setText(''); done() } })
  const remove = useMutation({ mutationFn: api.removeFact, onSuccess: done })
  const confirm = useMutation({ mutationFn: api.confirmFact, onSuccess: done })
  const restore = useMutation({ mutationFn: api.restoreMemory, onSuccess: done })
  const groups = useMemo(() => {
    const m = new Map<string, Fact[]>()
    for (const f of q.data?.facts ?? []) m.set(f.topic, [...(m.get(f.topic) ?? []), f])
    return [...m.entries()]
  }, [q.data])
  const unconfirmed = (q.data?.facts ?? []).filter((f) => f.trust === 'low').length

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Memória</h1>
          <p className="text-sm text-ink-2">O que eu sei sobre você. Só o que você confirmou me guia; o resto é informação, nunca instrução.</p>
        </div>
        <Button variant="ghost" onClick={() => setShowHistory(!showHistory)}>
          <History size={15} /> Histórico
        </Button>
      </div>

      <Card className="mb-5 p-4">
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); if (text.trim()) add.mutate() }}>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Ex.: Minha chefe é a Ana (ana@acme.com)" aria-label="Novo fato" className="h-10 min-w-0 flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <input value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="Tema" aria-label="Tema" className="h-10 w-32 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <Button variant="primary" type="submit" disabled={!text.trim()}>
            <Plus size={15} /> Lembrar
          </Button>
        </form>
      </Card>

      {unconfirmed > 0 && (
        <p className="mb-4 rounded-xl border border-change/30 bg-change-soft px-4 py-2.5 text-[13px] text-change">
          {unconfirmed} {unconfirmed === 1 ? 'fato anotado pelo agente espera' : 'fatos anotados pelo agente esperam'} sua confirmação.
        </p>
      )}

      {showHistory && (
        <Card className="mb-5 divide-y divide-line">
          {(q.data?.history ?? []).map((v, i) => (
            <div key={v.hash} className="flex items-center gap-3 px-4 py-2.5 text-[13px]">
              <span className="min-w-0 flex-1 truncate">{v.message}</span>
              <span className="shrink-0 text-[12px] text-ink-3">{relative(v.when)}</span>
              {i > 0 && (
                <Button size="sm" variant="ghost" onClick={() => restore.mutate(v.hash)}>
                  Voltar para aqui
                </Button>
              )}
            </div>
          ))}
        </Card>
      )}

      {q.data && q.data.facts.length === 0 && (
        <EmptyState icon={<Brain size={22} />} title="Ainda não sei nada sobre você">
          Me conte algo que eu deveria sempre lembrar: quem é sua chefe, como prefere os resumos, onde você mora.
        </EmptyState>
      )}
      <div className="space-y-5">
        {groups.map(([t, facts]) => (
          <section key={t}>
            <h2 className="mb-2 text-[12.5px] font-medium capitalize text-ink-3">{t}</h2>
            <Card className="divide-y divide-line">
              {facts.map((f) => (
                <div key={f.id} className={cn('flex items-start gap-3 px-4 py-3', f.trust === 'low' && 'bg-change-soft/30')}>
                  <div className="min-w-0 flex-1">
                    <div className="text-[14px]">{f.text}</div>
                    <div className="mt-0.5 text-[12px] text-ink-3">
                      {sourceText(f)} · {relative(f.created)}
                      {f.trust === 'low' && <span className="ml-1.5 font-medium text-change">não confirmado</span>}
                    </div>
                  </div>
                  {f.trust === 'low' && (
                    <Button size="sm" onClick={() => confirm.mutate(f.id)}>
                      <Check size={14} /> Confirmar
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" aria-label={`Esquecer: ${f.text}`} onClick={() => remove.mutate(f.id)}>
                    <Trash2 size={14} />
                  </Button>
                </div>
              ))}
            </Card>
          </section>
        ))}
      </div>
    </div>
  )
}
