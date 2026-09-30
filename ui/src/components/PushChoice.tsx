import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Zap } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { Button, Switch } from './ui'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// PushChoice lets a routine that watches Gmail or Slack hear of new items
// as they happen, and says whether that is working right now.
export function PushChoice({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['push', id], queryFn: () => api.routinePush(id) })
  const set = useMutation({ mutationFn: (on: boolean) => api.setRoutinePush(id, on), onSuccess: (d) => qc.setQueryData(['push', id], d) })
  const p = q.data
  if (!p || !p.kind) return null
  const g = p.gmail
  let status = ''
  if (p.on && p.live) status = p.kind === 'gmail' ? t('push.gmailLive', { date: g?.until ? new Date(g.until).toLocaleDateString() : '' }) : t('push.slackLive')
  else if (p.on && p.kind === 'gmail') status = g?.error ? t('push.failed', { error: g.error }) : !g?.configured ? '' : !g.signed_in ? t('push.gmailSignIn') : ''
  else if (p.on && p.kind === 'slack') status = p.slack?.owner_only ? t('push.slackOwnerOnly') : t('push.slackOff')
  return (
    <div className="mt-3 rounded-xl border border-line p-3">
      <div className="flex items-start gap-3">
        <Zap size={15} className="mt-0.5 shrink-0 text-explore" />
        <div className="min-w-0 flex-1">
          <div className="text-[13px] font-medium">{t('push.title')}</div>
          <p className="text-[12px] text-ink-3">{p.kind === 'gmail' ? t('push.gmailText') : t('push.slackText')}</p>
        </div>
        <Switch on={p.on} label={t('push.title')} disabled={set.isPending} onChange={(v) => set.mutate(v)} />
      </div>
      {p.on && (
        <div className="mt-2 space-y-1 text-[12px]" role="status">
          {status && <p className={p.live ? 'text-read' : 'text-ink-2'}>{status}</p>}
          {p.kind === 'gmail' && !g?.configured && <GmailSetup onSaved={() => set.mutate(true)} />}
          <p className="text-ink-3">{p.live ? t('push.safety') : t('push.polling')}</p>
        </div>
      )}
      {set.error && <p className="mt-2 text-[12.5px] text-danger">{set.error.message}</p>}
    </div>
  )
}

// GmailSetup is the administrator's one-time Google Cloud setup; others
// are told to ask for it.
function GmailSetup({ onSaved }: { onSaved: () => void }) {
  const t = useT()
  // The default role while loading is the owner's; wait to know.
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const role = state.data?.role
  const q = useQuery({ queryKey: ['gmail-push'], queryFn: api.gmailPush, enabled: role === 'owner' })
  const [topic, setTopic] = useState('')
  const [account, setAccount] = useState('')
  const [copied, setCopied] = useState(false)
  useEffect(() => { setTopic(q.data?.topic ?? ''); setAccount(q.data?.account ?? '') }, [q.data?.topic, q.data?.account])
  const save = useMutation({ mutationFn: () => api.setGmailPush({ topic, account }), onSuccess: (d) => { if (d.ready) onSaved() } })
  if (!role) return null
  if (role !== 'owner') return <p className="text-ink-2">{t('push.gmailNeedsAdmin')}</p>
  const endpoint = q.data?.endpoint
  return (
    // Not a form of its own: it sits inside the routine's settings form.
    <div className="mt-2 space-y-2 rounded-lg bg-sunken p-3" role="group" aria-label={t('push.setup.title')}>
      <div className="text-[13px] font-medium text-ink">{t('push.setup.title')}</div>
      <p className="text-ink-3">{t('push.setup.help')}</p>
      <label className="block"><span className="mb-1 block text-ink-2">{t('push.setup.topic')}</span>
        <input className={field} value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="projects/my-project/topics/pimpo-gmail" />
      </label>
      <label className="block"><span className="mb-1 block text-ink-2">{t('push.setup.account')}</span>
        <input className={field} value={account} onChange={(e) => setAccount(e.target.value)} placeholder="pimpo-push@my-project.iam.gserviceaccount.com" />
      </label>
      <div>
        <span className="mb-1 block text-ink-2">{t('push.setup.endpoint')}</span>
        {endpoint ? (
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-lg bg-bg px-2 py-1 font-mono text-[12px]" title={endpoint}>{endpoint}</code>
            <Button type="button" size="sm" variant="ghost" aria-label={t('wh.copy')} onClick={() => { navigator.clipboard?.writeText(endpoint); setCopied(true); setTimeout(() => setCopied(false), 1500) }}>
              {copied ? <Check size={13} className="text-read" /> : <Copy size={13} />}
            </Button>
          </div>
        ) : <p className="text-ink-2">{t('push.setup.noPublic')}</p>}
      </div>
      {save.error && <p className="text-danger">{save.error.message}</p>}
      <div className="flex justify-end"><Button type="button" size="sm" variant="primary" disabled={save.isPending || !topic || !account} onClick={() => save.mutate()}>{t('push.setup.save')}</Button></div>
    </div>
  )
}
