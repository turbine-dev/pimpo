import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, CalendarDays, Check, ChevronRight, Cpu, Mail, MessageCircle, Music, Sparkles } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button, Card } from '../components/ui'
import { Catalog, KindList, channelKinds } from '../components/Catalog'
import { TelegramBots } from '../components/TelegramBots'
import { api, type Connection } from '../lib/api'
import { cn } from '../lib/cn'
import { fill, useT } from '../lib/i18n'

export function Connections() {
  const t = useT()
  const q = useQuery({ queryKey: ['connections'], queryFn: api.connections, refetchInterval: (d) => (d.state.data?.some((c) => c.kind === 'telegram' && c.configured && !c.paired) ? 3000 : false) })
  const by = (k: Connection['kind']) => q.data?.find((c) => c.kind === k)
  return (
    <div className="mx-auto max-w-4xl space-y-10">
      <div>
        <h1 className="mb-1 text-[22px] font-semibold tracking-tight">{t('nav.connections')}</h1>
        <p className="text-sm text-ink-2">{t('conn.subtitle')}</p>
      </div>
      <Section title={t('conn.secChat')} text={t('conn.secChatText')}>
        <Telegram c={by('telegram')} />
        <TelegramBots telegram={!!by('telegram')?.configured} />
        <Setup
          kind="whatsapp"
          icon={<MessageCircle size={18} />}
          title={t('conn.whatsapp')}
          c={by('whatsapp')}
          fields={[
            { name: 'token', label: t('conn.whatsappToken'), placeholder: 'EAAG…', secret: true },
            { name: 'phone_id', label: t('conn.whatsappPhone'), placeholder: '1234567890' },
            { name: 'app_secret', label: t('conn.whatsappSecret'), placeholder: t('conn.whatsappSecretPlaceholder'), secret: true },
          ]}
          help={fill(t('conn.whatsappHelp'), { link: <a className="underline" href="https://developers.facebook.com/apps" target="_blank" rel="noreferrer">developers.facebook.com</a> })}
          extra={<WhatsAppSteps c={by('whatsapp')} />}
        />
        <KindList include={channelKinds} />
      </Section>
      <Section title={t('conn.secAccounts')} text={t('conn.secAccountsText')}>
        <GoogleSignIn connected={!!by('mail')?.detail?.includes('(Google)')} />
        <Setup
          kind="mail"
          icon={<Mail size={18} />}
          title={t('conn.mail')}
          c={by('mail')}
          fields={[
            { name: 'user', label: t('conn.mailUser'), placeholder: t('conn.mailUserPlaceholder') },
            { name: 'password', label: t('people.appPassword'), placeholder: 'xxxx xxxx xxxx xxxx', secret: true },
            { name: 'addr', label: t('conn.mailAddr'), placeholder: 'imap.gmail.com:993' },
          ]}
          help={fill(t('conn.mailHelp'), { link: <a className="underline" href="https://myaccount.google.com/apppasswords" target="_blank" rel="noreferrer">{t('conn.mailHelpLink')}</a> })}
        />
        <Setup
          kind="calendar"
          icon={<CalendarDays size={18} />}
          title={t('conn.calendar')}
          c={by('calendar')}
          fields={[{ name: 'feeds', label: t('conn.calendarFeeds'), placeholder: 'https://calendar.google.com/calendar/ical/…/basic.ics', multiline: true, secret: true }]}
          help={t('conn.calendarHelp')}
        />
      </Section>
      <Section title={t('conn.secBrain')} text={t('conn.secBrainText')}>
        <ModelsLink />
        <Setup
          kind="jev"
          icon={<Sparkles size={18} />}
          title={t('conn.jev')}
          c={by('jev')}
          fields={[{ name: 'key', label: t('conn.jevKey'), placeholder: 'ts_…', secret: true }]}
          help={t('conn.jevHelp')}
        />
      </Section>
      <Section title={t('conn.secServices')} text={t('conn.secServicesText')}>
        <SpotifySignIn />
        <Catalog />
      </Section>
    </div>
  )
}

