import * as Dialog from '@radix-ui/react-dialog'
import { useMutation } from '@tanstack/react-query'
import { ArrowRight, Sparkles, X } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { Button } from './ui'

const suggestions = ['newTask.suggestion1', 'newTask.suggestion2', 'newTask.suggestion3'] as const

export function NewTask({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const t = useT()
  const [text, setText] = useState('')
  const nav = useNavigate()
  const start = useMutation({
    mutationFn: () => api.explore(text),
    onSuccess: ({ id }) => {
      onOpenChange(false)
      setText('')
      nav(`/explorations/${id}`)
    },
  })
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px] data-[state=open]:animate-in" />
        <Dialog.Content className="fixed left-1/2 top-[14vh] z-50 w-[min(640px,calc(100vw-32px))] -translate-x-1/2 rounded-2xl border border-line bg-surface p-6 shadow-[var(--shadow-pop)] focus:outline-none">
          <div className="mb-4 flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-[17px] font-semibold tracking-tight">{t('newTask.title')}</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm text-ink-2">{t('newTask.text')}</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-8 place-items-center rounded-lg text-ink-3 hover:bg-sunken" aria-label={t('common.close')}>
              <X size={16} />
            </Dialog.Close>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              if (text.trim()) start.mutate()
            }}
          >
            <textarea
              autoFocus
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && text.trim()) start.mutate()
              }}
              rows={4}
              placeholder={t('newTask.placeholder')}
              aria-label={t('newTask.label')}
              className="w-full resize-none rounded-xl border border-line bg-bg px-4 py-3 text-[14.5px] leading-relaxed outline-none placeholder:text-ink-3 focus:border-accent"
            />
            <div className="mt-3 flex flex-col gap-1.5">
              {suggestions.map((k) => t(k)).map((s) => (
                <button type="button" key={s} onClick={() => setText(s)} className="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-[13px] text-ink-2 hover:bg-sunken hover:text-ink">
                  <Sparkles size={13} className="shrink-0 text-accent" /> {s}
                </button>
              ))}
            </div>
            {start.error && <p className="mt-3 text-sm text-danger">{start.error.message}</p>}
            <div className="mt-5 flex items-center justify-between">
              <span className="text-[12px] text-ink-3">{t('newTask.hint')}</span>
              <Button variant="primary" type="submit" disabled={!text.trim() || start.isPending}>
                {start.isPending ? t('newTask.starting') : t('newTask.start')} <ArrowRight size={15} />
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
