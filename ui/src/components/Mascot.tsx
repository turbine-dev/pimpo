import { useQuery } from '@tanstack/react-query'
import { BellOff, EyeOff, Inbox, Plus } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type VEvent } from '../lib/api'
import { useT, type TKey } from '../lib/i18n'
import { Head, type Mood } from './PimpoArt'

// The Pimpo mascot: a tuxedo kitten that sits in a corner, shows how
// things are going through its mood, and raises what needs the owner in a
// speech bubble. It only reacts to events; it never acts on its own.

type Bubble = { kind: string; text: string; count: number }

const store = {
  get(k: string) { try { return localStorage.getItem(k) } catch { return null } },
  set(k: string, v: string) { try { localStorage.setItem(k, v) } catch { /* private mode */ } },
}

// In the desktop app the mascot lives in its own window, on the desktop
// even with the app closed; the app switches it (see desktop/src-tauri).
export const inDesktopApp = () => '__PIMPO_DESKTOP__' in window

// The mascot is off until the owner turns it on.
export const mascotOn = () => store.get('pimpo.mascot') === 'on'
export const setMascotOn = (on: boolean) => {
  store.set('pimpo.mascot', on ? 'on' : 'off')
  window.dispatchEvent(new Event('pimpo:mascot'))
  if (inDesktopApp()) window.location.assign('/desktop/mascot?on=' + (on ? '1' : '0'))
}

const reducedMotion = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
const games: Play[] = ['butterfly', 'ball', 'yawn', 'groom']
// A game every two to five quiet minutes.
const nextGame = () => Date.now() + (2 + Math.random() * 3) * 60_000

const SLEEP_AFTER = 4 * 60_000

// The desktop app's floating window, when the cat lives in one. It may only
// resize, move and drag itself.
type TauriWindow = {
  outerPosition(): Promise<{ x: number; y: number }>
  outerSize(): Promise<{ width: number; height: number }>
  scaleFactor(): Promise<number>
  setPosition(p: unknown): Promise<void>
  setSize(s: unknown): Promise<void>
  startDragging(): Promise<void>
}
type TauriGlobal = { window?: { getCurrentWindow(): TauriWindow }; dpi?: { PhysicalPosition: new (x: number, y: number) => unknown; LogicalSize: new (w: number, h: number) => unknown } }
const tauri = () => (window as unknown as { __TAURI__?: TauriGlobal }).__TAURI__

// fitWindow grows the floating window for the bubble or menu and shrinks it
// back to the cat, keeping its bottom-right corner in place.
async function fitWindow(expanded: boolean | 'play') {
  const t = tauri()
  const w = t?.window?.getCurrentWindow()
  if (!w || !t?.dpi) return
  const [pos, size, scale] = await Promise.all([w.outerPosition(), w.outerSize(), w.scaleFactor()])
  const [nw, nh] = expanded === 'play' ? [200, 126] : expanded ? [330, 480] : [96, 120]
  await w.setPosition(new t.dpi.PhysicalPosition(Math.round(pos.x + size.width - nw * scale), Math.round(pos.y + size.height - nh * scale)))
  await w.setSize(new t.dpi.LogicalSize(nw, nh))
}

export type Play = 'butterfly' | 'ball' | 'yawn' | 'groom'

// What the cat's face does during each game.
const playMood: Record<Play, Mood> = { butterfly: 'working', ball: 'alert', yawn: 'yawn', groom: 'happy' }