// SpotifySignIn connects the owner's Spotify with their own app's Client
// ID (PKCE, no secret).
function SpotifySignIn() {
  const t = useT()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['spotify'], queryFn: api.spotify })
  const [open, setOpen] = useState(false)
  const [id, setId] = useState('')
  const result = new URLSearchParams(location.search).get('spotify')
  const start = useMutation({ mutationFn: () => api.spotifyStart(id.trim()), onSuccess: (r) => { location.href = r.url } })
  const off = useMutation({ mutationFn: api.spotifyOff, onSuccess: () => qc.invalidateQueries({ queryKey: ['spotify'] }) })
  const connected = !!q.data?.connected
  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <Icon ok={connected}><Music size={18} /></Icon>
        <div className="flex-1">
          <div className="text-[15px] font-medium">Spotify</div>
          <div className="text-[13px] text-ink-3">{t('conn.spotifyText')}</div>
        </div>
        {connected
          ? <><span className="flex items-center gap-1 text-[12px] font-medium text-read"><Check size={13} /> {t('conn.connected')}</span><Button size="sm" variant="ghost" onClick={() => off.mutate()}>{t('conn.disconnect')}</Button></>
          : <Button size="sm" onClick={() => setOpen(!open)}>{t('conn.setUp')}</Button>}
      </div>
      {result && result !== 'ok' && <p className="mt-3 text-[13px] text-danger">{t('conn.spotifyRefused', { reason: result })}</p>}
      {open && !connected && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); start.mutate() }}>
          <ol className="list-decimal space-y-1 pl-5 text-[13px] text-ink-2">
            <li>{fill(t('conn.spotifyStep1'), { link: <a className="underline" href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer">developer.spotify.com</a> })}</li>
            <li>{fill(t('conn.spotifyStep2'), { uri: <code className="rounded bg-sunken px-1 text-[12px]">{q.data?.redirect}</code> })}</li>
            <li>{t('conn.spotifyStep3')}</li>
          </ol>
          <input value={id} onChange={(e) => setId(e.target.value)} placeholder="Client ID" aria-label={t('conn.spotifyId')} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {start.error && <p className="text-[13px] text-danger">{start.error.message}</p>}
          <Button variant="primary" type="submit" disabled={(!id.trim() && !q.data?.client_id) || start.isPending}>{t('conn.spotifySignIn')}</Button>
        </form>
      )}
    </Card>
  )
}

function GoogleSignIn({ connected }: { connected: boolean }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [id, setId] = useState('')
  const [secret, setSecret] = useState('')
  const params = new URLSearchParams(location.search)
  const result = params.get('google')
  const start = useMutation({ mutationFn: () => api.googleStart(id.trim(), secret.trim()), onSuccess: (r) => { location.href = r.url } })
  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <Icon ok={connected}>
          <Mail size={18} />
        </Icon>
        <div className="flex-1">
          <div className="text-[15px] font-medium">{t('conn.google')}</div>
          <div className="text-[13px] text-ink-3">{t('conn.googleText')}</div>
        </div>
        {connected ? <span className="flex items-center gap-1 text-[12px] font-medium text-read"><Check size={13} /> {t('conn.connected')}</span> : <Button size="sm" onClick={() => setOpen(!open)}>{t('conn.setUp')}</Button>}
      </div>
      {result && result !== 'ok' && <p className="mt-3 text-[13px] text-danger">{t('conn.googleRefused', { reason: result })}</p>}
      {open && !connected && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); start.mutate() }}>
          <ol className="list-decimal space-y-1 pl-5 text-[13px] text-ink-2">
            <li>{fill(t('conn.googleStep1'), { link: <a className="underline" href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noreferrer">Google Cloud Console</a> })}</li>
            <li>{fill(t('conn.googleStep2'), { type: <b>{t('conn.googleClientType')}</b>, kind: <b>{t('conn.googleAppKind')}</b> })}</li>
            <li>{t('conn.googleStep3')}</li>
          </ol>
          <input value={id} onChange={(e) => setId(e.target.value)} placeholder={t('conn.googleIdPlaceholder')} aria-label={t('conn.googleId')} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <input value={secret} onChange={(e) => setSecret(e.target.value)} type="password" placeholder={t('conn.googleSecretPlaceholder')} aria-label={t('conn.googleSecret')} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {start.error && <p className="text-[13px] text-danger">{start.error.message}</p>}
          <Button variant="primary" type="submit" disabled={!id || !secret || start.isPending}>{t('conn.google')}</Button>
        </form>
      )}
    </Card>
  )
}

