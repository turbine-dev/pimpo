import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, CalendarDays, Check, Cpu, Mail, MessageCircle, Sparkles } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Button, Card } from '../components/ui'
import { api, type Connection } from '../lib/api'
import { cn } from '../lib/cn'

export function Connections() {
  const q = useQuery({ queryKey: ['connections'], queryFn: api.connections, refetchInterval: (d) => (d.state.data?.some((c) => c.kind === 'telegram' && c.configured && !c.paired) ? 3000 : false) })
  const by = (k: Connection['kind']) => q.data?.find((c) => c.kind === k)
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-[22px] font-semibold tracking-tight">Conexões</h1>
      <p className="mb-6 text-sm text-ink-2">As chaves ficam criptografadas no seu computador. O agente nunca as vê.</p>
      <div className="space-y-4">
        <Telegram c={by('telegram')} />
        <GoogleSignIn connected={!!by('mail')?.detail?.includes('(Google)')} />
        <Setup
          kind="mail"
          icon={<Mail size={18} />}
          title="E-mail (Gmail ou IMAP)"
          c={by('mail')}
          fields={[
            { name: 'user', label: 'Seu e-mail', placeholder: 'voce@gmail.com' },
            { name: 'password', label: 'Senha de app', placeholder: 'xxxx xxxx xxxx xxxx', secret: true },
            { name: 'addr', label: 'Servidor IMAP (opcional)', placeholder: 'imap.gmail.com:993' },
          ]}
          help={
            <>
              No Gmail: ative a verificação em duas etapas e crie uma <a className="underline" href="https://myaccount.google.com/apppasswords" target="_blank" rel="noreferrer">senha de app</a>. Ela dá acesso só ao e-mail e pode ser revogada quando quiser.
            </>
          }
        />
        <Setup
          kind="calendar"
          icon={<CalendarDays size={18} />}
          title="Agenda"
          c={by('calendar')}
          fields={[{ name: 'feeds', label: 'Links iCal privados (um por linha)', placeholder: 'https://calendar.google.com/calendar/ical/…/basic.ics', multiline: true, secret: true }]}
          help={<>No Google Agenda: Configurações → sua agenda → “Endereço secreto no formato iCal”. Só leitura, sem login.</>}
        />
        <Setup
          kind="whatsapp"
          icon={<MessageCircle size={18} />}
          title="WhatsApp (API oficial)"
          c={by('whatsapp')}
          fields={[
            { name: 'token', label: 'Token de acesso permanente', placeholder: 'EAAG…', secret: true },
            { name: 'phone_id', label: 'ID do número de telefone', placeholder: '1234567890' },
            { name: 'app_secret', label: 'Chave secreta do app', placeholder: 'confere a assinatura das mensagens', secret: true },
          ]}
          help={
            <>
              Crie um app em <a className="underline" href="https://developers.facebook.com/apps" target="_blank" rel="noreferrer">developers.facebook.com</a> com o produto WhatsApp e um número próprio. É a via oficial: sem risco de bloqueio do seu número pessoal.
            </>
          }
          extra={<WhatsAppSteps c={by('whatsapp')} />}
        />
        <Setup
          kind="jev"
          icon={<Sparkles size={18} />}
          title="Jev (julgamentos calibrados, opcional)"
          c={by('jev')}
          fields={[{ name: 'key', label: 'Chave da TypeSafe', placeholder: 'ts_…', secret: true }]}
          help={<>Sem chave, os julgamentos usam um modelo local (Ollama) ou o seu modelo de linguagem.</>}
        />
        <Card className="flex items-center gap-4 p-5">
          <Icon ok={!!by('claude')?.configured}>
            <Cpu size={18} />
          </Icon>
          <div className="flex-1">
            <div className="text-[15px] font-medium">Claude Code</div>
            <div className="text-[13px] text-ink-3">{by('claude')?.configured ? 'Encontrado. Uso a sua assinatura para explorar e compilar.' : 'Não encontrado. Instale o Claude Code para explorar tarefas novas.'}</div>
          </div>
        </Card>
      </div>
    </div>
  )
}

function GoogleSignIn({ connected }: { connected: boolean }) {
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
          <div className="text-[15px] font-medium">Entrar com Google</div>
          <div className="text-[13px] text-ink-3">E-mail e agenda de uma vez, sem senha de app. Usa o seu próprio cliente OAuth: nada passa por servidores do Vigia.</div>
        </div>
        {connected ? <span className="flex items-center gap-1 text-[12px] font-medium text-read"><Check size={13} /> Conectado</span> : <Button size="sm" onClick={() => setOpen(!open)}>Configurar</Button>}
      </div>
      {result && result !== 'ok' && <p className="mt-3 text-[13px] text-danger">O Google recusou: {result}</p>}
      {open && !connected && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); start.mutate() }}>
          <ol className="list-decimal space-y-1 pl-5 text-[13px] text-ink-2">
            <li>No <a className="underline" href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noreferrer">Google Cloud Console</a>, crie um projeto e ative as APIs Gmail e Google Calendar.</li>
            <li>Crie uma credencial <b>ID do cliente OAuth</b> do tipo <b>App para computador</b>.</li>
            <li>Cole o ID e a chave secreta aqui e entre com a sua conta.</li>
          </ol>
          <input value={id} onChange={(e) => setId(e.target.value)} placeholder="ID do cliente (…apps.googleusercontent.com)" aria-label="ID do cliente Google" className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          <input value={secret} onChange={(e) => setSecret(e.target.value)} type="password" placeholder="Chave secreta do cliente" aria-label="Chave secreta do cliente Google" className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {start.error && <p className="text-[13px] text-danger">{start.error.message}</p>}
          <Button variant="primary" type="submit" disabled={!id || !secret || start.isPending}>Entrar com Google</Button>
        </form>
      )}
    </Card>
  )
}

