import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Gauge, KeyRound, Loader2, LogOut, MonitorSmartphone, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { relative, usd } from '../lib/format'
import { useT } from '../lib/i18n'
import { addPasskey, canUsePasskeys, passkeyMessage } from '../lib/passkey'
import { label } from '../components/ModelSetup'
import { Button, Card } from '../components/ui'

// Account is each person's own: the passkeys that sign them in, and the
// devices and sessions open in their name.
export function Account() {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['passkeys'], queryFn: api.passkeys })
  const [name, setName] = useState('')
  const done = () => qc.invalidateQueries({ queryKey: ['passkeys'] })
  const add = useMutation({ mutationFn: () => addPasskey(name.trim() || t('acct.defaultName')), onSuccess: () => { setName(''); done() } })
  const remove = useMutation({ mutationFn: api.deletePasskey, onSuccess: done })
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <h1 className="mb-1 flex items-center gap-2 text-[22px] font-semibold tracking-tight"><KeyRound size={20} /> {t('acct.title')}</h1>
        <p className="text-sm text-ink-2">{t('acct.text')}</p>
      </div>
      <Card className="space-y-3 p-4">
        {(list.data ?? []).map((k) => (
          <div key={k.id} className="flex items-center gap-3 text-[14px]">
            <KeyRound size={14} className="text-ink-3" />
            <span className="min-w-0 flex-1">
              <span className="block font-medium">{k.name}</span>
              <span className="block text-[12px] text-ink-3">{k.address} · {k.last_used ? t('acct.used', { when: relative(k.last_used) }) : t('acct.unused')}</span>
            </span>
            <Button size="sm" variant="ghost" aria-label={t('acct.remove', { name: k.name })} onClick={() => remove.mutate(k.id)}><Trash2 size={14} /></Button>
          </div>
        ))}
        {canUsePasskeys() ? (
          <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); add.mutate() }}>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('acct.name')} aria-label={t('acct.name')}
              className="h-10 min-w-[180px] flex-1 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
            <Button type="submit" disabled={add.isPending}>{add.isPending ? <Loader2 size={14} className="animate-spin" /> : <KeyRound size={14} />} {t('acct.add')}</Button>
          </form>
        ) : <p className="text-[13px] text-ink-2">{t('passkey.address')}</p>}
        {add.error && <p className="text-[13px] text-danger">{passkeyMessage(add.error, t)}</p>}
        {canUsePasskeys() && <p className="text-[12.5px] text-ink-3">{t('acct.where')}</p>}
      </Card>
      <MyDevices />
      <MyLimits />
    </div>
  )
}

// MyLimits shows the models the administrator lets this person use, their
// own daily limit and what they spent today: theirs, no one else's.
function MyLimits() {
  const t = useT()
  const q = useQuery({ queryKey: ['my-limits'], queryFn: api.myLimits })
  const l = q.data
  if (!l) return null
  const spent = usd(l.spent_today)
  return (
    <Card className="space-y-2 p-4">
      <h2 className="flex items-center gap-2 text-[15px] font-medium"><Gauge size={16} /> {t('lim.button')}</h2>
      <p className="text-[13px] text-ink-2">{l.all_models ? t('lim.allModels') : l.models.length ? t('lim.only', { list: l.models.map(label).join(', ') }) : t('lim.noneForYou')}</p>
      <p className="text-[13px] text-ink-2">
        {l.daily_usd > 0 ? t('lim.yours', { limit: usd(l.daily_usd), spent }) : l.house_usd > 0 ? t('lim.house', { limit: usd(l.house_usd), spent }) : t('lim.unlimited', { spent })}
      </p>
      {l.reached && <p className="text-[13px] text-danger" role="status">{t('lim.youReached')}</p>}
    </Card>
  )
}

// MyDevices lists what opens this person's account, so a device they did
// not add stands out, and signs any of it out.
function MyDevices() {
  const t = useT()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['my-devices'], queryFn: api.myDevices })
  const out = useMutation({ mutationFn: api.signOutDevice, onSuccess: () => qc.invalidateQueries({ queryKey: ['my-devices'] }) })
  return (
    <Card className="space-y-3 p-4">
      <div>
        <h2 className="flex items-center gap-2 text-[15px] font-medium"><MonitorSmartphone size={16} /> {t('acct.devices')}</h2>
        <p className="text-[13px] text-ink-3">{t('acct.devicesText')}</p>
      </div>
      {(list.data ?? []).length === 0 && !list.isLoading && <p className="text-[13px] text-ink-2">{t('acct.noDevices')}</p>}
      <ul className="space-y-2" aria-label={t('acct.devices')}>
        {(list.data ?? []).map((d) => (
          <li key={d.id} className="flex items-center gap-3 text-[14px]">
            <MonitorSmartphone size={14} className="text-ink-3" />
            <span className="min-w-0 flex-1">
              <span className="block font-medium">{d.name}{d.current && <span className="ml-2 text-[12px] font-normal text-ink-3">{t('acct.thisDevice')}</span>}</span>
              <span className="block text-[12px] text-ink-3">{d.pending ? t('acct.pending') : d.last_seen ? t('acct.seen', { when: relative(d.last_seen) }) : t('acct.unused')}</span>
            </span>
            <Button size="sm" variant="ghost" aria-label={t('acct.signOut', { name: d.name })} onClick={() => out.mutate(d.id)}><LogOut size={14} /></Button>
          </li>
        ))}
      </ul>
      {out.error && <p className="text-[13px] text-danger">{out.error.message}</p>}
    </Card>
  )
}