function WhatsAppSteps({ c }: { c?: Connection }) {
  const t = useT()
  if (!c?.configured || c.paired) return null
  return (
    <ol className="mt-4 list-decimal space-y-2 border-t border-line pt-4 pl-5 text-[13px] text-ink-2">
      <li>
        {fill(t('conn.whatsappStep1'), {
          webhook: c.webhook?.startsWith('https://') ? <code className="break-all rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">{c.webhook}</code> : fill(t('conn.whatsappNoWebhook'), { path: <code className="font-mono">/webhook/whatsapp</code> }),
          token: <code className="rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">{c.verify_token}</code>,
          field: <b>messages</b>,
        })}
      </li>
      <li>{fill(t('conn.whatsappStep2'), { code: <code className="rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">pimpo {c.pairing_code}</code> })}</li>
    </ol>
  )
}

function Icon({ ok, children }: { ok: boolean; children: ReactNode }) {
  return <div className={cn('grid size-10 shrink-0 place-items-center rounded-xl', ok ? 'bg-read-soft text-read' : 'bg-sunken text-ink-3')}>{children}</div>
}

function Status({ c }: { c?: Connection }) {
  const t = useT()
  if (!c?.configured) return <span className="text-[12px] text-ink-3">{t('conn.notConnected')}</span>
  return (
    <span className="flex items-center gap-1 text-[12px] font-medium text-read">
      <Check size={13} /> {c.detail || t('conn.connected')}
    </span>
  )
}

function Telegram({ c }: { c?: Connection }) {
  const t = useT()
  const qc = useQueryClient()
  const [token, setToken] = useState('')
  const save = useMutation({ mutationFn: () => api.connect('telegram', { token }), onSuccess: () => { setToken(''); qc.invalidateQueries({ queryKey: ['connections'] }) } })
  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <Icon ok={!!c?.paired}>
          <Bot size={18} />
        </Icon>
        <div className="flex-1">
          <div className="text-[15px] font-medium">Telegram</div>
          <div className="text-[13px] text-ink-3">{t('conn.telegramText')}</div>
        </div>
        {c?.paired ? <span className="flex items-center gap-1 text-[12px] font-medium text-read"><Check size={13} /> {c.bot ? t('conn.connectedTo', { bot: c.bot }) : t('conn.connected')}</span> : <Status c={c} />}
      </div>
      {!c?.configured && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <ol className="list-decimal space-y-1 pl-5 text-[13px] text-ink-2">
            <li>{fill(t('conn.telegramStep1'), { bot: <a className="underline" href="https://t.me/BotFather" target="_blank" rel="noreferrer">@BotFather</a>, cmd: <code className="rounded bg-sunken px-1">/newbot</code> })}</li>
            <li>{t('conn.telegramStep2')}</li>
            <li>{t('conn.telegramStep3')}</li>
          </ol>
          <input value={token} onChange={(e) => setToken(e.target.value)} placeholder="123456789:AA…" aria-label={t('conn.botToken')} type="password" className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
          <Button variant="primary" type="submit" disabled={!token || save.isPending}>{save.isPending ? t('conn.checking') : t('conn.connect')}</Button>
        </form>
      )}
      {c?.configured && !c.paired && c.pairing_code && (
        <div className="mt-4 rounded-xl border border-explore/30 bg-explore-soft p-4 text-[13.5px]">
          {fill(t('conn.telegramPair'), { bot: c.bot ? <a className="font-medium underline" href={`https://t.me/${c.bot}?start=${c.pairing_code}`} target="_blank" rel="noreferrer">@{c.bot}</a> : t('conn.yourBot') })}
          <div className="mt-2 font-mono text-[18px] font-semibold tracking-wider">/start {c.pairing_code}</div>
          <div className="mt-2 text-[12px] text-ink-3">{t('conn.autoRefresh')}</div>
        </div>
      )}
    </Card>
  )
}

