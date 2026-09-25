import { Bell, Brain, Menu, Coins, Moon, Plug, ReceiptText, Repeat, Search, Settings, ShieldCheck, Sun, Users, LibraryBig } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { Kbd } from './ui'

export type NavItem = { to: string; label: TKey; icon: ReactNode; badge?: number }

export const nav: NavItem[] = [
  { to: '/', label: 'nav.routines', icon: <Repeat size={17} /> },
  { to: '/inbox', label: 'nav.inbox', icon: <Bell size={17} /> },
  { to: '/gallery', label: 'nav.gallery', icon: <LibraryBig size={17} /> },
  { to: '/receipts', label: 'nav.receipts', icon: <ReceiptText size={17} /> },
  { to: '/rules', label: 'nav.rules', icon: <ShieldCheck size={17} /> },
  { to: '/cost', label: 'nav.cost', icon: <Coins size={17} /> },
  { to: '/memory', label: 'nav.memory', icon: <Brain size={17} /> },
  { to: '/connections', label: 'nav.connections', icon: <Plug size={17} /> },
  { to: '/people', label: 'nav.people', icon: <Users size={17} /> },
  { to: '/settings', label: 'nav.settings', icon: <Settings size={17} /> },
]

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <defs>
        <linearGradient id="zg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#6d5dfc" />
          <stop offset=".55" stopColor="#4f46e5" />
          <stop offset="1" stopColor="#1e1b4b" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="7.5" fill="url(#zg)" />
      <path d="M10 11.1h10.5l-9.75 9.75H22" fill="none" stroke="white" strokeWidth="2.9" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M23.6 6.75c.3 1.7.9 2.25 2.55 2.55-1.65.3-2.25.9-2.55 2.55-.3-1.65-.9-2.25-2.55-2.55 1.65-.3 2.25-.85 2.55-2.55Z" fill="white" />
    </svg>
  )
}

function useTheme() {
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    try {
      return (localStorage.getItem('zodim.theme') as 'light' | 'dark') || 'dark'
    } catch {
      return 'dark'
    }
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('zodim.theme', theme)
    } catch {
      /* private mode: the theme just is not remembered */
    }
  }, [theme])
  return [theme, () => setTheme(theme === 'dark' ? 'light' : 'dark')] as const
}

export function Budget({ spent, limit }: { spent: number; limit: number }) {
  const t = useT()
  const pct = limit > 0 ? Math.min(100, (spent / limit) * 100) : 0
  const tone = pct > 90 ? 'bg-danger' : pct > 70 ? 'bg-change' : 'bg-read'
  return (
    <div className="hidden items-center gap-2.5 rounded-full border border-line bg-surface px-3 py-1.5 text-[12.5px] sm:flex" title={t('shell.budget')}>
      <span className="tabular-nums text-ink">${spent.toFixed(2)}</span>
      <span className="h-1.5 w-16 overflow-hidden rounded-full bg-sunken">
        <span className={cn('block h-full rounded-full transition-all', tone)} style={{ width: `${pct}%` }} />
      </span>
      <span className="tabular-nums text-ink-3">{t('shell.budgetOf', { limit: `$${limit.toFixed(2)}` })}</span>
    </div>
  )
}

export function Shell({ children, items = nav, budget, healthy = true, onSearch }: { children: ReactNode; items?: NavItem[]; budget?: { spent: number; limit: number }; healthy?: boolean; onSearch?: () => void }) {
  const t = useT()
  const [theme, toggle] = useTheme()
  const [more, setMore] = useState(false)
  return (
    <div className="flex h-full">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-line bg-surface/60 px-3 py-4 backdrop-blur md:flex">
        <div className="mb-6 flex items-center gap-2.5 px-2 in-data-[desktop=mac]:mt-7">
          <Logo />
          <span className="text-[15px] font-semibold tracking-tight">Zodim</span>
        </div>
        <nav className="flex flex-col gap-0.5" aria-label={t('shell.main')}>
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
              <span className="flex-1">{t(it.label)}</span>
              {!!it.badge && <span className="rounded-full bg-danger px-1.5 text-[11px] font-semibold text-white tabular-nums">{it.badge}</span>}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex items-center gap-2 px-2 text-[12px] text-ink-3">
          <span className={cn('size-2 rounded-full', healthy ? 'bg-read' : 'animate-pulse-soft bg-danger')} />
          {healthy ? t('shell.healthy') : t('shell.unhealthy')}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-line bg-bg/80 px-4 backdrop-blur md:px-8 in-data-[desktop=mac]:max-md:pl-20">
          <div className="flex items-center gap-2 md:hidden">
            <Logo size={24} />
          </div>
          <button onClick={onSearch} className="flex h-9 max-w-md flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 text-left text-[13px] text-ink-3 hover:border-line-strong">
            <Search size={15} />
            <span className="flex-1">{t('shell.search')}</span>
            <Kbd>⌘K</Kbd>
          </button>
          <div className="ml-auto flex items-center gap-2">
            {budget && <Budget {...budget} />}
            <button onClick={toggle} className="grid size-9 place-items-center rounded-[10px] text-ink-2 hover:bg-sunken" aria-label={theme === 'dark' ? t('shell.light') : t('shell.dark')}>
              {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
            </button>
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto px-4 pb-24 pt-6 md:px-8 md:pb-10">{children}</main>
      </div>

      <nav className="fixed inset-x-0 bottom-0 z-20 flex border-t border-line bg-surface/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden" aria-label={t('shell.mainMobile')}>
        {['/inbox', '/receipts', '/'].map((to) => items.find((it) => it.to === to)).filter((it): it is NavItem => !!it).map((it) => (
          <NavLink key={it.to} to={it.to} end={it.to === '/'} onClick={() => setMore(false)} className={({ isActive }) => cn('relative flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px]', isActive ? 'text-ink' : 'text-ink-3')}>
            {it.icon}
            {it.to === '/inbox' ? t('nav.approve') : t(it.label)}
            {!!it.badge && <span className="absolute right-[30%] top-1.5 size-2 rounded-full bg-danger" />}
          </NavLink>
        ))}
        <button type="button" onClick={() => setMore(!more)} aria-expanded={more} className={cn('flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px]', more ? 'text-ink' : 'text-ink-3')}>
          <Menu size={17} />
          {t('nav.more')}
        </button>
      </nav>
      {more && (
        <div className="fixed inset-x-0 bottom-[calc(57px+env(safe-area-inset-bottom))] z-20 border-t border-line bg-surface p-2 shadow-[var(--shadow-pop)] md:hidden">
          <div className="grid grid-cols-3 gap-1">
            {items.filter((it) => !['/inbox', '/receipts', '/'].includes(it.to)).map((it) => (
              <NavLink key={it.to} to={it.to} onClick={() => setMore(false)} className={({ isActive }) => cn('flex flex-col items-center gap-1 rounded-xl py-3 text-[11.5px]', isActive ? 'bg-sunken text-ink' : 'text-ink-2')}>
                {it.icon}
                {t(it.label)}
              </NavLink>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
