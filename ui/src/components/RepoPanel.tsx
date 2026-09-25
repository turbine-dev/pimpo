import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, CircleAlert, Download, FolderGit2, Upload } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, type RepoView } from '../lib/api'
import { useT } from '../lib/i18n'
import { capabilityLabel } from './RoutineCard'
import { Button, Card } from './ui'

const field = 'h-10 min-w-0 flex-1 rounded-[10px] border border-line bg-bg px-3 font-mono text-[13px] outline-none focus:border-accent'

// RepoPanel keeps routines in a folder the owner controls, usually a git
// repository: send them there, and bring back changes after their checks.
export function RepoPanel() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['repo'], queryFn: api.repo })
  const [path, setPath] = useState('')
  useEffect(() => { if (q.data) setPath(q.data.path) }, [q.data?.path])
  const put = (v: RepoView) => qc.setQueryData(['repo'], v)
  const save = useMutation({ mutationFn: () => api.setRepo(path), onSuccess: put })
  const exp = useMutation({ mutationFn: api.repoExport, onSuccess: (r) => put(r.view) })
  const pull = useMutation({ mutationFn: api.repoPull, onSuccess: put })
  const push = useMutation({ mutationFn: api.repoPush, onSuccess: (r) => put(r.view) })
  const apply = useMutation({
    mutationFn: api.repoApply,
    onSuccess: (r) => { put(r.view); qc.invalidateQueries({ queryKey: ['routines'] }) },
  })
  const v = q.data
  const error = save.error ?? exp.error ?? pull.error ?? push.error ?? apply.error
  const busy = save.isPending || exp.isPending || pull.isPending || push.isPending

  return (
    <div className="space-y-4">
      <Card className="p-5">
        <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><FolderGit2 size={17} /> {t('repo.title')}</div>
        <p className="mb-4 text-[13px] text-ink-3">{t('repo.text')}</p>
        <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <input className={field} value={path} onChange={(e) => setPath(e.target.value)} placeholder={t('repo.pathPlaceholder')} aria-label={t('repo.path')} />
          <Button type="submit" disabled={busy || path === (v?.path ?? '')}>{t('common.save')}</Button>
        </form>
        {v?.path && (
          <>
            <p className="mt-2 text-[12.5px] text-ink-3">
              {v.git ? (v.remote ? t('repo.gitRemote', { head: v.head ?? '' }) : t('repo.gitLocal', { head: v.head ?? '' })) : t('repo.plain')}
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Button onClick={() => exp.mutate()} disabled={busy}><Upload size={15} /> {t('repo.export')}</Button>
              <Button onClick={() => pull.mutate()} disabled={busy}><Download size={15} /> {v.remote ? t('repo.pull') : t('repo.check')}</Button>
              {v.remote && <Button variant="ghost" onClick={() => push.mutate()} disabled={busy}>{t('repo.push')}</Button>}
            </div>
            {exp.data && <p className="mt-2 text-[12.5px] text-read">{t('repo.exported', { count: exp.data.written })}{exp.data.commit ? ` · ${exp.data.commit}` : ''}</p>}
            {push.data?.pushed && <p className="mt-2 text-[12.5px] text-read">{t('repo.pushed')}</p>}
          </>
        )}
        {(error || v?.error) && <p className="mt-2 text-[13px] text-danger">{error?.message ?? v?.error}</p>}
      </Card>

      {v?.path && (
        <div className="space-y-2">
          <h2 className="text-[13px] font-semibold uppercase tracking-wide text-ink-3">{t('repo.changes')}</h2>
          {v.changes.length === 0 && Object.keys(v.broken).length === 0 && <p className="text-[13px] text-ink-3">{t('repo.upToDate')}</p>}
          {v.changes.map((c) => (
            <Card key={c.id} className="p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-[14px] font-medium">{c.name} <code className="text-[12px] text-ink-3">{c.id}</code></div>
                  <div className="text-[12.5px] text-ink-3">{c.new ? t('repo.new') : t('repo.changed')} · {t('repo.tests', { count: c.tests })}</div>
                </div>
                {c.problems.length === 0
                  ? <Button variant="primary" size="sm" onClick={() => apply.mutate(c.id)} disabled={apply.isPending}><Check size={14} /> {c.new ? t('repo.install') : t('repo.update')}</Button>
                  : <span className="flex items-center gap-1 text-[12.5px] text-danger"><CircleAlert size={14} /> {t('repo.blocked')}</span>}
              </div>
              {c.added.length > 0 && (
                <p className="mt-2 rounded-lg bg-change-soft px-3 py-1.5 text-[12.5px] text-change">{t('repo.added', { list: c.added.map(capabilityLabel).join(', ') })}</p>
              )}
              {c.removed.length > 0 && <p className="mt-2 text-[12.5px] text-ink-3">{t('repo.removed', { list: c.removed.map(capabilityLabel).join(', ') })}</p>}
              {c.problems.length > 0 && (
                <ul className="mt-2 list-disc space-y-0.5 pl-5 text-[12.5px] text-danger">{c.problems.map((p, i) => <li key={i}>{p}</li>)}</ul>
              )}
            </Card>
          ))}
          {Object.entries(v.broken).map(([id, why]) => (
            <Card key={id} className="p-4 text-[13px]"><code>{id}</code> <span className="text-danger">— {why}</span></Card>
          ))}
        </div>
      )}
    </div>
  )
}
