import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Brain, Check, History, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button, Card, EmptyState } from '../components/ui'
import { api, type Fact } from '../lib/api'
import { cn } from '../lib/cn'
import { relative } from '../lib/format'
import { tr, useT } from '../lib/i18n'

function sourceText(f: Fact) {
  if (f.source === 'owner') return tr('memory.fromOwner')
  if (f.source.startsWith('exploration:')) return tr('memory.fromExploration')
  if (f.source.startsWith('email:')) return tr('memory.fromEmail')
  if (f.source.startsWith('import:')) return tr('memory.fromImport', { from: f.source.slice(7) })
  return f.source
}

export function Memory() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['memory'], queryFn: api.memory })
  const [text, setText] = useState('')
  const [topic, setTopic] = useState('')
  const people = useQuery({ queryKey: ['people'], queryFn: api.people })
  const house = people.data ?? []
  const [whose, setWhose] = useState('')
  const [filter, setFilter] = useState('all')
  const nameOf = (id?: string) => (!id ? '' : id === 'casa' ? t('memory.house') : house.find((p) => p.id === id)?.name ?? id)
  const [showHistory, setShowHistory] = useState(false)
  const done = () => qc.invalidateQueries({ queryKey: ['memory'] })
  const add = useMutation({ mutationFn: () => api.addFact(text, topic, whose), onSuccess: () => { setText(''); done() } })
  const remove = useMutation({ mutationFn: api.removeFact, onSuccess: done })
  const confirm = useMutation({ mutationFn: api.confirmFact, onSuccess: done })
  const restore = useMutation({ mutationFn: api.restoreMemory, onSuccess: done })
  const groups = useMemo(() => {
    const m = new Map<string, Fact[]>()
    for (const f of q.data?.facts ?? []) {
      if (filter !== 'all' && (f.person ?? '') !== filter) continue
      m.set(f.topic, [...(m.get(f.topic) ?? []), f])
    }
    return [...m.entries()]
  }, [q.data, filter])
  const unconfirmed = (q.data?.facts ?? []).filter((f) => f.trust === 'low').length

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('memory.title')}</h1>
          <p className="text-sm text-ink-2">{t('memory.subtitle')}</p>
        </div>
        <Button variant="ghost" onClick={() => setShowHistory(!showHistory)}>
          <History size={15} /> {t('memory.history')}
        </Button>
      </div>

      <Card className="mb-5 p-4">
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); if (text.trim()) add.mutate() }}>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder={t('memory.placeholder')} aria-label={t('memory.new')} className="h-10 min-w-0 flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <input value={topic} onChange={(e) => setTopic(e.target.value)} placeholder={t('memory.topic')} aria-label={t('memory.topic')} className="h-10 w-32 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {house.length > 1 && (
            <select value={whose} onChange={(e) => setWhose(e.target.value)} aria-label={t('memory.whose')} className="h-10 rounded-[10px] border border-line bg-bg px-2 text-sm">
              <option value="">{t('memory.mine')}</option>
              <option value="casa">{t('memory.household')}</option>
              {house.filter((p) => p.role !== 'owner').map((p) => <option key={p.id} value={p.id}>{t('memory.of', { name: p.name })}</option>)}
            </select>
          )}
          <Button variant="primary" type="submit" disabled={!text.trim()}>
            <Plus size={15} /> {t('memory.remember')}
          </Button>
        </form>
      </Card>

      {house.length > 1 && (
        <div role="tablist" aria-label={t('memory.filter')} className="mb-4 flex flex-wrap gap-1.5">
          {[{ id: 'all', name: t('memory.all') }, { id: '', name: t('memory.you') }, { id: 'casa', name: t('memory.house') }, ...house.filter((p) => p.role !== 'owner')].map((p) => (
            <button key={p.id} role="tab" aria-selected={filter === p.id} onClick={() => setFilter(p.id)}
              className={cn('rounded-full border px-3 py-1 text-[12.5px] transition', filter === p.id ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
              {p.name}
            </button>
          ))}
        </div>
      )}

      {unconfirmed > 0 && (
        <p className="mb-4 rounded-xl border border-change/30 bg-change-soft px-4 py-2.5 text-[13px] text-change">
          {t('memory.unconfirmed', { count: unconfirmed })}
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
                  {t('memory.restore')}
                </Button>
              )}
            </div>
          ))}
        </Card>
      )}

      {q.data && q.data.facts.length === 0 && (
        <EmptyState icon={<Brain size={22} />} title={t('memory.emptyTitle')}>
          {t('memory.emptyText')}
        </EmptyState>
      )}
      <div className="space-y-5">
        {groups.map(([topic, facts]) => (
          <section key={topic}>
            <h2 className="mb-2 text-[12.5px] font-medium capitalize text-ink-3">{topic}</h2>
            <Card className="divide-y divide-line">
              {facts.map((f) => (
                <div key={f.id} className={cn('flex items-start gap-3 px-4 py-3', f.trust === 'low' && 'bg-change-soft/30')}>
                  <div className="min-w-0 flex-1">
                    <div className="text-[14px]">{f.text}</div>
                    <div className="mt-0.5 text-[12px] text-ink-3">
                      {f.person && <span className="mr-1.5 rounded-full bg-explore-soft px-1.5 py-px text-[11px] font-medium text-explore">{nameOf(f.person)}</span>}
                      {sourceText(f)} · {relative(f.created)}
                      {f.trust === 'low' && <span className="ml-1.5 font-medium text-change">{t('memory.notConfirmed')}</span>}
                    </div>
                  </div>
                  {f.trust === 'low' && (
                    <Button size="sm" onClick={() => confirm.mutate(f.id)}>
                      <Check size={14} /> {t('memory.confirm')}
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" aria-label={t('memory.forget', { text: f.text })} onClick={() => remove.mutate(f.id)}>
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
