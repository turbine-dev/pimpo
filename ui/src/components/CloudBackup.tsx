import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Cloud, CloudUpload, History, Loader2 } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api, type CloudConfig } from '../lib/api'
import { cn } from '../lib/cn'
import { relative, when } from '../lib/format'
import { useT } from '../lib/i18n'
import { Button, Card } from './ui'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

function size(n = 0) {
  return n < 1 << 20 ? `${Math.max(1, Math.round(n / 1024))} KB` : `${(n / (1 << 20)).toFixed(1)} MB`
}

// Automatic backups to the owner's S3 bucket or Google Drive, sealed with
// a passphrase on this computer before they leave.
export function CloudBackup() {
  const t = useT()
  const qc = useQueryClient()
  const state = useQuery({ queryKey: ['cloud'], queryFn: api.cloud })
  const [c, setC] = useState<CloudConfig>({ kind: '', every: 'daily', keep: 7 })
  const [keys, setKeys] = useState({ access_key: '', secret_key: '' })
  const [pass, setPass] = useState('')
  const [showFiles, setShowFiles] = useState(false)
  useEffect(() => { if (state.data) setC(state.data.config) }, [state.data])
  const refresh = (d?: unknown) => { if (d && typeof d === 'object' && 'config' in d) qc.setQueryData(['cloud'], d); qc.invalidateQueries({ queryKey: ['cloud'] }) }
  const save = useMutation({
    mutationFn: () => api.saveCloud({ ...c, ...(keys.access_key ? keys : {}), ...(pass ? { passphrase: pass } : {}) }),
    onSuccess: (d) => { refresh(d); setKeys({ access_key: '', secret_key: '' }); setPass('') },
  })
  const off = useMutation({ mutationFn: api.cloudOff, onSuccess: refresh })
  const run = useMutation({ mutationFn: api.cloudRun, onSettled: () => { refresh(); qc.invalidateQueries({ queryKey: ['cloud-files'] }) } })
  const s = state.data
  if (!s) return null
  const saved = s.config.kind !== ''
  const set = (p: Partial<CloudConfig>) => setC({ ...c, ...p })
  const driveBlocked = c.kind === 'drive' && (!s.google.connected || !s.google.drive)
  const ready = c.kind !== '' && !driveBlocked && (s.has_passphrase || pass.length >= 8) && (c.kind !== 's3' || (!!c.bucket && (s.has_keys || (!!keys.access_key && !!keys.secret_key))))

  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Cloud size={17} /> {t('cloud.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('cloud.text')}</p>

      <div role="radiogroup" aria-label={t('cloud.where')} className="mb-4 flex flex-wrap gap-2">
        {(['', 's3', 'drive'] as const).map((k) => (
          <button key={k} type="button" role="radio" aria-checked={c.kind === k} onClick={() => (k === '' && saved ? off.mutate() : set({ kind: k }))}
            className={cn('rounded-full border px-3 py-1.5 text-[12.5px] transition', c.kind === k ? 'border-ink bg-ink text-bg' : 'border-line text-ink-2 hover:border-line-strong')}>
            {t(k === '' ? 'cloud.off' : k === 's3' ? 'cloud.s3' : 'cloud.drive')}
          </button>
        ))}
      </div>

      {c.kind !== '' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          {c.kind === 's3' && (
            <div className="grid gap-3 sm:grid-cols-2">
              <Label text={t('cloud.bucket')}><input className={field} value={c.bucket ?? ''} onChange={(e) => set({ bucket: e.target.value })} autoComplete="off" /></Label>
              <Label text={t('cloud.region')}><input className={field} value={c.region ?? ''} onChange={(e) => set({ region: e.target.value })} placeholder={t('cloud.regionHint')} autoComplete="off" /></Label>
              <Label text={t('cloud.endpoint')}><input className={field} value={c.endpoint ?? ''} onChange={(e) => set({ endpoint: e.target.value })} placeholder={t('cloud.endpointHint')} inputMode="url" autoComplete="off" /></Label>
              <Label text={t('cloud.prefix')}><input className={field} value={c.prefix ?? ''} onChange={(e) => set({ prefix: e.target.value })} placeholder="pimpo/" autoComplete="off" /></Label>
              <Label text={t('cloud.access')}><input className={field} value={keys.access_key} onChange={(e) => setKeys({ ...keys, access_key: e.target.value })} placeholder={s.has_keys ? t('cloud.keySaved') : ''} autoComplete="off" /></Label>
              <Label text={t('cloud.secret')}><input className={field} type="password" value={keys.secret_key} onChange={(e) => setKeys({ ...keys, secret_key: e.target.value })} placeholder={s.has_keys ? t('cloud.keySaved') : ''} autoComplete="new-password" /></Label>
            </div>
          )}
          {c.kind === 'drive' && (
            <p className={cn('rounded-lg px-3 py-2 text-[13px]', driveBlocked ? 'bg-sunken text-ink-2' : 'bg-read-soft text-read')}>
              {!s.google.connected ? t('cloud.driveNeedsGoogle') : !s.google.drive ? t('cloud.driveNeedsScope') : t('cloud.driveReady')}
              {driveBlocked && <> <Link to="/connections" className="font-medium underline">{t('cloud.goConnections')}</Link></>}
            </p>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <Label text={t('cloud.every')}>
              <select className={field} value={c.every} onChange={(e) => set({ every: e.target.value as CloudConfig['every'] })}>
                <option value="daily">{t('cloud.daily')}</option>
                <option value="weekly">{t('cloud.weekly')}</option>
              </select>
            </Label>
            <Label text={t('cloud.keep')}><input className={field} type="number" min={1} max={90} value={c.keep} onChange={(e) => set({ keep: Number(e.target.value) })} /></Label>
          </div>
          <Label text={t('cloud.pass')}>
            <input className={field} type="password" value={pass} onChange={(e) => setPass(e.target.value)} placeholder={s.has_passphrase ? t('cloud.passSaved') : t('cloud.passNew')} autoComplete="new-password" />
          </Label>
          <p className="text-[12.5px] text-ink-3">{t('cloud.passWarn')}</p>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" variant="primary" disabled={!ready || save.isPending}>{save.isPending && <Loader2 size={14} className="animate-spin" />} {t('cloud.save')}</Button>
            {save.isSuccess && <span className="text-[13px] text-read">{t('cloud.saved')}</span>}
          </div>
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
        </form>
      )}

      {saved && (
        <div className="mt-4 space-y-3 border-t border-line pt-4">
          <div className="text-[13px]">
            {!s.last ? <span className="text-ink-3">{t('cloud.never')}</span>
              : s.last.ok ? <span className="text-ink-2">{t('cloud.last', { when: relative(s.last.at), size: size(s.last.size) })}</span>
                : <span className="text-danger">{t('cloud.lastFailed', { when: relative(s.last.at), error: s.last.error ?? '' })}</span>}
            {s.next && <span className="text-ink-3"> · {t('cloud.next', { when: when(s.next) })}</span>}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => run.mutate()} disabled={run.isPending}>
              {run.isPending ? <Loader2 size={14} className="animate-spin" /> : <CloudUpload size={14} />} {t('cloud.now')}
            </Button>
            <Button size="sm" variant="ghost" aria-expanded={showFiles} onClick={() => setShowFiles(!showFiles)}><History size={14} /> {t('cloud.files')}</Button>
          </div>
          {run.error && <p className="text-[13px] text-danger">{run.error.message}</p>}
          {showFiles && <CloudFiles />}
        </div>
      )}
    </Card>
  )
}

function Label({ text, children }: { text: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[12.5px] text-ink-2">{text}</span>
      {children}
    </label>
  )
}

function CloudFiles() {
  const t = useT()
  const files = useQuery({ queryKey: ['cloud-files'], queryFn: api.cloudFiles })
  const [picked, setPicked] = useState('')
  const [other, setOther] = useState('')
  const restore = useMutation({ mutationFn: () => api.cloudRestore(picked, other || undefined) })
  if (files.isPending) return <Loader2 size={16} className="animate-spin text-ink-3" />
  if (files.error) return <p className="text-[13px] text-danger">{files.error.message}</p>
  if (!files.data?.length) return <p className="text-[13px] text-ink-3">{t('cloud.noFiles')}</p>
  return (
    <div className="space-y-3">
      <ul className="divide-y divide-line rounded-xl border border-line">
        {files.data.map((f) => (
          <li key={f.name} className="flex items-center gap-3 px-3 py-2 text-[13px]">
            <span className="flex-1">{when(stamp(f.name) ?? f.modified)}</span>
            <span className="text-[12px] text-ink-3 tabular-nums">{size(f.size)}</span>
            <Button size="sm" variant={picked === f.name ? 'primary' : 'ghost'} aria-label={t('cloud.restoreOf', { when: when(stamp(f.name) ?? f.modified) })}
              onClick={() => { setPicked(f.name); restore.reset() }}>{t('cloud.restore')}</Button>
          </li>
        ))}
      </ul>
      {picked && (
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); restore.mutate() }}>
          <input type="password" className={cn(field, 'min-w-[220px] flex-1')} value={other} onChange={(e) => setOther(e.target.value)} placeholder={t('cloud.otherPass')} aria-label={t('cloud.otherPass')} />
          <Button type="submit" disabled={restore.isPending}>{t('cloud.restoreOf', { when: when(stamp(picked) ?? '') })}</Button>
        </form>
      )}
      {restore.error && <p className="text-[13px] text-danger">{restore.error.message}</p>}
      {restore.data && <p className="rounded-lg bg-read-soft px-3 py-2 text-[13px] text-read">{t('backup.checked', { count: restore.data.secrets })}</p>}
    </div>
  )
}

// stamp reads the UTC time in a backup's name (pimpo-20260925-120000.pimpo).
function stamp(name: string) {
  const m = name.match(/^pimpo-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})\.pimpo$/)
  return m ? `${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}Z` : undefined
}
