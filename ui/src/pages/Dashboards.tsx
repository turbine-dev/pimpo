import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Home, LayoutDashboard, Loader2, Pencil, Plus, Settings2, Trash2, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type PointerEvent as RPointerEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, type Dashboard, type LayoutItem, type WidgetKind, type WidgetView } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { Button } from '../components/ui'
import { DragHandle, WidgetBody, WidgetCard } from '../components/widgets/Widget'

const COLS = 12
const ROW = 72 // px per grid row on a computer
const GAP = 16

// The size a widget of each kind starts with.
const startSize: Record<WidgetKind, [number, number]> = {
  metric: [3, 3], progress: [3, 3], status: [4, 3], text: [4, 3], list: [4, 4], table: [6, 4], chart: [6, 4],
}

const EMOJIS = ['🏠', '💰', '💼', '📈', '🛒', '📅', '🏃', '📬', '🧾', '🌤️', '🎯', '🔔', '🐱', '⭐️', '🧪', '🚗']

const overlaps = (a: LayoutItem, b: LayoutItem) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h

// settle puts the moved widget where it was dropped and lets the others
// move down out of its way, then floats everything up into free space.
function settle(items: LayoutItem[], moved?: string): LayoutItem[] {
  const out = items.map((i) => ({ ...i }))
  const order = [...out].sort((a, b) => (a.id === moved ? -1 : b.id === moved ? 1 : a.y - b.y || a.x - b.x))
  const placed: LayoutItem[] = []
  for (const it of order) {
    if (it.id !== moved) {
      while (it.y > 0 && !placed.some((p) => overlaps(p, { ...it, y: it.y - 1 }))) it.y--
    }
    while (placed.some((p) => overlaps(p, it))) it.y++
    placed.push(it)
  }
  return out
}

