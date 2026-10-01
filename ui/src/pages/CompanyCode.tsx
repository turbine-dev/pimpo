import { useQuery } from '@tanstack/react-query'
import { Field } from '../components/Modal'
import { Switch } from '../components/ui'
import { api, type Member, type Org } from '../lib/api'
import { useT } from '../lib/i18n'
import { area, field } from './Companies'

// splitCoder reads a member's coder setting: the CLI, then its model.
export function splitCoder(coder?: string): { cli: string; model: string } {
  if (!coder) return { cli: 'claude', model: '' }
  const [cli, ...rest] = coder.split(':')
  return { cli, model: rest.join(':') }
}

// codes says whether a member can use code.workspace, itself or by role.
export function codes(org: Org, m: Member) {
  const caps = m.capabilities?.length ? m.capabilities : org.roles.find((r) => r.id === m.role)?.capabilities ?? []
  return caps.includes('code.workspace')
}

export function CoderChoice({ org, m, onChange }: { org: Org; m: Member; onChange: (m: Member) => void }) {
  const t = useT()
  const coders = useQuery({ queryKey: ['company-coders', org.id], queryFn: () => api.companyCoders(org.id) })
  const { cli, model } = splitCoder(m.coder)
  const set = (cli: string, model: string, sandbox = m.code_sandbox) => {
    const coder = cli === 'claude' && !model ? undefined : model ? `${cli}:${model}` : cli
    onChange({ ...m, coder, code_sandbox: cli === 'opencode' ? false : sandbox })
  }
  const missing = coders.data?.find((c) => c.id === cli && !c.installed)
  return (
    <fieldset className="space-y-3 rounded-xl border border-line p-3">
      <legend className="px-1 text-[13px] font-medium">{t('co.coding')}</legend>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t('co.coder')}>
          <select className={field} value={cli} onChange={(e) => set(e.target.value, '')}>
            {(coders.data ?? [{ id: 'claude', name: 'Claude Code' }]).map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </Field>
        <Field label={t('co.coderModel')}>
          <input className={field} value={model} placeholder={cli === 'opencode' ? 'provider/model' : t('co.coderDefault')} onChange={(e) => set(cli, e.target.value.trim())} />
        </Field>
      </div>
      {missing && <p className="text-[12.5px] text-change">{t('co.coderMissing', { name: missing.name })}</p>}
      <div className="flex items-center justify-between gap-3 text-[13px]">
        <span>{t('co.codeSandbox')}</span>
        <Switch on={!!m.code_sandbox} disabled={cli === 'opencode'} onChange={(on) => set(cli, model, on)} label={t('co.codeSandbox')} />
      </div>
      <p className="text-[12px] text-ink-3">{cli === 'opencode' ? t('co.codeSandboxNone') : m.code_sandbox ? t('co.codeSandboxOn') : t('co.codeSandboxOff')}</p>
    </fieldset>
  )
}

// envText and envOf turn a company's coding variables into lines and back.
export const envText = (env?: Record<string, string>) => Object.entries(env ?? {}).map(([k, v]) => `${k}=${v}`).join('\n')

export function envOf(text: string): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const i = line.indexOf('=')
    if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return Object.keys(out).length ? out : undefined
}

export function CodeEnvField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const t = useT()
  return (
    <div>
      <Field label={t('co.codeEnv')}>
        <textarea className={area + ' font-mono text-[12.5px]'} value={value} placeholder="API_URL=http://localhost:8080" onChange={(e) => onChange(e.target.value)} />
      </Field>
      <p className="mt-1 text-[12px] text-ink-3">{t('co.codeEnvHint')}</p>
    </div>
  )
}