function WhatsAppSteps({ c }: { c?: Connection }) {
  if (!c?.configured || c.paired) return null
  return (
    <ol className="mt-4 list-decimal space-y-2 border-t border-line pt-4 pl-5 text-[13px] text-ink-2">
      <li>
        No app da Meta, em WhatsApp › Configuração, use como webhook
        {c.webhook?.startsWith('https://') ? <code className="mx-1 break-all rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">{c.webhook}</code> : <> o endereço público do Vigia (defina em Ajustes › Abrir no celular) seguido de <code className="font-mono">/webhook/whatsapp</code></>}
        e o token de verificação <code className="rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">{c.verify_token}</code>. Assine o campo <b>messages</b>.
      </li>
      <li>Do seu WhatsApp, mande para o número do app: <code className="rounded bg-sunken px-1.5 py-0.5 font-mono text-[12px]">vigia {c.pairing_code}</code></li>
    </ol>
  )
}

function Icon({ ok, children }: { ok: boolean; children: ReactNode }) {
  return <div className={cn('grid size-10 shrink-0 place-items-center rounded-xl', ok ? 'bg-read-soft text-read' : 'bg-sunken text-ink-3')}>{children}</div>
}

function Status({ c }: { c?: Connection }) {
  if (!c?.configured) return <span className="text-[12px] text-ink-3">Não conectado</span>
  return (
    <span className="flex items-center gap-1 text-[12px] font-medium text-read">
      <Check size={13} /> {c.detail || 'Conectado'}
    </span>
  )
}

function Telegram({ c }: { c?: Connection }) {
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
          <div className="text-[13px] text-ink-3">Onde eu converso com você, peço aprovações e aviso quando algo falha.</div>
        </div>
        {c?.paired ? <span className="flex items-center gap-1 text-[12px] font-medium text-read"><Check size={13} /> Conectado{c.bot ? ` a @${c.bot}` : ''}</span> : <Status c={c} />}
      </div>
      {!c?.configured && (
        <form className="mt-4 space-y-3 border-t border-line pt-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <ol className="list-decimal space-y-1 pl-5 text-[13px] text-ink-2">
            <li>No Telegram, fale com <a className="underline" href="https://t.me/BotFather" target="_blank" rel="noreferrer">@BotFather</a> e envie <code className="rounded bg-sunken px-1">/newbot</code>.</li>
            <li>Escolha um nome. Ele te devolve um token.</li>
            <li>Cole o token aqui.</li>
          </ol>
          <input value={token} onChange={(e) => setToken(e.target.value)} placeholder="123456789:AA…" aria-label="Token do bot" type="password" className="h-10 w-full rounded-[10px] border border-line bg-bg px-3 text-sm outline-none focus:border-accent" />
          {save.error && <p className="text-[13px] text-danger">{save.error.message}</p>}
          <Button variant="primary" type="submit" disabled={!token || save.isPending}>{save.isPending ? 'Conferindo…' : 'Conectar'}</Button>
        </form>
      )}
      {c?.configured && !c.paired && c.pairing_code && (
        <div className="mt-4 rounded-xl border border-explore/30 bg-explore-soft p-4 text-[13.5px]">
          Agora abra {c.bot ? <a className="font-medium underline" href={`https://t.me/${c.bot}?start=${c.pairing_code}`} target="_blank" rel="noreferrer">@{c.bot}</a> : 'o seu bot'} e envie:
          <div className="mt-2 font-mono text-[18px] font-semibold tracking-wider">/start {c.pairing_code}</div>
          <div className="mt-2 text-[12px] text-ink-3">Esta tela atualiza sozinha quando você enviar.</div>
        </div>
      )}
    </Card>
  )
}

type Field = { name: string; label: string; placeholder: string; secret?: boolean; multiline?: boolean }

function Setup({ kind, icon, title, c, fields, help, extra }: { kind: string; icon: ReactNode; title: string; c?: Connection; fields: Field[]; help: ReactNode; extra?: ReactNode }) {
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
            <Button size="sm" variant="ghost" onClick={() => setOpen(!open)}>Trocar</Button>
            <Button size="sm" variant="ghost" onClick={() => remove.mutate()}>Remover</Button>
          </div>
        ) : (
          <Button size="sm" onClick={() => setOpen(!open)}>Conectar</Button>
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
          <Button variant="primary" type="submit" disabled={save.isPending}>Salvar</Button>
        </form>
      )}
    </Card>
  )
}
