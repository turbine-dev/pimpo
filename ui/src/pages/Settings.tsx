import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, Bell, ChevronRight, Cpu, FlaskConical, HardDriveDownload, ShieldCheck, SlidersHorizontal, Smartphone } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { BackupCard } from '../components/BackupCard'
import { CloudBackup } from '../components/CloudBackup'
import { SnapshotsCard } from '../components/SnapshotsCard'
import { inDesktopApp, mascotOn, setMascotOn } from '../components/Mascot'
import { UpdateCard } from '../components/Updates'
import { Head } from '../components/PimpoArt'
import { ModelSetup } from '../components/ModelSetup'
import { ProtectionCard } from '../components/ProtectionCard'
import { PhonePairing } from '../components/PhonePairing'
import { Button, Card, Switch } from '../components/ui'
import { api, type Settings as S } from '../lib/api'
import { cn } from '../lib/cn'
import { fill, useT, type TKey, LANGUAGES } from '../lib/i18n'

const judges = [
  { value: 'local', title: 'settings.judge.local', text: 'settings.judge.localText' },
  { value: 'jev', title: 'settings.judge.jev', text: 'settings.judge.jevText' },
  { value: 'llm', title: 'settings.judge.llm', text: 'settings.judge.llmText' },
] as const

const sections: { id: string; label: TKey; icon: ReactNode }[] = [
  { id: 'geral', label: 'set.general', icon: <SlidersHorizontal size={16} /> },
  { id: 'celular', label: 'set.phone', icon: <Smartphone size={16} /> },
  { id: 'modelos', label: 'set.models', icon: <Cpu size={16} /> },
  { id: 'notificacoes', label: 'set.notifications', icon: <Bell size={16} /> },
  { id: 'backup', label: 'set.backup', icon: <HardDriveDownload size={16} /> },
  { id: 'privacidade', label: 'set.privacy', icon: <ShieldCheck size={16} /> },
  { id: 'laboratorio', label: 'set.labs', icon: <FlaskConical size={16} /> },
  { id: 'trazer', label: 'set.import', icon: <ArrowDownToLine size={16} /> },
]

const field = 'h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// Settings, split into sections; changes to preferences wait in a bar at
// the bottom until saved, while cards with their own button save at once.
const modelFields = ['models', 'explore_model', 'compile_model', 'judge_model', 'fallbacks', 'ollama_url', 'lmstudio_url', 'custom_url'] as const
const pick = (d: S, keys: readonly (keyof S)[]) => Object.fromEntries(keys.map((k) => [k, d[k]])) as Partial<S>

