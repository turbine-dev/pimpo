import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, CalendarDays, Check, Cpu, Mail, Sparkles } from 'lucide-react'
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

function Setup({ kind, icon, title, c, fields, help }: { kind: string; icon: ReactNode; title: string; c?: Connection; fields: Field[]; help: ReactNode }) {
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