export function Cat({ mood, petting, play = null }: { mood: Mood; petting: boolean; play?: Play | null }) {
  const face = play ? playMood[play] : mood
  return (
    <svg viewBox="40 70 440 640" className={`pimpo pimpo-${face}${petting ? ' pimpo-pet' : ''}${play ? ` pimpo-play pimpo-play-${play}` : ''}`} aria-hidden>
      <g className="pimpo-tail">
        <path d="M352 646c86 8 124-58 96-132-10-28-38-24-30 2" fill="none" stroke="#111" strokeWidth="34" strokeLinecap="round" />
      </g>
      <g className="pimpo-body">
        <path d="M150 664c-22-104 20-226 106-246 86 20 128 142 106 246-36 22-176 22-212 0Z" fill="#111" />
        <path d="M256 430c-34 40-44 130-34 222h68c10-92 0-182-34-222Z" fill="#fff" />
        <ellipse cx="212" cy="664" rx="42" ry="24" fill="#fff" stroke="#111" strokeWidth="6" />
        <ellipse cx="300" cy="664" rx="42" ry="24" fill="#fff" stroke="#111" strokeWidth="6" />
        <path d="M200 656v14M222 656v14M288 656v14M310 656v14" stroke="#111" strokeWidth="5" strokeLinecap="round" />
      </g>
      <g className="pimpo-head">
        <Head mood={face} />
      </g>
      {(play === 'butterfly' || play === 'ball' || play === 'groom') && (
        // A raised front paw: swiping at the game, or washing the face.
        <g className="pimpo-paw">
          <path d="M214 520c-16-40-30-78-40-112" fill="none" stroke="#111" strokeWidth="44" strokeLinecap="round" />
          <ellipse cx="172" cy="398" rx="26" ry="22" fill="#fff" stroke="#111" strokeWidth="6" />
        </g>
      )}
      {play === 'butterfly' && (
        <g className="pimpo-butterfly"><g transform="scale(2.3)">
          <g className="pimpo-wings">
            <path d="M0 0c-26-30-58-18-50 6 6 18 30 18 50-6Z" fill="#fff" stroke="#111" strokeWidth="5" />
            <path d="M0 0c26-30 58-18 50 6-6 18-30 18-50-6Z" fill="#fff" stroke="#111" strokeWidth="5" />
            <circle cx="-26" cy="-2" r="6" fill="#111" />
            <circle cx="26" cy="-2" r="6" fill="#111" />
          </g>
          <path d="M0-14v34" stroke="#111" strokeWidth="7" strokeLinecap="round" />
        </g></g>
      )}
      {play === 'ball' && (
        <g className="pimpo-ball"><g transform="scale(1.7)">
          <g className="pimpo-ball-spin">
            <circle r="36" fill="#fff" stroke="#111" strokeWidth="6" />
            <path d="M-30-14c20 6 44 4 58-8M-34 8c24 6 50 2 66-12M-22 26c16 2 36-4 48-18M-8-34c-6 22-4 48 10 66" fill="none" stroke="#111" strokeWidth="4" strokeLinecap="round" />
          </g>
        </g></g>
      )}
      {mood === 'sleep' && !play && (
        <g className="pimpo-z" fill="#888" fontFamily="Inter, sans-serif" fontWeight="700">
          <text x="390" y="150" fontSize="54">z</text>
          <text x="430" y="104" fontSize="40">z</text>
        </g>
      )}
      {petting && !play && <text className="pimpo-heart" x="380" y="170" fontSize="60">♥</text>}
    </svg>
  )
}

