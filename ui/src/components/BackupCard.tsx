import { useMutation } from '@tanstack/react-query'
import { ArchiveRestore, Download, Upload } from 'lucide-react'
import { useState } from 'react'
import { useT } from '../lib/i18n'
import { Button, Card } from './ui'

async function post(path: string, body: BodyInit, json = false) {
  const res = await fetch(path, { method: 'POST', body, credentials: 'same-origin', headers: json ? { 'Content-Type': 'application/json' } : {} })
  if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? res.statusText)
  return res
}

// Everything in one file: routines, history, receipts, memory, people,
// connectors and secrets, the secrets sealed with a passphrase.
export function BackupCard() {
  const t = useT()
  const [pass, setPass] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [importPass, setImportPass] = useState('')
  const exp = useMutation({
    mutationFn: async () => {
      const res = await post('/api/backup/export', JSON.stringify({ passphrase: pass }), true)
      const url = URL.createObjectURL(await res.blob())
      const a = document.createElement('a')
      a.href = url
      a.download = res.headers.get('Content-Disposition')?.match(/filename="(.+)"/)?.[1] ?? 'zodim.zodim'
      a.click()
      URL.revokeObjectURL(url)
    },
  })
  const imp = useMutation({
    mutationFn: async () => {
      const form = new FormData()
      form.append('file', file!)
      form.append('passphrase', importPass)
      return (await post('/api/backup/import', form)).json() as Promise<{ secrets: number }>
    },
  })
  const input = 'h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'
  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><ArchiveRestore size={17} /> {t('backup.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('backup.text')}</p>
      <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); exp.mutate() }}>
        <input type="password" value={pass} onChange={(e) => setPass(e.target.value)} placeholder={t('backup.passPlaceholder')} aria-label={t('backup.passExport')} className={input + ' min-w-[220px] flex-1'} />
        <Button type="submit" disabled={pass.length < 8 || exp.isPending}><Download size={15} /> {t('backup.export')}</Button>
      </form>
      {exp.error && <p className="mt-2 text-[13px] text-danger">{exp.error.message}</p>}
      <form className="mt-4 flex flex-wrap gap-2 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); imp.mutate() }}>
        <input type="file" accept=".zodim" aria-label={t('backup.file')} onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="min-w-[200px] flex-1 text-[13px] file:mr-3 file:rounded-lg file:border file:border-line file:bg-surface file:px-3 file:py-1.5" />
        <input type="password" value={importPass} onChange={(e) => setImportPass(e.target.value)} placeholder={t('backup.passImportPlaceholder')} aria-label={t('backup.passImport')} className={input} />
        <Button type="submit" disabled={!file || !importPass || imp.isPending}><Upload size={15} /> {t('backup.import')}</Button>
      </form>
      {imp.error && <p className="mt-2 text-[13px] text-danger">{imp.error.message}</p>}
      {imp.data && <p className="mt-2 rounded-lg bg-read-soft px-3 py-2 text-[13px] text-read">{t('backup.checked', { count: imp.data.secrets })}</p>}
    </Card>
  )
}
