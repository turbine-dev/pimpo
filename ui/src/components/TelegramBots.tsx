import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { useT } from '../lib/i18n'
import { Button, Card } from './ui'

const input = 'h-10 rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent'

// Extra bots only send: a routine picks them as destinations.
export function TelegramBots() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['bots'], queryFn: api.bots })
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const refresh = () => { qc.invalidateQueries({ queryKey: ['bots'] }); qc.invalidateQueries({ queryKey: ['destinations'] }) }
  const add = useMutation({ mutationFn: () => api.addBot(name, token), onSuccess: () => { setName(''); setToken(''); refresh() } })
  const detect = useMutation({ mutationFn: api.detectBot, onSuccess: refresh })
  const remove = useMutation({ mutationFn: api.removeBot, onSuccess: refresh })
  const bots = q.data ?? []
  return (
    <Card className="p-5">
      <div className="mb-1 flex items-center gap-2 text-[15px] font-medium"><Bot size={17} /> {t('bots.title')}</div>
      <p className="mb-4 text-[13px] text-ink-3">{t('bots.text')}</p>
      {bots.length > 0 && (
        <ul className="mb-4 divide-y divide-line rounded-xl border border-line" aria-label={t('bots.title')}>
          {bots.map((b) => (
            <li key={b.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-[13.5px]">
              <span className="font-medium">{b.name}</span>
              <span className="text-ink-3">@{b.username}{b.chat_name ? ` → ${b.chat_name}` : ` · ${t('bots.noChat')}`}</span>
              <span className="flex-1" />
              <Button size="sm" variant="ghost" onClick={() => detect.mutate(b.id)} disabled={detect.isPending}>{t('bots.detect')}</Button>
              <Button size="sm" variant="ghost" aria-label={t('bots.remove', { name: b.name })} onClick={() => remove.mutate(b.id)}><Trash2 size={14} /></Button>
            </li>
          ))}
        </ul>
      )}
      {detect.error && <p className="mb-3 text-[13px] text-danger">{detect.error.message}</p>}
      <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); add.mutate() }}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('bots.name')} aria-label={t('bots.name')} className={input + ' w-40'} />
        <input value={token} onChange={(e) => setToken(e.target.value)} type="password" placeholder={t('bots.token')} aria-label={t('bots.token')} className={input + ' min-w-[220px] flex-1'} />
        <Button type="submit" disabled={!token.trim() || add.isPending}>{t('bots.add')}</Button>
      </form>
      {add.error && <p className="mt-2 text-[13px] text-danger">{add.error.message}</p>}
    </Card>
  )
}