export function Settings() {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const { hash } = useLocation()
  const section = sections.find((x) => x.id === hash.slice(1))?.id ?? 'geral'
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const [s, setS] = useState<S>()
  const [budget, setBudget] = useState('')
  // The model screen saves on its own; its fields follow what it saved
  // without losing unsaved edits elsewhere on this page.
  useEffect(() => {
    const d = settings.data
    if (!d) return
    setS((prev) => (!prev ? d : { ...prev, ...pick(d, modelFields) }))
  }, [settings.data])
  useEffect(() => { if (state.data) setBudget(String(state.data.budget.limit)) }, [state.data])
  const dirty = !!s && !!settings.data && (JSON.stringify(s) !== JSON.stringify(settings.data) || (state.data && budget !== String(state.data.budget.limit)))
  const save = useMutation({
    mutationFn: async () => {
      if (s) await api.saveSettings(s)
      await api.setBudget(Number(budget))
    },
    onSuccess: () => qc.invalidateQueries(),
  })
  if (!s) return null
  const discard = () => { setS(settings.data); if (state.data) setBudget(String(state.data.budget.limit)); save.reset() }

  return (
    <div className="mx-auto max-w-5xl pb-20">
      <div className="mb-6">
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('settings.title')}</h1>
        <p className="text-sm text-ink-2">{t('settings.subtitle')}</p>
      </div>
      <div className="flex flex-col gap-6 md:flex-row">
        <nav aria-label={t('settings.title')} className="-mx-1 flex gap-1 overflow-x-auto pb-1 md:mx-0 md:w-52 md:shrink-0 md:flex-col md:overflow-visible">
          {sections.map((x) => (
            <button key={x.id} type="button" onClick={() => nav(`/settings#${x.id}`, { replace: true })} aria-current={section === x.id ? 'page' : undefined}
              className={cn('flex shrink-0 items-center gap-2.5 whitespace-nowrap rounded-[10px] px-3 py-2 text-left text-[13.5px] transition-colors',
                section === x.id ? 'bg-sunken font-medium text-ink' : 'text-ink-2 hover:bg-sunken/70 hover:text-ink')}>
              <span className="text-ink-3">{x.icon}</span>{t(x.label)}
            </button>
          ))}
        </nav>

        <div className="min-w-0 flex-1 space-y-4">
          {section === 'geral' && <>
            <MascotCard />
            <UpdateCard />
            <Card className="grid gap-4 p-5 sm:grid-cols-2">
              <label>
                <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.language')}</span>
                <select value={s.locale} onChange={(e) => setS({ ...s, locale: e.target.value })} className={cn(field, 'w-full')}>
                  {LANGUAGES.map((l) => <option key={l.tag} value={l.tag}>{l.name}</option>)}
                </select>
              </label>
              <label>
                <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.zone')}</span>
                <input value={s.zone} onChange={(e) => setS({ ...s, zone: e.target.value })} className={cn(field, 'w-full')} />
              </label>
            </Card>
            <Card className="p-5">
              <div className="text-[15px] font-medium">{t('settings.budget')}</div>
              <p className="mb-3 text-[13px] text-ink-3">{t('settings.budgetText')}</p>
              <div className="flex items-center gap-2">
                <span className="text-ink-3">$</span>
                <input value={budget} onChange={(e) => setBudget(e.target.value)} type="number" min="0" step="0.5" aria-label={t('settings.budgetLabel')} className={cn(field, 'w-32')} />
                <span className="text-[12.5px] text-ink-3">{t('settings.noLimit')}</span>
              </div>
            </Card>
          </>}

          {section === 'celular' && <PhonePairing />}

          {section === 'modelos' && <>
            <ModelSetup />
            <Card className="p-5">
              <div className="text-[15px] font-medium">{t('settings.judges')}</div>
              <p className="mb-3 text-[13px] text-ink-3">{t('set.judgesText')}</p>
              <div className="grid gap-2 sm:grid-cols-3">
                {judges.map((j) => (
                  <button key={j.value} type="button" onClick={() => setS({ ...s, judge_backend: j.value })} aria-pressed={s.judge_backend === j.value}
                    className={cn('rounded-xl border p-3 text-left transition-colors', s.judge_backend === j.value ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
                    <div className="text-[14px] font-medium">{t(j.title)}</div>
                    <div className="mt-0.5 text-[12.5px] text-ink-3">{t(j.text)}</div>
                  </button>
                ))}
              </div>
              {s.judge_backend === 'local' && (
                <label className="mt-4 block">
                  <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.localUrl')}</span>
                  <input value={s.local_judge_url} onChange={(e) => setS({ ...s, local_judge_url: e.target.value })} className={cn(field, 'w-full font-mono text-[13px]')} />
                  <span className="mt-1 block text-[12px] text-ink-3">{fill(t('settings.localHint'), { cmd: <code className="rounded bg-sunken px-1">tools/judge/serve.py</code> })}</span>
                </label>
              )}
            </Card>
          </>}

          {section === 'notificacoes' && <>
            <Toggles title={t('notif.title')} text={t('notif.text')} items={[
              { key: 'approval', label: t('notif.approval'), hint: t('notif.approvalHint'), on: true, locked: true },
              ...(['task', 'failure', 'backup'] as const).map((k) => ({ key: k, label: t(`notif.${k}`), on: !(s.mute ?? []).includes(k) })),
            ]} onToggle={(k, on) => setS({ ...s, mute: on ? (s.mute ?? []).filter((x) => x !== k) : [...(s.mute ?? []), k] })} />
            <Toggles title={t('settings.email')} text={t('settings.emailText')} items={[{ key: 'email', label: t('set.emailOn'), on: !!s.email_channel }]}
              onToggle={(_, on) => setS({ ...s, email_channel: on })} />
            <Toggles title={t('settings.suggestTitle')} text={t('settings.suggestText')} items={[{ key: 'suggest', label: t('settings.suggestTitle'), on: !s.suggest_off }]}
              onToggle={(_, on) => setS({ ...s, suggest_off: !on })} />
            <Toggles title={t('settings.learnTitle')} text={t('settings.learnText')} items={[{ key: 'learn', label: t('settings.learnTitle'), on: !s.learn_off }]}
              onToggle={(_, on) => setS({ ...s, learn_off: !on })} />
            <Toggles title={t('settings.lessonDigestTitle')} text={t('settings.lessonDigestText')} items={[{ key: 'lessonDigest', label: t('settings.lessonDigestTitle'), on: !s.lesson_digest_off }]}
              onToggle={(_, on) => setS({ ...s, lesson_digest_off: !on })} />
          </>}

          {section === 'backup' && <>
            <SnapshotsCard />
            <BackupCard />
            <CloudBackup />
          </>}

          {section === 'privacidade' && <ProtectionCard on={!!s.protection_network} toggle={() => setS({ ...s, protection_network: !s.protection_network })} />}

          {section === 'laboratorio' && (<>
            <Toggles title={t('labs.title')} text={t('labs.text')} items={(['memory_organize', 'meaning_search', 'mcp_registry'] as const).map((k) => ({
              key: k, label: t(`labs.${k}`), hint: t(`labs.${k}Hint`), on: !(s.labs_off ?? []).includes(k),
            }))} onToggle={(k, on) => setS({ ...s, labs_off: on ? (s.labs_off ?? []).filter((x) => x !== k) : [...(s.labs_off ?? []), k] })} />
            <Toggles title={t('labs.powerTitle')} text={t('labs.powerText')} items={(['code_sandbox', 'browser', 'whatsapp_personal'] as const).map((k) => ({
              key: k, label: t(`labs.${k}`), hint: t(`labs.${k}Hint`), on: (s.labs_on ?? []).includes(k),
            }))} onToggle={(k, on) => setS({ ...s, labs_on: on ? [...(s.labs_on ?? []), k] : (s.labs_on ?? []).filter((x) => x !== k) })} />
            {(s.labs_on ?? []).includes('browser') && <BrowserLogin />}
          </>)}

          {section === 'trazer' && (
            <Card role="link" tabIndex={0} onClick={() => nav('/import')} onKeyDown={(k) => k.key === 'Enter' && nav('/import')} className="flex cursor-pointer items-center gap-4 p-5 hover:border-line-strong">
              <div className="grid size-10 place-items-center rounded-xl bg-explore-soft text-explore"><ArrowDownToLine size={18} /></div>
              <div className="flex-1">
                <div className="text-[15px] font-medium">{t('settings.import')}</div>
                <div className="text-[13px] text-ink-3">{t('settings.importText')}</div>
              </div>
              <ChevronRight size={17} className="text-ink-3" />
            </Card>
          )}
        </div>
      </div>

      {(dirty || save.isSuccess || save.error) && (
        <div className="fixed inset-x-0 bottom-[calc(64px+env(safe-area-inset-bottom))] z-30 flex justify-center px-4 md:bottom-6 md:left-60">
          <div className="flex items-center gap-3 rounded-2xl border border-line bg-surface px-4 py-2.5 shadow-[var(--shadow-pop)]">
            <span className={cn('text-[13px]', save.error ? 'text-danger' : 'text-ink-2')}>
              {save.error ? save.error.message : dirty ? t('set.unsaved') : t('common.saved')}
            </span>
            {dirty && <>
              <Button size="sm" variant="ghost" onClick={discard}>{t('set.discard')}</Button>
              <Button size="sm" variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{t('common.save')}</Button>
            </>}
          </div>
        </div>
      )}
    </div>
  )
}

type Toggle = { key: string; label: string; hint?: string; on: boolean; locked?: boolean }

function Toggles({ title, text, items, onToggle }: { title: string; text: string; items: Toggle[]; onToggle: (key: string, on: boolean) => void }) {
  return (
    <Card className="p-5">
      <div className="text-[15px] font-medium">{title}</div>
      <p className="mb-3 text-[13px] text-ink-3">{text}</p>
      <ul className="divide-y divide-line">
        {items.map((it) => (
          <li key={it.key} className="flex items-start gap-3 py-2.5">
            <div className="flex-1">
              <div className="text-[13.5px]">{it.label}</div>
              {it.hint && <div className="text-[12px] text-ink-3">{it.hint}</div>}
            </div>
            <Switch on={it.on} label={it.label} disabled={it.locked} onChange={(v) => onToggle(it.key, v)} className="mt-0.5" />
          </li>
        ))}
      </ul>
    </Card>
  )
}

// MascotCard turns the Pimpo mascot on or off in this app; the desktop
// app also offers it on the whole screen from the menu bar.
function MascotCard() {
  const t = useT()
  const [on, setOn] = useState(mascotOn)
  return (
    <Card className="flex items-center gap-4 p-5">
      <svg viewBox="0 0 512 512" className="size-12 shrink-0" aria-hidden><Head mood={on ? 'happy' : 'sleep'} /></svg>
      <div className="min-w-0 flex-1">
        <div className="text-[15px] font-medium">{t('mascot.setting')}</div>
        <p className="text-[13px] text-ink-3">{t('mascot.settingText')}</p>
        {inDesktopApp() && <p className="mt-1 text-[12.5px] text-ink-3">{t('mascot.desktopHint')}</p>}
      </div>
      <Switch on={on} label={t('mascot.setting')} onChange={(v) => { setMascotOn(v); setOn(v) }} />
    </Card>
  )
}

// BrowserLogin opens a window of Pimpo's own browser profile, for signing
// in to the sites routines will use.
function BrowserLogin() {
  const t = useT()
  const [url, setUrl] = useState('')
  const open = useMutation({ mutationFn: () => api.browserLogin(url) })
  return (
    <Card className="space-y-2 p-5">
      <div className="text-[15px] font-medium">{t('labs.browserLogin')}</div>
      <p className="text-[13px] text-ink-3">{t('labs.browserLoginHint')}</p>
      <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); open.mutate() }}>
        <input className="h-9 min-w-[220px] flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://" aria-label={t('labs.browserLoginUrl')} inputMode="url" />
        <Button type="submit" disabled={open.isPending}>{t('labs.browserLogin')}</Button>
      </form>
      {open.isSuccess && <p className="text-[12.5px] text-read">{t('labs.browserLoginOpened')}</p>}
      {open.error && <p className="text-[12.5px] text-danger">{open.error.message}</p>}
    </Card>
  )
}

