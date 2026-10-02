import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useT } from '../lib/i18n'

// Modal is a dialog with a title. A footer stays in view below what
// scrolls, for the dialog's main actions.
export function Modal({ title, onClose, children, footer }: { title: string; onClose: () => void; children: ReactNode; footer?: ReactNode }) {
  const t = useT()
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
        <Dialog.Content aria-describedby={undefined} className="fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(620px,calc(100vw-32px))] -translate-x-1/2 flex-col rounded-2xl border border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="flex items-start justify-between gap-4 px-6 pb-4 pt-6">
            <Dialog.Title className="min-w-0 truncate text-[17px] font-semibold tracking-tight">{title}</Dialog.Title>
            <Dialog.Close className="grid size-8 shrink-0 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}><X size={16} /></Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-6">{children}</div>
          {footer && <div className="flex flex-wrap items-center gap-2 rounded-b-2xl border-t border-line bg-surface px-6 py-3">{footer}</div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block space-y-1"><span className="text-[12.5px] text-ink-2">{label}</span>{children}</label>
}

