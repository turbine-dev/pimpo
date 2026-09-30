import { useMutation } from '@tanstack/react-query'
import { Check, Link2, Loader2 } from 'lucide-react'
import { type InputHTMLAttributes, type KeyboardEvent, type MouseEvent, type ReactNode, useState } from 'react'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'

// isReference tells a reference to an outside password manager from a secret.
export const isReference = (v: string) => /^(op|vault):\/\//.test(v.trim())

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange' | 'type'> & {
  value: string
  onValue: (v: string) => void
  /** Classes of the box around the input and its controls (width, flex). */
  box?: string
}

// SecretInput is a secret field that can hold a reference to 1Password or
// HashiCorp Vault instead of the secret. A reference is checked by reading
// it once on the server, which answers only whether it was found.
export function SecretInput({ value, onValue, box, className, placeholder, ...p }: Props) {
  const t = useT()
  const [ref, setRef] = useState(() => isReference(value))
  const check = useMutation({ mutationFn: () => api.checkReference(value.trim()) })
  const toggle = () => { setRef(!ref); onValue(''); check.reset() }
  return (
    <span className={cn('flex flex-col gap-1', box)}>
      <span className="flex items-center gap-1.5">
        <input {...p} type={ref ? 'text' : 'password'} spellCheck={false} value={value} placeholder={ref ? t('ref.placeholder') : placeholder}
          onChange={(e) => { onValue(e.target.value); check.reset() }} className={cn(className, 'min-w-0 flex-1', ref && 'font-mono text-[12.5px]')} />
        {/* Not a <button>: these sit inside the field's <label>, which must
            name only the input. */}
        <Act role="switch" checked={ref} label={t('ref.use')} onAct={toggle}
          className={cn('grid size-8 shrink-0 place-items-center rounded-lg hover:bg-sunken', ref ? 'bg-sunken text-ink' : 'text-ink-3')}>
          <Link2 size={15} />
        </Act>
      </span>
      {ref && (
        <span className="flex flex-wrap items-center gap-2 text-[12px]">
          <Act role="button" label={t('ref.check')} disabled={!value.trim() || check.isPending} onAct={() => check.mutate()}
            className="inline-flex h-7 items-center gap-1 rounded-lg border border-line px-2.5 font-medium hover:bg-sunken aria-disabled:opacity-50">
            {check.isPending && <Loader2 size={12} className="animate-spin" />} {t('ref.check')}
          </Act>
          {check.isSuccess && <span role="status" className="flex items-center gap-1 text-read"><Check size={12} /> {t('ref.found')}</span>}
          {check.error && <span role="alert" className="text-danger">{check.error.message}</span>}
          {!check.isSuccess && !check.error && <span className="text-ink-3">{t('ref.hint')}</span>}
        </span>
      )}
    </span>
  )
}

function Act({ role, checked, label, disabled, onAct, className, children }: { role: 'switch' | 'button'; checked?: boolean; label: string; disabled?: boolean; onAct: () => void; className: string; children: ReactNode }) {
  const act = (e: MouseEvent | KeyboardEvent) => {
    e.preventDefault()
    if (!disabled) onAct()
  }
  return (
    <span role={role} tabIndex={0} aria-label={label} aria-checked={role === 'switch' ? checked : undefined} aria-disabled={disabled || undefined} title={label}
      onClick={act} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') act(e) }} className={cn('cursor-pointer select-none outline-none focus-visible:ring-2 focus-visible:ring-accent', className)}>
      {children}
    </span>
  )
}
