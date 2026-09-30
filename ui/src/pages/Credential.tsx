import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LockKeyhole } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Card, EmptyState, PageSkeleton } from '../components/ui'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'

// The private form a credential request links to. What is typed here goes
// straight to the vault; it is never shown again, sent to a chat or a
// model, or kept in the page after saving.
export function Credential() {
  const t = useT()
  const nav = useNavigate()
  const qc = useQueryClient()
  const { id = '' } = useParams()
  const [value, setValue] = useState('')
  const req = useQuery({ queryKey: ['credential', id], queryFn: () => api.credential(id), retry: false })
  const save = useMutation({
    mutationFn: () => api.saveCredential(id, value),
    onSuccess: () => { setValue(''); qc.invalidateQueries({ queryKey: ['credentials'] }) },
  })
  const dismiss = useMutation({ mutationFn: () => api.dismissCredential(id), onSuccess: () => { qc.invalidateQueries({ queryKey: ['credentials'] }); nav('/inbox') } })
  const again = useMutation({
    mutationFn: async () => {
      const done = save.data
      if (done?.routine) { await api.routineAction(done.routine, 'run'); return `/routines/${done.routine}` }
      if (done?.exploration) { const r = await api.retryExploration(done.exploration); return `/explorations/${r.id}` }
      return '/inbox'
    },
    onSuccess: (path) => nav(path),
  })
  const submit = (e: FormEvent) => { e.preventDefault(); if (value.trim()) save.mutate() }

  if (save.data) {
    return (
      <div className="mx-auto max-w-lg">
        <EmptyState icon={<LockKeyhole size={22} />} title={t('cred.saved')}
          action={save.data.routine || save.data.exploration ? <Button variant="primary" disabled={again.isPending} onClick={() => again.mutate()}>{save.data.routine ? t('cred.runAgain') : t('cred.tryAgain')}</Button> : undefined}>
          {t('cred.savedText')}
        </EmptyState>
        {again.error && <p className="mt-2 text-center text-[13px] text-danger">{again.error.message}</p>}
      </div>
    )
  }
  if (req.isLoading) return <PageSkeleton />
  if (req.error || !req.data) {
    return (
      <div className="mx-auto max-w-lg">
        <EmptyState icon={<KeyRound size={22} />} title={t('cred.goneTitle')}>{req.error?.message ?? t('cred.goneTitle')}</EmptyState>
      </div>
    )
  }
  const c = req.data
  return (
    <div className="mx-auto max-w-lg">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('cred.title', { title: c.title })}</h1>
      <p className="mb-6 text-sm text-ink-2">{c.description}</p>
      <Card className="p-5">
        <form onSubmit={submit} className="space-y-3" autoComplete="off">
          <label htmlFor="cred-value" className="block text-[13px] font-medium">{c.label}</label>
          <input id="cred-value" value={value} onChange={(e) => setValue(e.target.value)} type="password" autoComplete="new-password" spellCheck={false} autoFocus
            className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <p className="flex items-start gap-2 text-[12.5px] text-ink-3"><LockKeyhole size={14} className="mt-0.5 shrink-0" aria-hidden /> {t('cred.private')}</p>
          {save.error && <p role="alert" className="text-[13px] text-danger">{save.error.message}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" disabled={dismiss.isPending} onClick={() => dismiss.mutate()}>{t('cred.dismiss')}</Button>
            <Button type="submit" variant="primary" disabled={!value.trim() || save.isPending}>{t('cred.save')}</Button>
          </div>
        </form>
      </Card>
    </div>
  )
}
