import { ArrowDownToLine, Loader2, RotateCcw, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useT } from '../lib/i18n'
import { inDesktopApp } from './Mascot'
import { Button, Card, Switch } from './ui'

// The desktop app tells the page what it knows about updates, as it does
// for the floating Pimpo: in localStorage, with a pimpo:update event. The
// page asks by visiting /desktop/update; it has no native access, and
// what gets installed is the signed update the app itself found.

export type UpdateState = { current: string; found?: string | null; notes?: string | null; beta: boolean; previous?: string | null; busy: boolean; error: string }

function read(): UpdateState | null {
  try {
    const raw = localStorage.getItem('pimpo.update')
    return raw ? (JSON.parse(raw) as UpdateState) : null
  } catch {
    return null
  }
}

export function useDesktopUpdate() {
  const [s, setS] = useState<UpdateState | null>(read)
  useEffect(() => {
    const sync = () => setS(read())
    window.addEventListener('pimpo:update', sync)
    return () => window.removeEventListener('pimpo:update', sync)
  }, [])
  return s
}

export const askDesktop = (what: 'check' | 'install' | 'rollback' | 'beta' | 'stable') => window.location.assign('/desktop/update?do=' + what)

// UpdateCard is in Ajustes › Geral, in the desktop app only.
export function UpdateCard() {
  const t = useT()
  const s = useDesktopUpdate()
  const [sure, setSure] = useState(false)
  if (!inDesktopApp() || !s) return null
  return (
    <Card className="space-y-3 p-5">
      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <div className="text-[15px] font-medium">{t('upd.title')}</div>
          <p className="text-[13px] text-ink-3">{t('upd.current', { version: s.current })}</p>
        </div>
        <Switch on={s.beta} label={t('upd.beta')} onChange={(v) => askDesktop(v ? 'beta' : 'stable')} />
      </div>
      <p className="text-[12.5px] text-ink-3">{t('upd.betaText')}</p>
      {s.found ? (
        <div className="space-y-2 rounded-xl bg-read-soft p-3">
          <p className="text-[13.5px] text-read">{t('upd.found', { version: s.found })}</p>
          {s.notes && <p className="line-clamp-4 whitespace-pre-line text-[12.5px] text-ink-2">{s.notes}</p>}
          <Button variant="primary" size="sm" onClick={() => askDesktop('install')} disabled={s.busy}>
            {s.busy ? <Loader2 size={14} className="animate-spin" /> : <ArrowDownToLine size={14} />} {t('upd.install')}
          </Button>
          <p className="text-[11.5px] text-ink-3">{t('upd.safe')}</p>
        </div>
      ) : (
        <Button size="sm" onClick={() => askDesktop('check')} disabled={s.busy}>
          {s.busy && <Loader2 size={14} className="animate-spin" />} {t('upd.check')}
        </Button>
      )}
      {s.error && <p className="text-[12.5px] text-danger">{s.error}</p>}
      {s.previous && (
        <div className="border-t border-line pt-3">
          {sure ? (
            <div className="space-y-2">
              <p className="text-[12.5px] text-ink-2">{t('upd.rollbackSure', { version: s.previous })}</p>
              <div className="flex gap-2">
                <Button size="sm" variant="danger" onClick={() => askDesktop('rollback')} disabled={s.busy}>{t('upd.rollbackYes')}</Button>
                <Button size="sm" variant="ghost" onClick={() => setSure(false)}>{t('common.cancel')}</Button>
              </div>
            </div>
          ) : (
            <Button size="sm" variant="ghost" onClick={() => setSure(true)}><RotateCcw size={14} /> {t('upd.rollback', { version: s.previous })}</Button>
          )}
        </div>
      )}
    </Card>
  )
}

// UpdateBanner says, once per version, that an update is ready.
export function UpdateBanner() {
  const t = useT()
  const s = useDesktopUpdate()
  const [hidden, setHidden] = useState(() => {
    try {
      return localStorage.getItem('pimpo.update.hidden') ?? ''
    } catch {
      return ''
    }
  })
  if (!inDesktopApp() || !s?.found || hidden === s.found) return null
  const hide = () => {
    try {
      localStorage.setItem('pimpo.update.hidden', s.found!)
    } catch {
      /* private mode */
    }
    setHidden(s.found!)
  }
  return (
    <div role="status" className="mb-4 flex items-center gap-3 rounded-xl border border-line bg-surface px-4 py-2.5 text-[13.5px]">
      <ArrowDownToLine size={16} className="shrink-0 text-accent" />
      <span className="min-w-0 flex-1">{t('upd.found', { version: s.found })}</span>
      <Button size="sm" variant="primary" onClick={() => askDesktop('install')} disabled={s.busy}>
        {s.busy && <Loader2 size={14} className="animate-spin" />} {t('upd.install')}
      </Button>
      <button type="button" onClick={hide} aria-label={t('common.close')} className="grid size-7 place-items-center rounded-lg text-ink-3 hover:bg-sunken"><X size={15} /></button>
    </div>
  )
}
