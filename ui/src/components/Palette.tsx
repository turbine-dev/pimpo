import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { CornerDownLeft, Plus, Repeat, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { nav as pages } from './Shell'

type Item = { id: string; label: string; hint: string; icon: React.ReactNode; run: () => void }

// Palette is ⌘K: jump to any page or routine, or start a task.
export function Palette({ open, onOpenChange, onNewTask }: { open: boolean; onOpenChange: (o: boolean) => void; onNewTask: () => void }) {
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [active, setActive] = useState(0)
  const routines = useQuery({ queryKey: ['routines'], queryFn: api.routines, enabled: open })
  const items = useMemo<Item[]>(() => {
    const go = (to: string) => () => {
      onOpenChange(false)
      navigate(to)
    }
    const all: Item[] = [
      { id: 'new', label: 'Nova tarefa', hint: 'ação', icon: <Plus size={15} />, run: () => { onOpenChange(false); onNewTask() } },
      ...pages.map((p) => ({ id: p.to, label: p.label, hint: 'página', icon: p.icon, run: go(p.to) })),
      ...(routines.data ?? []).map((r) => ({ id: r.id, label: r.name, hint: 'rotina', icon: <Repeat size={15} />, run: go(`/routines/${r.id}`) })),
    ]
    const needle = q.trim().toLowerCase()
    return needle ? all.filter((i) => i.label.toLowerCase().includes(needle)) : all
  }, [q, routines.data, navigate, onOpenChange, onNewTask])
  useEffect(() => setActive(0), [q])
  return (
    <Dialog.Root open={open} onOpenChange={(o) => { onOpenChange(o); if (!o) setQ('') }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[18vh] z-50 w-[min(560px,calc(100vw-32px))] -translate-x-1/2 overflow-hidden rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none" aria-describedby={undefined}>
          <Dialog.Title className="sr-only">Buscar</Dialog.Title>
          <div className="flex items-center gap-3 border-b border-line px-4">
            <Search size={16} className="text-ink-3" />
            <input
              autoFocus
              value={q}
              onChange={(e) => setQ(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => Math.min(a + 1, items.length - 1)) }
                if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)) }
                if (e.key === 'Enter') items[active]?.run()
              }}
              placeholder="Ir para uma rotina, página ou ação…"
              aria-label="Buscar"
              className="h-12 flex-1 bg-transparent text-[14.5px] outline-none placeholder:text-ink-3"
            />
          </div>
          <ul className="max-h-80 overflow-y-auto p-2" role="listbox">
            {items.map((it, i) => (
              <li key={it.id} role="option" aria-selected={i === active}>
                <button onMouseEnter={() => setActive(i)} onClick={it.run} className={cn('flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-[13.5px]', i === active ? 'bg-sunken' : '')}>
                  <span className="text-ink-3">{it.icon}</span>
                  <span className="flex-1">{it.label}</span>
                  <span className="text-[11.5px] text-ink-3">{it.hint}</span>
                  {i === active && <CornerDownLeft size={13} className="text-ink-3" />}
                </button>
              </li>
            ))}
            {items.length === 0 && <li className="px-3 py-6 text-center text-[13px] text-ink-3">Nada encontrado.</li>}
          </ul>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
