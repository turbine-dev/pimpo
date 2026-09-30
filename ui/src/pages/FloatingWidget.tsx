import { useQuery } from '@tanstack/react-query'
import { Maximize2, X } from 'lucide-react'
import { useEffect } from 'react'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { WidgetBody, WidgetCard } from '../components/widgets/Widget'

type TauriGlobal = { window?: { getCurrentWindow(): { startDragging(): Promise<void> } } }
const drag = () => (window as unknown as { __TAURI__?: TauriGlobal }).__TAURI__?.window?.getCurrentWindow().startDragging().catch(() => {})

// FloatingWidget is one widget alone in the desktop app's floating window:
// dragged by its card, resized from its edges, closed from its own button.
// Links go through /open, which the app follows in its main window.
export function FloatingWidget({ id }: { id: string }) {
  const t = useT()
  const q = useQuery({ queryKey: ['widgets', 'one', id], queryFn: () => api.widget(id), refetchInterval: 60_000 })
  useEffect(() => {
    document.body.style.background = 'transparent'
    document.documentElement.style.background = 'transparent'
    document.body.style.overflow = 'hidden'
  }, [])
  const close = () => window.location.assign(`/desktop/float?widget=${encodeURIComponent(id)}&on=0`)
  const open = () => window.location.assign('/open?path=' + encodeURIComponent('/dashboards'))
  const buttons = (
    <div className="absolute right-3 top-3 z-10 flex gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/float:opacity-100">
      <button type="button" onClick={open} aria-label={t('float.open')} className="grid size-7 place-items-center rounded-lg bg-surface/90 text-ink-3 hover:bg-sunken hover:text-ink"><Maximize2 size={13} /></button>
      <button type="button" onClick={close} aria-label={t('float.close')} className="grid size-7 place-items-center rounded-lg bg-surface/90 text-ink-3 hover:bg-sunken hover:text-ink"><X size={14} /></button>
    </div>
  )
  return (
    <div className="group/float relative h-screen p-1.5" onPointerDown={(e) => { if (e.button === 0 && !(e.target as Element).closest('button,a')) drag() }}>
      {buttons}
      {q.data ? (
        <WidgetCard w={q.data}><WidgetBody w={q.data} size="small" /></WidgetCard>
      ) : (
        <div className="flex h-full items-center justify-center rounded-[20px] border border-line bg-surface p-4 text-center text-[13px] text-ink-3">
          {q.error ? t('float.gone') : '…'}
        </div>
      )}
    </div>
  )
}
