import { useMutation, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { Logo } from '../components/Shell'
import { Button } from '../components/ui'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { addPasskey, canUsePasskeys } from '../lib/passkey'

// CreateAdmin is the first visit: the person who installed Pimpo makes the
// administrator's account, and a passkey to come back with, before anything
// else.
export function CreateAdmin({ onStart, onDone }: { onStart?: () => void; onDone?: () => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [step, setStep] = useState<'name' | 'passkey'>('name')
  const finish = () => { qc.invalidateQueries({ queryKey: ['state'] }); onDone?.() }
  // Saving the name tells the app the account exists; the screen asks to
  // stay until the passkey step is answered.
  const save = useMutation({ mutationFn: () => { onStart?.(); return api.saveAccount(name.trim()) }, onSuccess: () => (canUsePasskeys() ? setStep('passkey') : finish()), onError: () => onDone?.() })
  const passkey = useMutation({ mutationFn: () => addPasskey(t('acct.defaultName')), onSuccess: finish })
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center px-4 text-center">
      <Logo size={56} />
      {step === 'name' ? (
        <>
          <h1 className="mt-4 text-[22px] font-semibold tracking-tight">{t('admin.title')}</h1>
          <p className="mt-1 max-w-md text-[13.5px] text-ink-2">{t('admin.text')}</p>
          <form className="mt-6 flex w-full max-w-sm flex-col gap-2" onSubmit={(e) => { e.preventDefault(); if (name.trim()) save.mutate() }}>
            <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={t('admin.name')} aria-label={t('admin.name')}
              className="h-11 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
            <Button type="submit" variant="primary" disabled={!name.trim() || save.isPending}>{save.isPending && <Loader2 size={14} className="animate-spin" />} {t('admin.create')}</Button>
          </form>
          {save.error && <p className="mt-3 text-[13px] text-danger">{save.error.message}</p>}
        </>
      ) : (
        <>
          <h1 className="mt-4 text-[22px] font-semibold tracking-tight">{t('admin.passkeyTitle')}</h1>
          <p className="mt-1 max-w-md text-[13.5px] text-ink-2">{t('admin.passkeyText')}</p>
          <div className="mt-6 flex flex-wrap justify-center gap-2">
            <Button variant="primary" onClick={() => passkey.mutate()} disabled={passkey.isPending}>
              {passkey.isPending ? <Loader2 size={14} className="animate-spin" /> : <KeyRound size={14} />} {t('acct.add')}
            </Button>
            <Button variant="ghost" onClick={finish}>{t('admin.later')}</Button>
          </div>
          {passkey.error && <p className="mt-3 max-w-md text-[13px] text-danger">{passkey.error.message}</p>}
        </>
      )}
    </div>
  )
}
