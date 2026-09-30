import { useQuery } from '@tanstack/react-query'
import { canOpen, useRole } from '../lib/roles'
import { UpdateBanner } from './Updates'
import { Activity, Bell, Bot, Brain, ChevronDown, CircleHelp, Coins, House, LibraryBig, Menu, MessageSquare, Moon, Plug, Plus, ReceiptText, Repeat, Search, Settings, ShieldCheck, Smartphone, Sun, Layers, KeyRound, Users, Puzzle } from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { InboxPanel } from './InboxPanel'
import { SystemPanel } from './SystemPanel'
import { Logo } from './PimpoArt'
import { Kbd } from './ui'

export type NavItem = { to: string; label: TKey; icon: ReactNode; badge?: number }

// What people open every day comes first; the rest waits under "More".
export const primary: NavItem[] = [
  { to: '/', label: 'nav.home', icon: <House size={17} /> },
  { to: '/routines', label: 'nav.routines', icon: <Repeat size={17} /> },
  { to: '/receipts', label: 'nav.activity', icon: <ReceiptText size={17} /> },
  { to: '/assistants', label: 'nav.assistants', icon: <Bot size={17} /> },
]

export const secondary: NavItem[] = [
  { to: '/gallery', label: 'nav.gallery', icon: <LibraryBig size={17} /> },
  { to: '/jobs', label: 'nav.jobs', icon: <Layers size={17} /> },
  { to: '/skills', label: 'nav.skills', icon: <Puzzle size={17} /> },
  { to: '/phone', label: 'nav.phone', icon: <Smartphone size={17} /> },
  { to: '/connections', label: 'nav.connections', icon: <Plug size={17} /> },
  { to: '/people', label: 'nav.people', icon: <Users size={17} /> },
  { to: '/memory', label: 'nav.memory', icon: <Brain size={17} /> },
  { to: '/rules', label: 'nav.rules', icon: <ShieldCheck size={17} /> },
  { to: '/cost', label: 'nav.cost', icon: <Coins size={17} /> },
]

// nav is every page, for the command palette.
export const nav: NavItem[] = [
  ...primary,
  { to: '/chat', label: 'nav.chat', icon: <MessageSquare size={17} /> },
  { to: '/inbox', label: 'nav.inbox', icon: <Bell size={17} /> },
  ...secondary,
  { to: '/settings', label: 'nav.settings', icon: <Settings size={17} /> },
  { to: '/help', label: 'nav.help', icon: <CircleHelp size={17} /> },
]

export { Logo }

function useTheme() {
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    try {
      return (localStorage.getItem('pimpo.theme') ?? localStorage.getItem('zodim.theme') ?? 'dark') as 'light' | 'dark'
    } catch {
      return 'dark'
    }
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('pimpo.theme', theme)
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

const link = (active: boolean) => cn('group flex items-center gap-3 rounded-[10px] px-2.5 py-1.5 text-[13.5px] transition-colors', active ? 'bg-sunken font-medium text-ink' : 'text-ink-2 hover:bg-sunken/70 hover:text-ink')

function useOutside(open: boolean, close: () => void) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const click = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) close() }
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    document.addEventListener('mousedown', click)
    document.addEventListener('keydown', key)
    return () => { document.removeEventListener('mousedown', click); document.removeEventListener('keydown', key) }
  }, [open, close])
  return ref
}

