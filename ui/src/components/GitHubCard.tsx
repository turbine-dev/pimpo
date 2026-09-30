import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, GitBranch, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { api, type GitHubHook } from '../lib/api'
import { useT, type TKey } from '../lib/i18n'
import { Button, Card, Switch } from './ui'

// GitHubCard gives the routine a GitHub webhook: an address and a secret
// GitHub signs each delivery with. The secret is shown once, when made.
export function GitHubCard({ id }: { id: string }) {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['github-hook', id], queryFn: () => api.githubHook(id) })
  const [secret, setSecret] = useState('')
  const act = useMutation({
    mutationFn: (a: 'on' | 'off' | 'rotate') => api.setGitHubHook(id, a),
    onSuccess: (d: GitHubHook) => { setSecret(d.secret ?? ''); qc.setQueryData(['github-hook', id], { on: d.on, urls: d.urls }) },
  })
  const [copied, setCopied] = useState('')
  const copy = (k: string, v: string) => { navigator.clipboard?.writeText(v); setCopied(k); setTimeout(() => setCopied(''), 1500) }
  const on = !!q.data?.on
  const urls = q.data?.urls ?? {}
  const row = (k: string, label: string, value: string, copyLabel: string) => (
    <div key={k} className="flex items-center gap-2">
      <span className="w-28 shrink-0 text-[12px] text-ink-3">{label}</span>
      <code className="min-w-0 flex-1 truncate rounded-lg bg-sunken px-2 py-1 font-mono text-[12px]" title={value}>{value}</code>
      <Button size="sm" variant="ghost" aria-label={copyLabel} onClick={() => copy(k, value)}>{copied === k ? <Check size={13} className="text-read" /> : <Copy size={13} />}</Button>
    </div>
  )
  return (
    <Card className="mb-6 p-5">
      <div className="flex items-start gap-3">
        <div className="grid size-9 shrink-0 place-items-center rounded-xl bg-explore-soft text-explore"><GitBranch size={17} /></div>
        <div className="min-w-0 flex-1">
          <h2 className="text-[15px] font-medium">{t('gh.title')}</h2>
          <p className="text-[13px] text-ink-3">{t('gh.text')}</p>
        </div>
        <Switch on={on} label={t('gh.title')} disabled={act.isPending || q.isLoading} onChange={(v) => act.mutate(v ? 'on' : 'off')} />
      </div>
      {on && (
        <div className="mt-4 space-y-2">
          {(['public', 'local'] as const).filter((k) => urls[k]).map((k) => row(k, t(`wh.${k}` as TKey), urls[k]!, t('wh.copy')))}
          {secret && (
            <>
              {row('secret', t('gh.secret'), secret, t('gh.copySecret'))}
              <p className="text-[12px] font-medium text-ink-2" role="status">{t('gh.secretOnce')}</p>
            </>
          )}
          <p className="text-[12px] text-ink-3">{urls.public ? t('gh.how') : t('wh.howLocal')}</p>
          <p className="text-[12px] text-ink-3">{t('gh.event')}</p>
          <Button size="sm" variant="ghost" onClick={() => act.mutate('rotate')}><RefreshCw size={13} /> {t('gh.rotate')}</Button>
        </div>
      )}
      {act.error && <p className="mt-2 text-[12.5px] text-danger">{act.error.message}</p>}
    </Card>
  )
}
