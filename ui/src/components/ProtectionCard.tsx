import { useQuery } from '@tanstack/react-query'
import { ShieldCheck } from 'lucide-react'
import { api } from '../lib/api'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { Card } from './ui'

export function ProtectionCard({ on, toggle }: { on: boolean; toggle: () => void }) {
  const t = useT()
  const q = useQuery({ queryKey: ['protection'], queryFn: api.protection })
  const p = q.data
  return (
    <Card className="flex items-start gap-4 p-5">
      <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-read-soft text-read"><ShieldCheck size={18} /></div>
      <div className="flex-1">
        <div className="text-[15px] font-medium">{t('protection.title')}</div>
        <p className="text-[13px] text-ink-3">{t('protection.text')}</p>
        {p && <p className="mt-1.5 text-[12.5px] text-ink-2">{t('protection.stats', { version: p.version, entries: p.entries, count: p.blocked })}</p>}
      </div>
      <button type="button" role="switch" aria-checked={on} aria-label={t('protection.toggle')} onClick={toggle}
        className={cn('mt-1 flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition', on ? 'bg-accent' : 'bg-line-strong')}>
        <span className={cn('size-5 rounded-full bg-white shadow transition', on && 'translate-x-5')} />
      </button>
    </Card>
  )
}
