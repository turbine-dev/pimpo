import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Button, Card } from '../components/ui'
import { api, type Org } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { area, field } from './Companies'

const nameOf = (org: Org, id: string) => org.members.find((m) => m.id === id)?.name ?? id

// author is who wrote a note: a person, or a member of the company.
function author(org: Org, by: string, you: string) {
  if (by.startsWith('human:')) return you
  return nameOf(org, by.split('/').pop() ?? by)
}

export function Digest({ org }: { org: Org }) {
  const t = useT()
  const digest = useQuery({ queryKey: ['company-digest', org.id], queryFn: () => api.companyDigest(org.id) })
  if (!digest.data) return null
  return (
    <Card className="p-4">
      <h2 className="mb-1 text-[13.5px] font-semibold">{t('co.thisWeek')}</h2>
      <p className="whitespace-pre-line text-[12.5px] text-ink-2">{digest.data.text}</p>
    </Card>
  )
}

export function MemoryTab({ org, can }: { org: Org; can: boolean }) {
  const t = useT()
  const qc = useQueryClient()
  const notes = useQuery({ queryKey: ['company-notes', org.id], queryFn: () => api.companyNotes(org.id) })
  const [note, setNote] = useState({ kind: 'decision', title: '', body: '' })
  const refresh = () => qc.invalidateQueries({ queryKey: ['company-notes', org.id] })
  const add = useMutation({ mutationFn: () => api.saveCompanyNote(org.id, note), onSuccess: () => { setNote({ ...note, title: '', body: '' }); refresh() } })
  const remove = useMutation({ mutationFn: (id: string) => api.deleteCompanyNote(org.id, id), onSuccess: refresh })
  return (
    <div className="max-w-3xl">
      <section className="space-y-3">
        <h2 className="text-[15px] font-semibold">{t('co.memory')}</h2>
        <p className="text-[12.5px] text-ink-3">{t('co.memoryHint')}</p>
        {can && (
          <form className="space-y-2 rounded-[var(--radius-card)] border border-line p-3" onSubmit={(e) => { e.preventDefault(); add.mutate() }}>
            <div className="flex gap-2">
              <select className={field + ' w-36'} aria-label={t('co.noteKind')} value={note.kind} onChange={(e) => setNote({ ...note, kind: e.target.value })}>
                {['decision', 'fact', 'lesson'].map((k) => <option key={k} value={k}>{t(`co.note.${k}` as 'co.note.fact')}</option>)}
              </select>
              <input className={field} aria-label={t('co.noteTitle')} placeholder={t('co.noteTitle')} value={note.title} maxLength={120} onChange={(e) => setNote({ ...note, title: e.target.value })} />
            </div>
            <textarea className={area} aria-label={t('co.noteText')} placeholder={t('co.noteText')} value={note.body} onChange={(e) => setNote({ ...note, body: e.target.value })} />
            {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
            <Button type="submit" size="sm" disabled={!note.title.trim() || !note.body.trim() || add.isPending}><Plus size={13} /> {t('co.keep')}</Button>
          </form>
        )}
        {(notes.data ?? []).map((n) => (
          <Card key={n.id} className="flex items-start gap-3 p-4">
            <div className="min-w-0 flex-1">
              <p className="text-[12px] text-ink-3">{t(`co.note.${n.kind}` as 'co.note.fact')} · {author(org, n.by, t('co.you'))} · {relative(n.created)}</p>
              <p className="text-[14px] font-medium">{n.title}</p>
              <p className="line-clamp-4 whitespace-pre-line text-[12.5px] text-ink-2">{n.body}</p>
            </div>
            {can && <Button size="sm" variant="ghost" aria-label={t('co.forget', { name: n.title })} onClick={() => remove.mutate(n.id)}><Trash2 size={14} /></Button>}
          </Card>
        ))}
      </section>
    </div>
  )
}
