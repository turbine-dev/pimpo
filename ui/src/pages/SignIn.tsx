import { KeyRound, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { Logo } from '../components/Shell'
import { useT } from '../lib/i18n'
import { canUsePasskeys, passkeyMessage, signIn } from '../lib/passkey'
import { Button } from '../components/ui'

// SignIn is what someone not signed in sees: a passkey, or the link.
export function SignIn() {
  const t = useT()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const go = async () => {
    setBusy(true)
    setError('')
    try {
      await signIn()
      window.location.assign('/')
    } catch (e) {
      setError(passkeyMessage(e, t))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center px-4 text-center">
      <Logo size={56} />
      <h1 className="mt-4 text-[22px] font-semibold tracking-tight">{t('signin.title')}</h1>
      <p className="mt-1 max-w-sm text-[13.5px] text-ink-2">{t('signin.text')}</p>
      {canUsePasskeys() && (
        <Button className="mt-6" variant="primary" onClick={go} disabled={busy}>
          {busy ? <Loader2 size={15} className="animate-spin" /> : <KeyRound size={15} />} {t('signin.passkey')}
        </Button>
      )}
      {error && <p className="mt-3 max-w-sm text-[13px] text-danger">{error}</p>}
    </div>
  )
}