// useWide says the board has room for the grid, by its own width: below
// that the widgets stack.
function useWide() {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  const [wide, setWide] = useState(true)
  useEffect(() => {
    if (!node || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(([e]) => setWide(e.contentRect.width >= 700))
    ro.observe(node)
    return () => ro.disconnect()
  }, [node])
  return [wide, setNode] as const
}

export function Dashboards() {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const { id } = useParams()
  const list = useQuery({ queryKey: ['dashboards'], queryFn: api.dashboards })
  const boards = useMemo(() => [...(list.data ?? [])].sort((a, b) => Number(b.mine) - Number(a.mine) || a.position - b.position), [list.data])
  const board = boards.find((d) => d.id === id) ?? boards[0]
  const [editing, setEditing] = useState(false)
  const [adding, setAdding] = useState(false)
  const [naming, setNaming] = useState<null | 'new' | 'rename'>(null)
  const done = () => qc.invalidateQueries({ queryKey: ['dashboards'] })
  const create = useMutation({ mutationFn: (v: { name: string; emoji: string }) => api.createDashboard(v.name, v.emoji), onSuccess: (d) => { done(); nav(`/dashboards/${d.id}`); setNaming(null) } })
  const save = useMutation({ mutationFn: (v: Partial<Dashboard>) => api.saveDashboard(board!.id, v), onSuccess: done })
  const remove = useMutation({ mutationFn: () => api.deleteDashboard(board!.id), onSuccess: () => { done(); nav('/dashboards') } })

  useEffect(() => { setEditing(false) }, [board?.id])

  return (
    <div className="mx-auto max-w-[1400px]">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h1 className="flex items-center gap-2 text-[22px] font-semibold tracking-tight"><LayoutDashboard size={20} /> {t('dash.title')}</h1>
        {board?.mine && (
          <div className="flex items-center gap-2">
            {editing && <Button size="sm" onClick={() => setAdding(true)}><Plus size={14} /> {t('dash.add')}</Button>}
            <Button size="sm" variant={editing ? 'primary' : 'secondary'} onClick={() => setEditing(!editing)}>
              {editing ? <><Check size={14} /> {t('dash.done')}</> : <><Pencil size={14} /> {t('dash.edit')}</>}
            </Button>
          </div>
        )}
      </div>

      <div className="mb-5 flex items-center gap-1 border-b border-line pb-px">
      <div role="tablist" aria-label={t('dash.tabs')} className="flex min-w-0 items-center gap-1 overflow-x-auto">
        {boards.map((d) => (
          <button key={d.id} role="tab" aria-selected={d.id === board?.id} onClick={() => nav(`/dashboards/${d.id}`)}
            className={cn('-mb-px flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2 text-[14px] transition',
              d.id === board?.id ? 'border-ink font-semibold text-ink' : 'border-transparent text-ink-3 hover:text-ink')}>
            <span aria-hidden>{d.emoji || '📊'}</span>{d.name}
            {d.shared && <Home size={12} className="text-ink-3" aria-label={t('dash.shared')} />}
          </button>
        ))}
      </div>
        <button type="button" onClick={() => setNaming('new')} aria-label={t('dash.new')} title={t('dash.new')}
          className="ml-1 grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken hover:text-ink"><Plus size={16} /></button>
      </div>

      {naming && (
        <NameDialog
          title={naming === 'new' ? t('dash.new') : t('dash.rename')}
          initial={naming === 'rename' && board ? { name: board.name, emoji: board.emoji } : { name: '', emoji: '📈' }}
          busy={create.isPending || save.isPending}
          onCancel={() => setNaming(null)}
          onSave={(v) => (naming === 'new' ? create.mutate(v) : save.mutate(v, { onSuccess: () => setNaming(null) }))}
        />
      )}

      {editing && board?.mine && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border border-line bg-surface p-2.5 text-[13px]">
          <Settings2 size={15} className="ml-1 text-ink-3" />
          <button type="button" className="rounded-lg px-2.5 py-1.5 hover:bg-sunken" onClick={() => setNaming('rename')}>{t('dash.rename')}</button>
          <label className="flex items-center gap-2 rounded-lg px-2.5 py-1.5 hover:bg-sunken">
            <input type="checkbox" checked={board.shared} onChange={(e) => save.mutate({ shared: e.target.checked })} />
            {t('dash.shareWithHouse')}
          </label>
          <span className="flex-1" />
          <button type="button" className="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-danger hover:bg-danger-soft"
            onClick={() => { if (window.confirm(t('dash.deleteSure', { name: board.name }))) remove.mutate() }}>
            <Trash2 size={14} /> {t('dash.delete')}
          </button>
        </div>
      )}

      {!board ? (
        <div className="grid h-64 place-items-center"><Loader2 className="animate-spin text-ink-3" /></div>
      ) : (
        <Board board={board} editing={editing && board.mine} onAdd={() => setAdding(true)} onLayout={(layout) => save.mutate({ layout })} />
      )}

      {adding && board && (
        <AddWidget taken={board.layout.map((l) => l.id)} onClose={() => setAdding(false)}
          onPick={(w) => {
            const [cw, ch] = startSize[w.kind] ?? [4, 3]
            const y = board.layout.reduce((m, l) => Math.max(m, l.y + l.h), 0)
            save.mutate({ layout: settle([...board.layout, { id: w.id, x: 0, y, w: cw, h: ch }]) })
          }} />
      )}
    </div>
  )
}

function NameDialog({ title, initial, busy, onCancel, onSave }: { title: string; initial: { name: string; emoji: string }; busy: boolean; onCancel: () => void; onSave: (v: { name: string; emoji: string }) => void }) {
  const t = useT()
  const [name, setName] = useState(initial.name)
  const [emoji, setEmoji] = useState(initial.emoji)
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-label={title} onClick={onCancel}>
      <form className="w-full max-w-sm rounded-2xl border border-line bg-raised p-5 shadow-[var(--shadow-pop)]" onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => { e.preventDefault(); if (name.trim()) onSave({ name: name.trim(), emoji }) }}>
        <h2 className="mb-3 text-[16px] font-semibold">{title}</h2>
        <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={t('dash.namePlaceholder')} aria-label={t('dash.name')}
          className="mb-3 h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        <div className="mb-4 grid grid-cols-8 gap-1" role="radiogroup" aria-label={t('dash.emoji')}>
          {EMOJIS.map((e) => (
            <button key={e} type="button" role="radio" aria-checked={emoji === e} onClick={() => setEmoji(e)}
              className={cn('grid aspect-square place-items-center rounded-lg text-[18px]', emoji === e ? 'bg-ink/10 ring-2 ring-ink' : 'hover:bg-sunken')}>{e}</button>
          ))}
        </div>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onCancel}>{t('dash.cancel')}</Button>
          <Button type="submit" variant="primary" disabled={!name.trim() || busy}>{busy && <Loader2 size={14} className="animate-spin" />} {t('dash.save')}</Button>
        </div>
      </form>
    </div>
  )
}