function Chats() {
  const t = useT()
  const chats = useQuery({ queryKey: ['chats'], queryFn: api.chats })
  const list = (chats.data ?? []).slice(0, 12)
  return (
    <div className="mt-5 min-h-0 flex-1 overflow-y-auto">
      <div className="mb-1 flex items-center justify-between px-2.5">
        <Link to="/chat" className="text-[11.5px] font-semibold uppercase tracking-wide text-ink-3 hover:text-ink">{t('shell.chats')}</Link>
        <Link to="/" aria-label={t('chat.new')} className="grid size-6 place-items-center rounded-md text-ink-3 hover:bg-sunken hover:text-ink"><Plus size={14} /></Link>
      </div>
      {list.length === 0 ? <p className="px-2.5 text-[12px] text-ink-3">{t('shell.noChats')}</p> : (
        <ul className="space-y-px">
          {list.map((c) => (
            <li key={c.id}>
              <NavLink to={`/chat/${c.id}`} className={({ isActive }) => cn('block truncate rounded-[10px] px-2.5 py-1.5 text-[13px]', isActive ? 'bg-sunken font-medium text-ink' : 'text-ink-2 hover:bg-sunken/70 hover:text-ink')}>{c.title}</NavLink>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function UserMenu({ onSystem, theme, toggleTheme }: { onSystem: () => void; theme: 'light' | 'dark'; toggleTheme: () => void }) {
  const t = useT()
  const nav = useNavigate()
  const [open, setOpen] = useState(false)
  const ref = useOutside(open, () => setOpen(false))
  const go = (to: string) => () => { setOpen(false); nav(to) }
  const role = useRole()
  const item = 'flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-[13px] text-ink-2 hover:bg-sunken hover:text-ink'
  return (
    <div ref={ref} className="relative flex-1">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open} aria-haspopup="menu" aria-label={t('shell.menu')}
        className="flex w-full items-center gap-2.5 rounded-[10px] px-2 py-1.5 text-left hover:bg-sunken">
        <Logo size={26} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium">Pimpo</span>
          <span className="block truncate text-[11.5px] text-ink-3">{t('shell.thisComputer')}</span>
        </span>
        <ChevronDown size={14} className="text-ink-3" />
      </button>
      {open && (
        <div role="menu" className="absolute bottom-full left-0 z-40 mb-2 w-72 rounded-xl border border-line bg-surface p-1.5 shadow-[var(--shadow-pop)]">
          <button role="menuitem" className={item} onClick={go('/account')}><KeyRound size={15} /> {t('acct.title')}</button>
          {role === 'owner' && <button role="menuitem" className={item} onClick={go('/settings')}><Settings size={15} /> {t('nav.settings')}</button>}
          {role !== 'guest' && <button role="menuitem" className={item} onClick={go('/cost')}><Coins size={15} /> {t('shell.usage')}</button>}
          {role === 'owner' && <button role="menuitem" className={item} onClick={() => { setOpen(false); onSystem() }}><Activity size={15} /> {t('sys.title')} <Kbd>⌘⇧D</Kbd></button>}
          {role === 'owner' && <button role="menuitem" className={item} onClick={go('/settings#celular')}><Smartphone size={15} /> {t('shell.pairPhone')}</button>}
          <div className="my-1 border-t border-line" />
          <button role="menuitem" className={item} onClick={() => { toggleTheme() }}>{theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />} {theme === 'dark' ? t('shell.light') : t('shell.dark')}</button>
          <button role="menuitem" className={item} onClick={go('/help')}><CircleHelp size={15} /> {t('nav.help')}</button>
        </div>
      )}
    </div>
  )
}

function InboxButton({ count }: { count: number }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const ref = useOutside(open, () => setOpen(false))
  return (
    <div ref={ref} className="relative">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open} aria-label={t('inbox.title')}
        className={cn('relative grid size-9 place-items-center rounded-[10px] text-ink-2 hover:bg-sunken hover:text-ink', open && 'bg-sunken text-ink')}>
        <Bell size={17} />
        {count > 0 && <span className="absolute -right-0.5 -top-0.5 grid min-w-4 place-items-center rounded-full bg-danger px-1 text-[10px] font-semibold text-white tabular-nums">{count}</span>}
      </button>
      {open && <InboxPanel onClose={() => setOpen(false)} />}
    </div>
  )
}

export function Shell({ children, attention = 0, budget, healthy = true, onSearch }: { children: ReactNode; attention?: number; budget?: { spent: number; limit: number }; healthy?: boolean; onSearch?: () => void }) {
  const t = useT()
  const role = useRole()
  const [theme, toggle] = useTheme()
  const [more, setMore] = useState(false)
  const [moreOpen, setMoreOpen] = useState(() => { try { return localStorage.getItem('pimpo.more') === '1' } catch { return false } })
  const [system, setSystem] = useState(false)
  useEffect(() => { try { localStorage.setItem('pimpo.more', moreOpen ? '1' : '0') } catch { /* not remembered */ } }, [moreOpen])
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'd') { e.preventDefault(); setSystem((s) => !s) }
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [])
  return (
    <div className="flex h-full">
      <aside className="hidden w-64 shrink-0 flex-col border-r border-line bg-surface/60 px-3 pb-3 pt-4 backdrop-blur md:flex">
        <div className="mb-4 flex items-center gap-2.5 px-2 in-data-[desktop=mac]:mt-7">
          <Logo />
          <span className="flex-1 text-[15px] font-semibold tracking-tight">Pimpo</span>
          <Link to="/" aria-label={t('chat.new')} title={t('chat.new')} className="grid size-8 place-items-center rounded-lg text-ink-2 hover:bg-sunken hover:text-ink"><Plus size={17} /></Link>
        </div>
        <nav className="flex flex-col gap-px" aria-label={t('shell.main')}>
          {primary.filter((it) => canOpen(it.to, role)).map((it) => (
            <NavLink key={it.to} to={it.to} end={it.to === '/'} className={({ isActive }) => link(isActive)}>
              <span className="text-ink-3 group-[.active]:text-ink">{it.icon}</span>
              <span className="flex-1">{t(it.label)}</span>
            </NavLink>
          ))}
          <button type="button" onClick={() => setMoreOpen(!moreOpen)} aria-expanded={moreOpen} className={cn(link(false), 'mt-1')}>
            <ChevronDown size={17} className={cn('text-ink-3 transition', !moreOpen && '-rotate-90')} />
            <span className="flex-1 text-left">{t('nav.more')}</span>
          </button>
          {moreOpen && secondary.filter((it) => canOpen(it.to, role)).map((it) => (
            <NavLink key={it.to} to={it.to} className={({ isActive }) => cn(link(isActive), 'pl-4')}>
              <span className="text-ink-3">{it.icon}</span>
              <span className="flex-1">{t(it.label)}</span>
            </NavLink>
          ))}
        </nav>
        <Chats />
        <div className="mt-3 flex items-center gap-1 border-t border-line pt-3">
          <UserMenu onSystem={() => setSystem(true)} theme={theme} toggleTheme={toggle} />
          <InboxButton count={attention} />
        </div>
        <button type="button" onClick={() => setSystem(true)} className="mt-2 flex items-center gap-2 px-2 text-left text-[11.5px] text-ink-3 hover:text-ink">
          <span className={cn('size-2 rounded-full', healthy ? 'bg-read' : 'animate-pulse-soft bg-danger')} />
          {healthy ? t('shell.healthy') : t('shell.unhealthy')}
        </button>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-line bg-bg/80 px-4 backdrop-blur md:px-8 in-data-[desktop=mac]:max-md:pl-20">
          <div className="flex items-center gap-2 md:hidden">
            <Logo size={24} />
          </div>
          <button onClick={onSearch} className="flex h-9 max-w-md flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 text-left text-[13px] text-ink-3 hover:border-line-strong">
            <Search size={15} />
            <span className="flex-1 truncate">{t('shell.search')}</span>
            <Kbd>⌘K</Kbd>
          </button>
          <div className="ml-auto flex items-center gap-2">
            {budget && <Budget {...budget} />}
            <div className="md:hidden"><InboxButton count={attention} /></div>
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto px-4 pb-24 pt-6 md:px-8 md:pb-10"><UpdateBanner />{children}</main>
      </div>

      <nav className="fixed inset-x-0 bottom-0 z-20 flex border-t border-line bg-surface/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden" aria-label={t('shell.mainMobile')}>
        {[{ to: '/', label: 'nav.home' as TKey, icon: <House size={17} /> }, { to: '/chat', label: 'nav.chat' as TKey, icon: <MessageSquare size={17} /> }, { to: '/routines', label: 'nav.routines' as TKey, icon: <Repeat size={17} /> }].map((it) => (
          <NavLink key={it.to} to={it.to} end={it.to === '/'} onClick={() => setMore(false)} className={({ isActive }) => cn('relative flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px]', isActive ? 'text-ink' : 'text-ink-3')}>
            {it.icon}
            {t(it.label)}
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
            {[...primary.slice(2), { to: '/inbox', label: 'nav.inbox' as TKey, icon: <Bell size={17} /> }, ...secondary, { to: '/settings', label: 'nav.settings' as TKey, icon: <Settings size={17} /> }, { to: '/help', label: 'nav.help' as TKey, icon: <CircleHelp size={17} /> }].filter((it) => canOpen(it.to, role)).map((it) => (
              <NavLink key={it.to} to={it.to} onClick={() => setMore(false)} className={({ isActive }) => cn('flex flex-col items-center gap-1 rounded-xl py-3 text-[11.5px]', isActive ? 'bg-sunken text-ink' : 'text-ink-2')}>
                {it.icon}
                {t(it.label)}
              </NavLink>
            ))}
            <button type="button" onClick={() => { setMore(false); setSystem(true) }} className="flex flex-col items-center gap-1 rounded-xl py-3 text-[11.5px] text-ink-2"><Activity size={17} />{t('sys.short')}</button>
            <button type="button" onClick={toggle} className="flex flex-col items-center gap-1 rounded-xl py-3 text-[11.5px] text-ink-2">{theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}{t('shell.theme')}</button>
          </div>
        </div>
      )}
      <SystemPanel open={system} onOpenChange={setSystem} />
    </div>
  )
}
