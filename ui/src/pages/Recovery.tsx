import { useMutation, useQuery } from '@tanstack/react-query'
import { Download, Loader2, RotateCcw, ShieldAlert, Sparkles } from 'lucide-react'
import { useEffect, type ReactNode } from 'react'
import { Logo } from '../components/Shell'
import { Button, Card } from '../components/ui'
import { api, ApiError, type Snapshot } from '../lib/api'
import { date } from '../lib/format'
import { useT } from '../lib/i18n'

const when = (s: Snapshot) => date(s.when, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })

// Recovery is all Pimpo shows while its damaged database is set aside:
// go back to the newest copy that passes its check, start fresh, or take
// the damaged file away. Only the administrator gets past the sign-in.
export function Recovery() {
  const t = useT()
  const q = useQuery({ queryKey: ['recovery'], queryFn: api.recovery })
  const restore = useMutation({ mutationFn: api.recoverFrom })
  const fresh = useMutation({ mutationFn: api.startFresh })
  const done = restore.isSuccess || fresh.isSuccess
  const busy = restore.isPending || fresh.isPending
  const error = restore.error ?? fresh.error

  // Once a choice is made Pimpo starts normally; open it when it answers.
  useEffect(() => {
    if (!done) return
    const timer = setInterval(async () => {
      try {
        await api.state()
      } catch (e) {
        // Still restarting, or not yet out of recovery.
        if (!(e instanceof ApiError) || e.recovery) return
      }
      window.location.assign('/')
    }, 1000)
    return () => clearInterval(timer)
  }, [done])

  if (q.error instanceof ApiError && q.error.status === 401) {
    return (
      <Page>
        <p className="mt-4 max-w-md text-[13.5px] text-ink-2">{t('rec.signIn')}</p>
      </Page>
    )
  }
  const list = q.data?.snapshots ?? []
  const newest = list.find((s) => s.name === q.data?.newest_good)
  const others = list.filter((s) => !s.damaged && s.name !== newest?.name)

  return (
    <Page>
      {q.data && <p className="mt-2 max-w-lg text-[13.5px] text-ink-2">{t('rec.text', { folder: q.data.folder })}</p>}
      {q.data?.reason && <p className="mt-2 max-w-lg break-words text-[12px] text-ink-3">{t('rec.reason', { reason: q.data.reason })}</p>}
      {done ? (
        <p role="status" className="mt-6 flex items-center gap-2 text-[14px]"><Loader2 size={15} className="animate-spin" /> {t('rec.done')}</p>
      ) : q.data && (
        <div className="mt-6 grid w-full max-w-lg gap-3 text-left">
          <Card className="p-5">
            {newest ? (
              <>
                <Button variant="primary" disabled={busy} onClick={() => restore.mutate(newest.name)}>
                  <RotateCcw size={15} /> {t('rec.restore', { when: when(newest) })}
                </Button>
                <p className="mt-2 text-[12.5px] text-ink-3">{t('rec.restoreText')}</p>
                {others.length > 0 && (
                  <details className="mt-3 text-[13px]">
                    <summary className="cursor-pointer text-ink-2">{t('rec.others')}</summary>
                    <ul className="mt-2 divide-y divide-line rounded-xl border border-line">
                      {others.map((s) => (
                        <li key={s.name} className="flex items-center gap-3 px-3 py-2">
                          <span className="flex-1 tabular-nums text-ink-2">{when(s)}</span>
                          <Button size="sm" variant="ghost" disabled={busy} onClick={() => restore.mutate(s.name)}>
                            <RotateCcw size={14} /> {t('rec.restore', { when: when(s) })}
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </details>
                )}
              </>
            ) : (
              <p className="text-[13px] text-ink-2">{t('rec.none')}</p>
            )}
          </Card>
          <Card className="p-5">
            <Button disabled={busy} onClick={() => window.confirm(t('rec.freshConfirm')) && fresh.mutate()}>
              <Sparkles size={15} /> {t('rec.fresh')}
            </Button>
            <p className="mt-2 text-[12.5px] text-ink-3">{t('rec.freshText')}</p>
          </Card>
          <Card className="p-5">
            <a href="/api/recovery/export" download className="inline-flex items-center gap-2 text-[13.5px] font-medium text-ink underline-offset-2 hover:underline">
              <Download size={15} /> {t('rec.export')}
            </a>
            <p className="mt-2 text-[12.5px] text-ink-3">{t('rec.exportText')}</p>
          </Card>
          {error && <p role="alert" className="text-[13px] text-danger">{t('rec.failed', { error: error.message })}</p>}
        </div>
      )}
    </Page>
  )
}

function Page({ children }: { children: ReactNode }) {
  const t = useT()
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center px-4 py-10 text-center">
      <Logo size={48} />
      <h1 className="mt-4 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><ShieldAlert size={20} className="text-danger" /> {t('rec.title')}</h1>
      {children}
    </main>
  )
}