export function Mascot({ standalone = false, onOpen }: { standalone?: boolean; onOpen: (path: string) => void }) {
  const t = useT()
  const [on, setOn] = useState(mascotOn)
  const [mood, setMood] = useState<Mood>('idle')
  const [bubble, setBubble] = useState<Bubble | null>(null)
  const [menu, setMenu] = useState(false)
  const [petting, setPetting] = useState(false)
  const [play, setPlay] = useState<Play | null>(null)
  const gameAt = useRef(nextGame())
  const [pos, setPos] = useState<{ right: number; bottom: number }>(() => {
    try { return JSON.parse(store.get('pimpo.mascot.pos') ?? '') } catch { return { right: 20, bottom: window.innerWidth < 768 ? 84 : 20 } }
  })
  const lastActive = useRef(Date.now())
  const moodTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const bubbleTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const drag = useRef<{ x: number; y: number; right: number; bottom: number; moved: boolean } | null>(null)
  const working = useRef(0)
  const windowDragged = useRef(false)
  const state = useQuery({ queryKey: ['state'], queryFn: api.state, enabled: on })
  const waiting = (state.data?.approvals ?? 0) + (state.data?.broken ?? 0)

  useEffect(() => {
    const sync = () => setOn(mascotOn())
    window.addEventListener('pimpo:mascot', sync)
    return () => window.removeEventListener('pimpo:mascot', sync)
  }, [])

  // settle returns to the resting mood after a reaction.
  const settle = useCallback((after = 4000) => {
    clearTimeout(moodTimer.current)
    moodTimer.current = setTimeout(() => setMood(working.current > 0 ? 'working' : 'idle'), after)
  }, [])

  const react = useCallback((m: Mood, after?: number) => {
    lastActive.current = Date.now()
    setMood(m)
    if (after) settle(after)
  }, [settle])

  const show = useCallback((kind: string, text: string) => {
    const muted = Number(store.get('pimpo.mascot.mute') ?? 0) > Date.now()
    if (muted) return
    setBubble((b) => ({ kind, text, count: (b?.count ?? 0) + 1 }))
    clearTimeout(bubbleTimer.current)
    if (kind !== 'approval') bubbleTimer.current = setTimeout(() => setBubble(null), 20_000)
  }, [])

  useEffect(() => {
    const onEvent = (ev: Event) => {
      const e = (ev as CustomEvent<VEvent>).detail
      const d = e.data as Record<string, unknown>
      setPlay(null)
      switch (e.type) {
        case 'exploration.started':
          working.current++
          react('working')
          break
        case 'exploration.finished':
        case 'exploration.failed':
          working.current = Math.max(0, working.current - 1)
          react(e.type === 'exploration.finished' ? 'happy' : 'worried', 4000)
          break
        case 'routine.run.finished':
          react('happy', 3000)
          break
        case 'routine.run.failed':
        case 'channel.down':
          react('worried', 8000)
          break
        case 'approval.requested':
          react('alert')
          break
        case 'approval.resolved':
          react('happy', 2500)
          setBubble((b) => (b?.kind === 'approval' ? null : b))
          break
        case 'notice.sent': {
          const text = String(d.text ?? '').trim()
          const kind = Array.isArray(d.actions) && (d.actions as { data?: string }[]).some((a) => a.data?.startsWith('approve:')) ? 'approval' : String(d.kind ?? '')
          if (text) show(kind, text)
          break
        }
      }
    }
    window.addEventListener('pimpo:event', onEvent)
    return () => window.removeEventListener('pimpo:event', onEvent)
  }, [react, show])

  // Asleep after a quiet while; any activity wakes it.
  useEffect(() => {
    const wake = () => {
      lastActive.current = Date.now()
      setMood((m) => (m === 'sleep' ? 'idle' : m))
    }
    const tick = setInterval(() => {
      if (Date.now() - lastActive.current > SLEEP_AFTER && working.current === 0 && !bubble) setMood((m) => (m === 'idle' ? 'sleep' : m))
    }, 15_000)
    window.addEventListener('pointerdown', wake)
    window.addEventListener('keydown', wake)
    return () => { clearInterval(tick); window.removeEventListener('pointerdown', wake); window.removeEventListener('keydown', wake) }
  }, [bubble])

  useEffect(() => { if (waiting > 0 && mood === 'idle') setMood('alert') }, [waiting, mood])

  // Now and then, when all is quiet, the cat plays for a few seconds.
  useEffect(() => {
    if (!on) return
    const tick = setInterval(() => {
      if (play || bubble || menu || mood !== 'idle' || reducedMotion() || Date.now() < gameAt.current) return
      setPlay(games[Math.floor(Math.random() * games.length)])
      gameAt.current = nextGame()
      setTimeout(() => setPlay(null), 7000)
    }, 10_000)
    return () => clearInterval(tick)
  }, [on, play, bubble, menu, mood])

  // The menu closes on a click anywhere else: another program (the floating
  // window loses focus), the empty space around the cat, or Escape.
  const menuRef = useRef<HTMLDivElement>(null)
  const catRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!menu) return
    const close = () => setMenu(false)
    const outside = (e: PointerEvent) => {
      const target = e.target as Node
      if (!menuRef.current?.contains(target) && !catRef.current?.contains(target)) close()
    }
    const escape = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    window.addEventListener('blur', close)
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', escape)
    return () => {
      window.removeEventListener('blur', close)
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('keydown', escape)
    }
  }, [menu])

  const expanded: boolean | 'play' = bubble || menu ? true : play === 'butterfly' || play === 'ball' ? 'play' : false
  useEffect(() => { if (standalone) fitWindow(expanded).catch(() => {}) }, [standalone, expanded])

  // The desktop app's main window leaves the cat to its floating window.
  if ((!on && !standalone) || (inDesktopApp() && !standalone)) return null

  const onPointerDown = (e: React.PointerEvent) => {
    drag.current = { x: e.clientX, y: e.clientY, right: pos.right, bottom: pos.bottom, moved: false }
    ;(e.target as Element).setPointerCapture?.(e.pointerId)
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.x
    const dy = e.clientY - d.y
    if (!d.moved && Math.hypot(dx, dy) < 5) return
    d.moved = true
    if (standalone) {
      // On the desktop the whole window moves with the cat.
      windowDragged.current = true
      drag.current = null
      tauri()?.window?.getCurrentWindow().startDragging().catch(() => {})
      return
    }
    setPos({ right: Math.min(Math.max(4, d.right - dx), window.innerWidth - 90), bottom: Math.min(Math.max(4, d.bottom - dy), window.innerHeight - 110) })
  }
  const onPointerUp = () => {
    const d = drag.current
    drag.current = null
    if (windowDragged.current) {
      windowDragged.current = false
      return
    }
    if (d?.moved) store.set('pimpo.mascot.pos', JSON.stringify(pos))
    else setMenu((m) => !m)
  }
  const title: TKey = bubble?.kind === 'approval' ? 'mascot.approval' : bubble?.kind === 'failure' ? 'mascot.failure' : bubble?.kind === 'task' ? 'mascot.task' : 'mascot.notice'
  const open = (path: string) => { setMenu(false); setBubble(null); onOpen(path) }

  return (
    <div className="pimpo-root pointer-events-none fixed z-40 flex flex-col items-end" style={standalone ? { right: 8, bottom: 8 } : { right: pos.right, bottom: pos.bottom }}>
      {bubble && (
        <div role="status" aria-live="polite" className="pimpo-bubble pointer-events-auto mb-2 w-[min(300px,calc(100vw-32px))] rounded-2xl border border-line bg-surface p-3.5 text-[13px] shadow-[var(--shadow-pop)]">
          <div className="mb-1 flex items-center justify-between gap-2 text-[12px] font-semibold text-ink">
            <span>🐾 {t(title)}</span>
            {bubble.count > 1 && <span className="rounded-full bg-sunken px-1.5 text-[11px] font-medium text-ink-3">+{bubble.count - 1}</span>}
          </div>
          <p className="line-clamp-4 whitespace-pre-line text-ink-2">{bubble.text}</p>
          <div className="mt-2.5 flex gap-2">
            <button type="button" className="rounded-lg bg-ink px-3 py-1.5 text-[12.5px] font-medium text-bg" onClick={() => open('/inbox')}>{t('mascot.see')}</button>
            <button type="button" className="rounded-lg px-3 py-1.5 text-[12.5px] text-ink-2 hover:bg-sunken" onClick={() => setBubble(null)}>{t('mascot.later')}</button>
          </div>
        </div>
      )}
      {menu && (
        <div ref={menuRef} role="menu" className="pointer-events-auto mb-2 w-56 rounded-xl border border-line bg-surface p-1 text-[13px] shadow-[var(--shadow-pop)]">
          <button role="menuitem" type="button" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 hover:bg-sunken" onClick={() => open('/')}><Plus size={15} /> {t('common.newTask')}</button>
          <button role="menuitem" type="button" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 hover:bg-sunken" onClick={() => open('/inbox')}><Inbox size={15} /> {t('nav.inbox')}{waiting > 0 && <span className="ml-auto rounded-full bg-change-soft px-1.5 text-[11px] text-change">{waiting}</span>}</button>
          <button role="menuitem" type="button" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 hover:bg-sunken" onClick={() => { store.set('pimpo.mascot.mute', String(Date.now() + 3600_000)); setMenu(false); setBubble(null) }}><BellOff size={15} /> {t('mascot.mute')}</button>
          {!standalone && <button role="menuitem" type="button" className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-ink-2 hover:bg-sunken" onClick={() => { setMenu(false); setMascotOn(false) }}><EyeOff size={15} /> {t('mascot.hide')}</button>}
        </div>
      )}
      <button ref={catRef} type="button" aria-label={t('mascot.label')} aria-haspopup="menu" aria-expanded={menu}
        className="pimpo-button pointer-events-auto h-[104px] w-[80px] cursor-grab touch-none select-none active:cursor-grabbing"
        onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp}
        onPointerEnter={() => { setPetting(true); lastActive.current = Date.now(); setMood((m) => (m === 'sleep' ? 'idle' : m)) }}
        onPointerLeave={() => setPetting(false)}>
        <Cat mood={mood} petting={petting} play={play} />
      </button>
    </div>
  )
}
