import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Loader2, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { useT } from '../lib/i18n'
import { addPasskey, canUsePasskeys } from '../lib/passkey'
import { Button, Card } from '../components/ui'

// Account is each person's own: the passkeys that sign them in.
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
        ) : <p className="text-[13px] text-ink-2">{t('acct.unsupported')}</p>}
        {add.error && <p className="text-[13px] text-danger">{add.error.message}</p>}
        <p className="text-[12.5px] text-ink-3">{t('acct.where')}</p>
      </Card>
    </div>
  )
}
