import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Link2, Loader2, Puzzle, Trash2, Upload } from 'lucide-react'
import { useState } from 'react'
import { api, type SkillPreview } from '../lib/api'
import { useT } from '../lib/i18n'
import { capabilityLabel } from './RoutineCard'
import { Button, Card, RiskBadge, type Risk } from './ui'

const field = 'h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// Skills installs SKILL.md skills (OpenClaw, Hermes, agentskills.io) as
// assistants limited to what the owner grants.
export function Skills() {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['skills'], queryFn: api.skills })
  const caps = useQuery({ queryKey: ['capabilities'], queryFn: api.capabilities })
  const [url, setUrl] = useState('')
  const [preview, setPreview] = useState<SkillPreview | null>(null)
  const [chosen, setChosen] = useState<string[]>([])
  const done = () => {
    qc.invalidateQueries({ queryKey: ['skills'] })
    qc.invalidateQueries({ queryKey: ['assistants'] })
  }
  const read = useMutation({
    mutationFn: (src: { file?: File; url?: string }) => api.previewSkill(src),
    onSuccess: (p) => { setPreview(p); setChosen(p.skill.suggested) },
  })
  const install = useMutation({
    mutationFn: () => api.installSkill(preview!.token, chosen),
    onSuccess: () => { setPreview(null); setUrl(''); done() },
  })
  const remove = useMutation({ mutationFn: api.deleteSkill, onSuccess: done })
  // guard.* protects other agents; it is nothing a skill could use.
  const all = (caps.data ?? []).filter((c) => !c.name.startsWith('guard.'))
  const sk = preview?.skill

  return (
    <section aria-labelledby="skills-title">
      <h1 id="skills-title" className="mb-1 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><Puzzle size={20} /> {t('sk.title')}</h1>
      <p className="mb-6 text-sm text-ink-2">{t('sk.text')}</p>

      {(list.data ?? []).length > 0 && (
        <div className="mb-4 grid gap-3 sm:grid-cols-2">
          {list.data!.map((s) => (
            <Card key={s.id} className="flex items-start gap-3 p-4">
              <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-sunken text-[20px]" aria-hidden>🧩</div>
              <div className="min-w-0 flex-1">
                <div className="text-[14.5px] font-medium">{s.name}</div>
                <p className="line-clamp-2 text-[12.5px] text-ink-3">{s.description}</p>
                <p className="mt-1 text-[12px] text-ink-2">{s.capabilities.length ? t('as.count', { count: s.capabilities.length }) : t('sk.none')}</p>
              </div>
              <Button size="sm" variant="ghost" aria-label={t('sk.remove', { name: s.name })} onClick={() => remove.mutate(s.id)}><Trash2 size={14} /></Button>
            </Card>
          ))}
        </div>
      )}

      {!sk && (
        <Card className="space-y-3 p-4">
          <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); read.mutate({ url }) }}>
            <label className="min-w-[240px] flex-1">
              <span className="sr-only">{t('sk.url')}</span>
              <input className={field} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://github.com/owner/repo/tree/main/skills/name" aria-label={t('sk.url')} spellCheck={false} />
            </label>
            <Button type="submit" disabled={!url.startsWith('https://github.com/') || read.isPending}>
              {read.isPending ? <Loader2 size={14} className="animate-spin" /> : <Link2 size={14} />} {t('sk.read')}
            </Button>
            <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-[10px] border border-line bg-surface px-3 text-[13px] hover:border-line-strong">
              <Upload size={14} /> {t('sk.zip')}
              <input type="file" accept=".zip" className="sr-only" onChange={(e) => e.target.files?.[0] && read.mutate({ file: e.target.files[0] })} />
            </label>
          </form>
          {read.error && <p className="text-[13px] text-danger">{read.error.message}</p>}
        </Card>
      )}

      {sk && (
        <Card className="space-y-4 p-5">
          <div>
            <div className="text-[15px] font-semibold">{sk.name}</div>
            <p className="text-[13px] text-ink-2">{sk.description}</p>
            {preview!.exists && <p className="mt-1 text-[12.5px] text-ink-3">{t('sk.replaces')}</p>}
          </div>
          <p className="flex gap-2 rounded-xl bg-sunken px-3 py-2 text-[12.5px] text-ink-2">
            <AlertTriangle size={15} className="mt-0.5 shrink-0 text-ink-3" /> {t('sk.warn')}
          </p>
          {sk.scripts.length > 0 && <p className="text-[12.5px] text-ink-3">{t('sk.scripts', { files: sk.scripts.join(', ') })}</p>}
          {sk.unsupported.length > 0 && <p className="text-[12.5px] text-ink-3">{t('sk.unsupported', { what: sk.unsupported.join('; ') })}</p>}
          <details className="text-[12.5px]">
            <summary className="cursor-pointer text-ink-2">{t('sk.readText')}</summary>
            <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap rounded-lg border border-line bg-bg p-3 font-mono text-[12px]">{sk.body}</pre>
          </details>
          <div>
            <div className="mb-1 text-[14px] font-medium">{t('sk.grant')}</div>
            <p className="mb-2 text-[12px] text-ink-3">{t('sk.grantHint')}</p>
            <ul className="max-h-72 divide-y divide-line overflow-y-auto rounded-xl border border-line">
              {[...all].sort((a, b) => Number(sk.suggested.includes(b.name)) - Number(sk.suggested.includes(a.name))).map((c) => (
                <li key={c.name} className="flex items-center gap-3 px-3 py-2">
                  <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={chosen.includes(c.name)} aria-label={c.name}
                    onChange={(e) => setChosen(e.target.checked ? [...chosen, c.name] : chosen.filter((x) => x !== c.name))} />
                  <span className="min-w-0 flex-1 text-[13px]">{capabilityLabel(c.name)} <code className="ml-1 font-mono text-[11.5px] text-ink-3">{c.name}</code></span>
                  <RiskBadge risk={c.risk as Risk} />
                </li>
              ))}
            </ul>
          </div>
          {install.error && <p className="text-[13px] text-danger">{install.error.message}</p>}
          <div className="flex gap-2">
            <Button variant="primary" onClick={() => install.mutate()} disabled={install.isPending}>
              {install.isPending && <Loader2 size={14} className="animate-spin" />} {t('sk.install', { count: chosen.length })}
            </Button>
            <Button variant="ghost" onClick={() => setPreview(null)}>{t('common.cancel')}</Button>
          </div>
        </Card>
      )}
    </section>
  )
}
