import { Bell, Brain, Coins, Moon, Plug, ReceiptText, Repeat, Search, Settings, ShieldCheck, Sun, Users, LibraryBig } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { cn } from '../lib/cn'
import { Kbd } from './ui'

export type NavItem = { to: string; label: string; icon: ReactNode; badge?: number }

export const nav: NavItem[] = [
  { to: '/', label: 'Rotinas', icon: <Repeat size={17} /> },
  { to: '/inbox', label: 'Precisa de você', icon: <Bell size={17} /> },
  { to: '/gallery', label: 'Galeria', icon: <LibraryBig size={17} /> },
  { to: '/receipts', label: 'Recibos', icon: <ReceiptText size={17} /> },
  { to: '/rules', label: 'Regras', icon: <ShieldCheck size={17} /> },
  { to: '/cost', label: 'Custo', icon: <Coins size={17} /> },
  { to: '/memory', label: 'Memória', icon: <Brain size={17} /> },
  { to: '/connections', label: 'Conexões', icon: <Plug size={17} /> },
  { to: '/people', label: 'Pessoas', icon: <Users size={17} /> },
  { to: '/settings', label: 'Ajustes', icon: <Settings size={17} /> },
]

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <defs>
        <linearGradient id="vg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="var(--color-accent)" />
          <stop offset="1" stopColor="var(--color-explore)" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="9" fill="url(#vg)" />
      <path d="M8 17c2.4-4.2 5-6.3 8-6.3s5.6 2.1 8 6.3c-2.4 4.2-5 6.3-8 6.3S10.4 21.2 8 17Z" fill="none" stroke="white" strokeWidth="2" strokeLinejoin="round" />
      <circle cx="16" cy="17" r="2.6" fill="white" />
    </svg>
  )
}

function useTheme() {
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    try {
      return (localStorage.getItem('vigia.theme') as 'light' | 'dark') || 'dark'
    } catch {
      return 'dark'
    }
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('vigia.theme', theme)
    } catch {
      /* private mode: the theme just is not remembered */
    }
  }, [theme])
  return [theme, () => setTheme(theme === 'dark' ? 'light' : 'dark')] as const
}

export function Budget({ spent, limit }: { spent: number; limit: number }) {
  const pct = limit > 0 ? Math.min(100, (spent / limit) * 100) : 0
  const tone = pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read'
  return (
    <div className="hidden items-center gap-2.5 rounded-full border border-line bg-surface px-3 py-1.5 text-[12.5px] sm:flex" title="Gasto de hoje com modelos">
      <span className="tabular-nums text-ink">${spent.toFixed(2)}</span>
      <span className="h-1.5 w-16 overflow-hidden rounded-full bg-sunken">
        <span className={cn('block h-full rounded-full transition-all', tone)} style={{ width: `${pct}%` }} />
      </span>
      <span className="tabular-nums text-ink-3">de ${limit.toFixed(2)}</span>
    </div>
  )
}

export function Shell({ children, items = nav, budget, healthy = true, onSearch }: { children: ReactNode; items?: NavItem[]; budget?: { spent: number; limit: number }; healthy?: boolean; onSearch?: () => void }) {
  const [theme, toggle] = useTheme()
  return (
    <div className="flex h-full">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-line bg-surface/60 px-3 py-4 backdrop-blur md:flex">
        <div className="mb-6 flex items-center gap-2.5 px-2">
          <Logo />
          <span className="text-[15px] font-semibold tracking-tight">Vigia</span>
        </div>
        <nav className="flex flex-col gap-0.5" aria-label="Principal">
          {items.map((it) => (
            <NavLink
              key={it.to}
              to={it.to}
              end={it.to === '/'}
              className={({ isActive }) =>
                cn('group flex items-center gap-3 rounded-[10px] px-2.5 py-2 text-[13.5px] transition-colors', isActive ? 'bg-sunken font-medium text-ink' : 'text-ink-2 hover:bg-sunken/70 hover:text-ink')
              }
            >
              <span className="text-ink-3 group-[.active]:text-ink">{it.icon}</span>
              <span className="flex-1">{it.label}</span>
              {!!it.badge && <span className="rounded-full bg-danger px-1.5 text-[11px] font-semibold text-white tabular-nums">{it.badge}</span>}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex items-center gap-2 px-2 text-[12px] text-ink-3">
          <span className={cn('size-2 rounded-full', healthy ? 'bg-read' : 'animate-pulse-soft bg-danger')} />
          {healthy ? 'Tudo funcionando' : 'Algo precisa de atenção'}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-line bg-bg/80 px-4 backdrop-blur md:px-8">
          <div className="flex items-center gap-2 md:hidden">
            <Logo size={24} />
          </div>
          <button onClick={onSearch} className="flex h-9 max-w-md flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 text-left text-[13px] text-ink-3 hover:border-line-strong">
            <Search size={15} />
            <span className="flex-1">Buscar rotinas, recibos, regras…</span>
            <Kbd>⌘K</Kbd>
          </button>
          <div className="ml-auto flex items-center gap-2">
            {budget && <Budget {...budget} />}
            <button onClick={toggle} className="grid size-9 place-items-center rounded-[10px] text-ink-2 hover:bg-sunken" aria-label={theme === 'dark' ? 'Usar tema claro' : 'Usar tema escuro'}>
              {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
            </button>
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto px-4 pb-24 pt-6 md:px-8 md:pb-10">{children}</main>
      </div>

      <nav className="fixed inset-x-0 bottom-0 z-20 flex border-t border-line bg-surface/95 backdrop-blur md:hidden" aria-label="Principal (celular)">
        {items.slice(0, 3).map((it) => (
          <NavLink key={it.to} to={it.to} end={it.to === '/'} className={({ isActive }) => cn('relative flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px]', isActive ? 'text-ink' : 'text-ink-3')}>
            {it.icon}
            {it.label}
            {!!it.badge && <span className="absolute right-[30%] top-1.5 size-2 rounded-full bg-danger" />}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