type Field = { name: string; label: string; placeholder: string; secret?: boolean; multiline?: boolean }

function Setup({ kind, icon, title, c, fields, help, extra }: { kind: string; icon: ReactNode; title: string; c?: Connection; fields: Field[]; help: ReactNode; extra?: ReactNode }) {
  const t = useT()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [values, setValues] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: () => api.connect(kind, values),
    onSuccess: () => {
      setOpen(false)
      setValues({})
      qc.invalidateQueries({ queryKey: ['connections'] })
    },
  })
  const remove = useMutation({ mutationFn: () => api.disconnect(kind), onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }) })
  return (
    <Card className="p-5">
      <div className="flex items-center gap-4">
        <Icon ok={!!c?.configured}>{icon}</Icon>
        <div className="min-w-0 flex-1">
          <div className="text-[15px] font-medium">{title}</div>
          <Status c={c} />
        </div>
        {c?.configured ? (
          <div className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={() => setOpen(!open)}>{t('conn.change')}</Button>
            <Button size="sm" variant="ghost" onClick={() => remove.mutate()}>{t('common.remove')}</Button>
          </div>
        ) : (
          <Button size="sm" onClick={() => setOpen(!open)}>{t('conn.connect')}</Button>
        )}
      </div>
      {extra}
      {open && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <p className="text-[13px] text-ink-2">{help}</p>
          {fields.map((f) => (
            <label key={f.name} className="block">
              <span className="mb-1 block text-[12.5px] font-medium text-ink-2">{f.label}</span>
              {f.multiline ? (
                <textarea rows={3} value={values[f.name] ?? ''} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })} placeholder={f.placeholder} className="w-full rounded-[10px] border border-line bg-bg px-3 py-2 font-mono text-[12.5px] outline-none focus:border-accent" />
              ) : (
                <input type={f.secret ? 'password' : 'text'} value={values[f.name] ?? ''} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })} placeholder={f.placeholder} className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
              )}
            </label>
          ))}
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
          <Button variant="primary" type="submit" disabled={save.isPending}>{t('common.save')}</Button>
        </form>
      )}
    </Card>
  )
}

function Section({ title, text, children }: { title: string; text: string; children: ReactNode }) {
  return (
    <section aria-label={title}>
      <h2 className="text-[16px] font-semibold tracking-tight">{title}</h2>
      <p className="mb-3 text-[13px] text-ink-3">{text}</p>
      <div className="space-y-3">{children}</div>
    </section>
  )
}

// ModelsLink shows which model does the work, with the way to change it
// first in the brain section, since nothing works without one.
function ModelsLink() {
  const t = useT()
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const models = useQuery({ queryKey: ['models'], queryFn: api.models })
  const s = settings.data
  const cc = ['sonnet', 'opus', 'haiku']
  const name = (id?: string) => (!id ? '' : cc.includes(id) ? `Claude Code · ${id}` : id === 'codex' ? 'Codex · ChatGPT' : id)
  const ok = !!s && (!cc.includes(s.explore_model) || !!models.data?.claude_code)
  const spare = s?.fallbacks?.explore?.length ?? 0
  return (
    <Card className="flex items-center gap-4 p-5">
      <Icon ok={ok}><Cpu size={18} /></Icon>
      <div className="min-w-0 flex-1">
        <div className="text-[15px] font-medium">{t('ms.inUse')}</div>
        <div className="truncate text-[13px] text-ink-3">
          {s ? `${t('models.explore')}: ${name(s.explore_model)}${spare ? ` · ${t('ms.fallbacks')} ${spare}` : ''}` : t('ui.loading')}
          {s && !ok && ` — ${t('ms.claudeMissing')}`}
        </div>
      </div>
      <Link to="/settings#modelos"><Button size="sm" variant={ok ? 'secondary' : 'primary'}>{t('conn.setUp')} <ChevronRight size={14} /></Button></Link>
    </Card>
  )
}
