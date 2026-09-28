import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, RefreshCw, Webhook } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card, Switch } from './ui'

// WebhookCard lets another service start the routine by calling a secret
// address: an iPhone Shortcut, IFTTT or Zapier, GitHub, a form.
export function WebhookCard({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['webhook', id], queryFn: () => api.webhook(id) })
  const act = useMutation({ mutationFn: (a: 'on' | 'off' | 'rotate') => api.setWebhook(id, a), onSuccess: (d) => qc.setQueryData(['webhook', id], d) })
  const [copied, setCopied] = useState('')
  const urls = q.data?.urls
  const on = !!urls
  const copy = (k: string, u: string) => { navigator.clipboard?.writeText(u); setCopied(k); setTimeout(() => setCopied(''), 1500) }
  return (
    <Card className="mb-6 p-5">
      <div className="flex items-start gap-3">
        <div className="grid size-9 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore"><Webhook size={17} /></div>
        <div className="min-w-0 flex-1">
          <h2 className="text-[15px] font-medium">{t('wh.title')}</h2>
          <p className="text-[13px] text-ink-3">{t('wh.text')}</p>
        </div>
        <Switch on={on} label={t('wh.title')} disabled={act.isPending || q.isLoading} onChange={(v) => act.mutate(v ? 'on' : 'off')} />
      </div>
      {on && (
        <div className="mt-4 space-y-2">
          {(['public', 'lan', 'local'] as const).filter((k) => urls![k]).map((k) => (
            <div key={k} className="flex items-center gap-2">
              <span className="w-28 shrink-0 text-[12px] text-ink-3">{t(`wh.${k}` as TKey)}</span>
              <code className="min-w-0 flex-1 truncate rounded-lg bg-sunken px-2 py-1 font-mono text-[12px]" title={urls![k]}>{urls![k]}</code>
              <Button size="sm" variant="ghost" aria-label={t('wh.copy')} onClick={() => copy(k, urls![k]!)}>{copied === k ? <Check size={13} className="text-read" /> : <Copy size={13} />}</Button>
            </div>
          ))}
          <p className="text-[12px] text-ink-3">{urls!.public ? t('wh.howPublic') : t('wh.howLocal')}</p>
          <p className="text-[12px] text-ink-3">{t('wh.event')}</p>
          <Button size="sm" variant="ghost" onClick={() => act.mutate('rotate')}><RefreshCw size={13} /> {t('wh.rotate')}</Button>
        </div>
      )}
      {act.error && <p className="mt-2 text-[12.5px] text-danger">{act.error.message}</p>}
    </Card>
  )
}
