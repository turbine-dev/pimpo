import * as Dialog from '@radix-ui/react-dialog'
import { useMutation } from '@tanstack/react-query'
import { Activity, AlignLeft, BarChart3, Gauge, Hash, LayoutDashboard, List, Loader2, Sparkles, Table2, X } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { Button } from './ui'

const KINDS = [
  ['', Sparkles],
  ['metric', Hash],
  ['chart', BarChart3],
  ['table', Table2],
  ['list', List],
  ['progress', Gauge],
  ['status', Activity],
  ['text', AlignLeft],
] as const

// MakeWidget turns a routine into a widget: Pimpo re-explores the task so it
// also ends by showing its result, and the owner approves the new version as
// with any change. A routine that already shows one links to the dashboards.
export function MakeWidget({ id, shows }: { id: string; shows: boolean }) {
  const t = useT()
  const nav = useNavigate()
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState('')
  const make = useMutation({
    mutationFn: () => api.routineWidget(id, kind),
    onSuccess: (res) => { if (res.exploration) nav(`/explorations/${res.exploration}`) },
  })
  if (shows) {
    return <Link to="/dashboards"><Button variant="ghost"><LayoutDashboard size={15} /> {t('makeWidget.see')}</Button></Link>
  }
  return (
    <Dialog.Root open={open} onOpenChange={(o) => { setOpen(o); if (!o) make.reset() }}>
      <Dialog.Trigger asChild>
        <Button variant="ghost"><LayoutDashboard size={15} /> {t('makeWidget.button')}</Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed left-1/2 top-[14vh] z-50 w-[min(520px,calc(100vw-32px))] -translate-x-1/2 rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-[17px] font-semibold tracking-tight">{t('makeWidget.title')}</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm text-ink-2">{t('makeWidget.text')}</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <form onSubmit={(e) => { e.preventDefault(); make.mutate() }}>
            <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4" role="radiogroup" aria-label={t('makeWidget.kind')}>
              {KINDS.map(([k, Icon]) => (
                <button key={k} type="button" role="radio" aria-checked={kind === k} onClick={() => setKind(k)}
                  className={cn('flex flex-col items-center gap-1.5 rounded-xl border px-2 py-3 text-[12.5px]', kind === k ? 'border-ink bg-ink/5 font-medium text-ink' : 'border-line text-ink-2 hover:bg-sunken')}>
                  <Icon size={18} aria-hidden />
                  {t(`makeWidget.kind.${k || 'auto'}`)}
                </button>
              ))}
            </div>
            {make.error && <p className="mb-3 text-[13px] text-danger">{make.error.message}</p>}
            <div className="flex justify-end">
              <Button variant="primary" type="submit" disabled={make.isPending}>
                {make.isPending && <Loader2 size={14} className="animate-spin" />} {t('makeWidget.go')}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
