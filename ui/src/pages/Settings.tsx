import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, ChevronRight } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { BackupCard } from '../components/BackupCard'
import { CloudBackup } from '../components/CloudBackup'
import { ProtectionCard } from '../components/ProtectionCard'
import { PhonePairing } from '../components/PhonePairing'
import { Button, Card } from '../components/ui'
import { api, type Settings as S } from '../lib/api'
import { cn } from '../lib/cn'
import { fill, useT } from '../lib/i18n'

const judges = [
  { value: 'local', title: 'settings.judge.local', text: 'settings.judge.localText' },
  { value: 'jev', title: 'settings.judge.jev', text: 'settings.judge.jevText' },
  { value: 'llm', title: 'settings.judge.llm', text: 'settings.judge.llmText' },
] as const

export function Settings() {
  const t = useT()
  const qc = useQueryClient()
  const nav = useNavigate()
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  const [s, setS] = useState<S>()
  const [budget, setBudget] = useState('')
  useEffect(() => { if (settings.data) setS(settings.data) }, [settings.data])
  useEffect(() => { if (state.data) setBudget(String(state.data.budget.limit)) }, [state.data])
  const save = useMutation({
    mutationFn: async () => {
      if (s) await api.saveSettings(s)
      await api.setBudget(Number(budget))
    },
    onSuccess: () => qc.invalidateQueries(),
  })
  if (!s) return null
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('settings.title')}</h1>
        <p className="text-sm text-ink-2">{t('settings.subtitle')}</p>
      </div>
      <Card role="link" tabIndex={0} onClick={() => nav('/import')} onKeyDown={(k) => k.key === 'Enter' && nav('/import')} className="flex cursor-pointer items-center gap-4 p-5 hover:border-line-strong">
        <div className="grid size-10 place-items-center rounded-xl bg-explore-soft text-explore"><ArrowDownToLine size={18} /></div>
        <div className="flex-1">
          <div className="text-[15px] font-medium">{t('settings.import')}</div>
          <div className="text-[13px] text-ink-3">{t('settings.importText')}</div>
        </div>
        <ChevronRight size={17} className="text-ink-3" />
      </Card>
      <PhonePairing />
      <BackupCard />
      <CloudBackup />
      <ProtectionCard on={!!s.protection_network} toggle={() => setS({ ...s, protection_network: !s.protection_network })} />
      <Card className="flex items-start gap-4 p-5">
        <div className="flex-1">
          <div className="text-[15px] font-medium">{t('settings.email')}</div>
          <p className="text-[13px] text-ink-3">{t('settings.emailText')}</p>
        </div>
        <button type="button" role="switch" aria-checked={!!s.email_channel} aria-label={t('settings.email')} onClick={() => setS({ ...s, email_channel: !s.email_channel })}
          className={cn('mt-1 flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition', s.email_channel ? 'bg-accent' : 'bg-line-strong')}>
          <span className={cn('size-5 rounded-full bg-white shadow transition', s.email_channel && 'translate-x-5')} />
        </button>
      </Card>
      <Card className="p-5">
        <div className="text-[15px] font-medium">{t('settings.budget')}</div>
        <p className="mb-3 text-[13px] text-ink-3">{t('settings.budgetText')}</p>
        <div className="flex items-center gap-2">
          <span className="text-ink-3">$</span>
          <input value={budget} onChange={(e) => setBudget(e.target.value)} type="number" min="0" step="0.5" aria-label={t('settings.budgetLabel')} className="h-10 w-32 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <span className="text-[12.5px] text-ink-3">{t('settings.noLimit')}</span>
        </div>
      </Card>
      <Card className="p-5">
        <div className="mb-3 text-[15px] font-medium">{t('settings.judges')}</div>
        <div className="grid gap-2 sm:grid-cols-3">
          {judges.map((j) => (
            <button key={j.value} onClick={() => setS({ ...s, judge_backend: j.value })} className={cn('rounded-xl border p-3 text-left transition-colors', s.judge_backend === j.value ? 'border-accent bg-accent/8' : 'border-line hover:border-line-strong')}>
              <div className="text-[14px] font-medium">{t(j.title)}</div>
              <div className="mt-0.5 text-[12.5px] text-ink-3">{t(j.text)}</div>
            </button>
          ))}
        </div>
        {s.judge_backend === 'local' && (
          <label className="mt-4 block">
            <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.localUrl')}</span>
            <input value={s.local_judge_url} onChange={(e) => setS({ ...s, local_judge_url: e.target.value })} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 font-mono text-[13px] outline-none focus:border-accent" />
            <span className="mt-1 block text-[12px] text-ink-3">{fill(t('settings.localHint'), { cmd: <code className="rounded bg-sunken px-1">tools/judge/serve.py</code> })}</span>
          </label>
        )}
      </Card>
      <Card className="grid gap-4 p-5 sm:grid-cols-2">
        <label>
          <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.zone')}</span>
          <input value={s.zone} onChange={(e) => setS({ ...s, zone: e.target.value })} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
        </label>
        <label>
          <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{t('settings.language')}</span>
          <select value={s.locale} onChange={(e) => setS({ ...s, locale: e.target.value })} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent">
            <option value="pt-BR">Português</option>
            <option value="en-US">English</option>
          </select>
        </label>
      </Card>
      {save.error && <p className="text-sm text-danger">{save.error.message}</p>}
      <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>{save.isSuccess ? t('common.saved') : t('common.save')}</Button>
    </div>
  )
}