function Board({ board, editing, onLayout, onAdd }: { board: Dashboard; editing: boolean; onLayout: (l: LayoutItem[]) => void; onAdd: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [wide, outer] = useWide()
  const widgets = useQuery({ queryKey: ['dashboard-widgets', board.id], queryFn: () => api.dashboardWidgets(board.id), refetchInterval: 60_000 })
  const [layout, setLayout] = useState<LayoutItem[]>(board.layout)
  useEffect(() => { setLayout(board.layout) }, [board.layout])
  const grid = useRef<HTMLDivElement>(null)
  const [refreshing, setRefreshing] = useState<string | null>(null)

  const commit = (next: LayoutItem[], moved?: string) => {
    const s = settle(next, moved)
    setLayout(s)
    onLayout(s)
  }

  // Dragging and resizing work in grid cells, from where the pointer is.
  const start = (e: RPointerEvent, item: LayoutItem, mode: 'move' | 'size') => {
    if (!grid.current) return
    e.preventDefault()
    const box = grid.current.getBoundingClientRect()
    const colW = (box.width - GAP * (COLS - 1)) / COLS
    const x0 = e.clientX
    const y0 = e.clientY
    let current = item
    const onMove = (ev: PointerEvent) => {
      const dx = Math.round((ev.clientX - x0) / (colW + GAP))
      const dy = Math.round((ev.clientY - y0) / (ROW + GAP))
      current = mode === 'move'
        ? { ...item, x: Math.min(Math.max(item.x + dx, 0), COLS - item.w), y: Math.max(item.y + dy, 0) }
        : { ...item, w: Math.min(Math.max(item.w + dx, 2), COLS - item.x), h: Math.min(Math.max(item.h + dy, 2), 8) }
      setLayout((l) => settle(l.map((i) => (i.id === item.id ? current : i)), item.id))
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      commit(layout.map((i) => (i.id === item.id ? current : i)), item.id)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  const refresh = async (id: string) => {
    setRefreshing(id)
    try {
      await api.refreshWidget(id)
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && window.confirm(t('widget.refreshCost'))) await api.refreshWidget(id, true)
      else if (!(e instanceof ApiError && e.status === 409)) window.alert(e instanceof Error ? e.message : String(e))
    } finally {
      setRefreshing(null)
      qc.invalidateQueries({ queryKey: ['dashboard-widgets', board.id] })
    }
  }
  const share = async (id: string, shared: boolean) => {
    await api.shareWidget(id, shared)
    qc.invalidateQueries({ queryKey: ['dashboard-widgets', board.id] })
  }

  if (layout.length === 0) {
    return (
      <div className="grid place-items-center rounded-[24px] border border-dashed border-line-strong bg-surface/50 px-6 py-20 text-center">
        <div className="mb-3 grid size-14 place-items-center rounded-2xl bg-explore-soft text-explore"><LayoutDashboard size={26} /></div>
        <h2 className="text-[18px] font-semibold">{t('dash.emptyTitle')}</h2>
        <p className="mt-1 max-w-md text-[14px] text-ink-2">{t('dash.emptyText')}</p>
        {board.mine && <Button className="mt-5" variant="primary" onClick={onAdd}><Plus size={15} /> {t('dash.add')}</Button>}
      </div>
    )
  }

  const items = wide ? layout : [...layout].sort((a, b) => a.y - b.y || a.x - b.x)
  const rows = layout.reduce((m, l) => Math.max(m, l.y + l.h), 0)
  return (
    <div ref={outer}>
    <div ref={grid} className={cn(wide ? 'grid' : 'flex flex-col gap-4', editing && wide && 'rounded-2xl bg-[linear-gradient(to_right,var(--color-line)_1px,transparent_1px)] bg-[length:calc((100%+16px)/12)_100%]')}
      style={wide ? { gridTemplateColumns: `repeat(${COLS}, minmax(0, 1fr))`, gridAutoRows: `${ROW}px`, gap: `${GAP}px`, minHeight: rows * (ROW + GAP) } : undefined}>
      {items.map((it) => {
        const w = widgets.data?.[it.id]
        const style = wide ? { gridColumn: `${it.x + 1} / span ${it.w}`, gridRow: `${it.y + 1} / span ${it.h}` } : { height: Math.max(it.h, 2) * 64 }
        const size = it.w <= 3 ? 'small' : 'wide'
        return (
          <div key={it.id} style={style} className="relative min-h-0">
            {!w ? (
              <div className="h-full animate-pulse rounded-[20px] border border-line bg-surface" />
            ) : (
              <WidgetCard w={w} editing={editing} refreshing={refreshing === it.id}
                drag={editing && wide ? <DragHandle onPointerDown={(e) => start(e, it, 'move')} aria-label={t('dash.move')} /> : undefined}
                onRemove={editing ? () => commit(layout.filter((l) => l.id !== it.id)) : undefined}
                onRefresh={'hidden' in w || w.source === 'builtin' ? undefined : () => refresh(it.id)}
                onShare={'hidden' in w || w.source !== 'routine' || !w.mine ? undefined : (s) => share(it.id, s)}>
                {!('hidden' in w) && <WidgetBody w={w} size={size} />}
              </WidgetCard>
            )}
            {editing && wide && (
              <span role="presentation" onPointerDown={(e) => start(e, it, 'size')}
                className="absolute bottom-1.5 right-1.5 z-10 size-4 cursor-se-resize rounded-sm border-b-2 border-r-2 border-ink-3 opacity-70 hover:opacity-100" />
            )}
          </div>
        )
      })}
    </div>
    </div>
  )
}

// AddWidget lists everything that can go on the dashboard, drawn as it
// will look.
function AddWidget({ taken, onPick, onClose }: { taken: string[]; onPick: (w: WidgetView) => void; onClose: () => void }) {
  const t = useT()
  const all = useQuery({ queryKey: ['widgets'], queryFn: api.widgets })
  const [q, setQ] = useState('')
  const groups: { key: WidgetView['source']; label: TKey }[] = [
    { key: 'routine', label: 'dash.fromRoutines' }, { key: 'builtin', label: 'dash.builtin' }, { key: 'status', label: 'dash.routineStatus' },
  ]
  const free = (all.data ?? []).filter((w) => !taken.includes(w.id) && (!q || w.title.toLowerCase().includes(q.toLowerCase())))
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" role="dialog" aria-modal="true" aria-label={t('dash.add')} onClick={onClose}>
      <div className="flex h-full w-full max-w-xl flex-col bg-bg shadow-[var(--shadow-pop)]" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 border-b border-line p-4">
          <h2 className="flex-1 text-[17px] font-semibold">{t('dash.add')}</h2>
          <button type="button" onClick={onClose} aria-label={t('dash.close')} className="grid size-8 place-items-center rounded-lg hover:bg-sunken"><X size={16} /></button>
        </div>
        <div className="p-4 pb-0">
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('dash.search')} aria-label={t('dash.search')}
            className="h-10 w-full rounded-[10px] border border-line bg-surface px-3 text-sm outline-none focus:border-accent" />
          <p className="mt-2 text-[12.5px] text-ink-3">{t('dash.addHint')}</p>
        </div>
        <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-4">
          {all.isLoading && <Loader2 className="mx-auto animate-spin text-ink-3" />}
          {groups.map((g) => {
            const ws = free.filter((w) => w.source === g.key)
            if (!ws.length) return null
            return (
              <section key={g.key}>
                <h3 className="mb-2 text-[12px] font-semibold uppercase tracking-wide text-ink-3">{t(g.label)}</h3>
                <div className="grid gap-3 sm:grid-cols-2">
                  {ws.map((w) => (
                    <button key={w.id} type="button" onClick={() => { onPick(w); onClose() }} className="group h-44 text-left" aria-label={t('dash.addThis', { name: w.title })}>
                      <div className="pointer-events-none h-full transition group-hover:-translate-y-0.5">
                        <WidgetCard w={w}><WidgetBody w={w} size="small" /></WidgetCard>
                      </div>
                    </button>
                  ))}
                </div>
              </section>
            )
          })}
          {!all.isLoading && free.length === 0 && <p className="text-center text-[13.5px] text-ink-3">{t('dash.nothingToAdd')}</p>}
        </div>
      </div>
    </div>
  )
}
