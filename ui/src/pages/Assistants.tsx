import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, Pencil, Plus, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { ModelChoice } from '../components/ModelChoice'
import { label } from '../components/ModelSetup'
import { capabilityLabel } from '../components/RoutineCard'
import { Button, Card, EmptyState, RiskBadge } from '../components/ui'
import { api, type Assistant } from '../lib/api'
import { useT } from '../lib/i18n'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

function slug(name: string) {
  return name.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40) || 'assistente'
}

export function Assistants() {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['assistants'], queryFn: api.assistants })
  const remove = useMutation({ mutationFn: api.deleteAssistant, onSuccess: () => qc.invalidateQueries({ queryKey: ['assistants'] }) })
  const [editing, setEditing] = useState<Assistant | null>(null)
  const items = list.data ?? []
  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('as.title')}</h1>
          <p className="text-sm text-ink-2">{t('as.subtitle')}</p>
        </div>
        <Button variant="primary" onClick={() => setEditing({ id: '', name: '', emoji: '🤖', instructions: '', capabilities: [] })}><Plus size={15} /> {t('as.new')}</Button>
      </div>
      {list.isSuccess && items.length === 0 ? (
        <EmptyState icon={<Bot size={22} />} title={t('as.emptyTitle')} action={<Button variant="primary" onClick={() => setEditing({ id: '', name: '', emoji: '🤖', instructions: '', capabilities: [] })}><Plus size={15} /> {t('as.new')}</Button>}>{t('as.empty')}</EmptyState>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {items.map((as) => (
            <Card key={as.id} className="flex items-start gap-3 p-4">
              <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-sunken text-[20px]" aria-hidden>{as.emoji || '🤖'}</div>
              <div className="min-w-0 flex-1">
                <div className="text-[14.5px] font-medium">{as.name}</div>
                <p className="line-clamp-2 text-[12.5px] text-ink-3">{as.instructions}</p>
                <p className="mt-1 text-[12px] text-ink-2">{as.capabilities.length ? t('as.count', { count: as.capabilities.length }) : t('as.everything')}</p>
                {!!as.models?.length && <p className="text-[12px] text-ink-3">{t('lim.only', { list: as.models.map(label).join(', ') })}</p>}
              </div>
              <Button size="sm" variant="ghost" aria-label={t('as.edit', { name: as.name })} onClick={() => setEditing(as)}><Pencil size={14} /></Button>
              <Button size="sm" variant="ghost" aria-label={t('as.delete', { name: as.name })} onClick={() => remove.mutate(as.id)}><Trash2 size={14} /></Button>
            </Card>
          ))}
        </div>
      )}
      {editing && <Editor start={editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

function Editor({ start, onClose }: { start: Assistant; onClose: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const caps = useQuery({ queryKey: ['capabilities'], queryFn: api.capabilities })
  const [as, setAs] = useState(start)
  const [all, setAll] = useState(start.id !== '' && start.capabilities.length === 0)
  const [models, setModels] = useState<string[] | null>(start.models?.length ? start.models : null)
  const save = useMutation({
    mutationFn: () => api.saveAssistant({ ...as, id: as.id || slug(as.name), capabilities: all ? [] : as.capabilities, models: models ?? undefined }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['assistants'] }); onClose() },
  })
  const groups = new Map<string, NonNullable<typeof caps.data>>()
  for (const c of caps.data ?? []) {
    const g = c.name.split('.')[0]
    groups.set(g, [...(groups.get(g) ?? []), c])
  }
  const toggle = (name: string) => setAs({ ...as, capabilities: as.capabilities.includes(name) ? as.capabilities.filter((x) => x !== name) : [...as.capabilities, name] })
  const empty = !all && as.capabilities.length === 0
  const noModel = models !== null && models.length === 0

  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[6vh] z-50 max-h-[88vh] w-[min(620px,calc(100vw-32px))] -translate-x-1/2 overflow-y-auto rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <Dialog.Title className="text-[17px] font-semibold tracking-tight">{start.id ? t('as.edit', { name: start.name }) : t('as.new')}</Dialog.Title>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
            <div className="flex gap-3">
              <label className="w-20 space-y-1"><span className="text-[12.5px] text-ink-2">{t('as.emoji')}</span>
                <input className={field + ' text-center text-[18px]'} value={as.emoji} maxLength={4} onChange={(e) => setAs({ ...as, emoji: e.target.value })} /></label>
              <label className="flex-1 space-y-1"><span className="text-[12.5px] text-ink-2">{t('as.name')}</span>
                <input className={field} value={as.name} maxLength={40} onChange={(e) => setAs({ ...as, name: e.target.value })} /></label>
            </div>
            <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{t('as.instructions')}</span>
              <textarea className="min-h-20 w-full rounded-[10px] border border-line bg-bg p-3 text-sm outline-none focus:border-accent" value={as.instructions} maxLength={2000}
                placeholder={t('as.instructionsHint')} onChange={(e) => setAs({ ...as, instructions: e.target.value })} /></label>
            <fieldset className="space-y-2">
              <legend className="mb-1 text-[13px] font-medium text-ink-2">{t('as.tools')}</legend>
              <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={all} onChange={(e) => setAll(e.target.checked)} /> {t('as.allTools')}</label>
              {!all && (
                <div className="max-h-72 space-y-3 overflow-y-auto rounded-xl border border-line p-3">
                  {[...groups.entries()].map(([g, list]) => (
                    <div key={g}>
                      <div className="mb-1 text-[11.5px] font-medium uppercase tracking-wide text-ink-3">{g}</div>
                      {list.map((c) => (
                        <label key={c.name} className="flex items-center gap-2 py-0.5 text-[13px]">
                          <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={as.capabilities.includes(c.name)} onChange={() => toggle(c.name)} />
                          <span className="flex-1">{capabilityLabel(c.name)} <code className="text-[11.5px] text-ink-3">{c.name}</code></span>
                          <RiskBadge risk={c.risk} />
                        </label>
                      ))}
                    </div>
                  ))}
                </div>
              )}
              {empty && <p className="text-[12.5px] text-ink-3">{t('as.none')}</p>}
            </fieldset>
            <ModelChoice value={models} onChange={setModels} legend={t('as.models')} />
            {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
            <Button type="submit" variant="primary" disabled={!as.name.trim() || empty || noModel || save.isPending}>{t('as.save')}</Button>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
