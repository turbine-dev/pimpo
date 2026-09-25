import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Import } from '../pages/Import'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const plan = {
  from: 'hermes', home: '/h', memories: [{ text: 'Gym on Tuesdays', topic: 'memória' }],
  tasks: [{ name: 'daily-brief', prompt: 'Summarize my unread email', schedule: 'cron 0 9 * * *', timezone: 'America/Sao_Paulo', deliver: 'telegram:42', enabled: true }],
  rules: [{ file: 'SOUL.md', text: 'Be terse' }],
  skills: [
    { name: 'triage', description: '', capabilities: ['gmail.search'], missing: [], verdict: 'works' },
    { name: 'deploy', description: '', capabilities: [], missing: ['roda comandos no terminal'], verdict: 'no' },
  ],
  telegram: { has_bot: true }, mail: {}, warnings: ['tarefa "backup" só roda um script'],
}

describe('Import', () => {
  it('shows what would come over before importing, and keeps tokens and trust off by default', async () => {
    const calls = mockFetch({ '/api/migrate/preview': plan, '/api/migrate/apply': { memories: 1, rules: 1, tasks: 1, telegram: false, mail: false } })
    wrap(<Import />)
    await userEvent.click(screen.getByRole('radio', { name: /Hermes/ }))
    await userEvent.click(screen.getByRole('button', { name: /Ver o que vem/ }))
    expect(await screen.findByText('daily-brief')).toBeInTheDocument()
    expect(screen.getByText('Vira rotina')).toBeInTheDocument()
    expect(screen.getByText('roda comandos no terminal')).toBeInTheDocument()
    expect(screen.getByText(/backup/)).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: /Copiar tokens/ })).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByRole('switch', { name: /Confiar/ })).toHaveAttribute('aria-checked', 'false')
    await userEvent.click(screen.getByRole('switch', { name: /Regras/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Importar' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/migrate/apply')?.body).toEqual({ from: 'hermes', home: '', memories: true, rules: false, tasks: true, secrets: false, trust: false }))
    expect(await screen.findByText('Pronto')).toBeInTheDocument()
  })
})

describe('PhonePairing', () => {
  it('turns on home and anywhere access, pairs a device and revokes another', async () => {
    const { PhonePairing } = await import('../components/PhonePairing')
    let remote: Record<string, unknown> = { tailscale: { state: 'off' }, lan: { on: false } }
    const calls = mockFetch({
      '/api/pairing': { base: '', devices: [{ id: 'd1', name: 'Tablet', created: new Date().toISOString() }] },
      '/api/remote': () => remote,
      'POST /api/remote/lan/on': () => (remote = { ...remote, lan: { on: true, url: 'http://192.168.1.20:7788' } }),
      'POST /api/remote/tailscale/on': () => (remote = { ...remote, tailscale: { state: 'needs_login', auth_url: 'https://login.tailscale.com/a/x' } }),
      'POST /api/pairing': { base: '', id: 'd2', link: 'http://192.168.1.20:7788/auth?token=t' },
      'DELETE /api/devices/d1': {},
    })
    wrap(<PhonePairing />)
    expect(await screen.findByText('Ligue uma das opções acima para gerar o código.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Gerar código' })).toBeDisabled()
    await userEvent.click(screen.getByRole('switch', { name: 'Em casa' }))
    expect(await screen.findByText('http://192.168.1.20:7788')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('switch', { name: 'De qualquer lugar' }))
    expect(await screen.findByRole('link', { name: 'Entrar no Tailscale' })).toHaveAttribute('href', 'https://login.tailscale.com/a/x')
    await userEvent.type(screen.getByLabelText('Nome do aparelho'), 'Celular')
    await userEvent.click(screen.getByRole('button', { name: 'Gerar código' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url === '/api/pairing')?.body).toEqual({ base: '', device: 'Celular' }))
    expect(await screen.findByAltText(/Código QR/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Desconectar Tablet' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/devices/d1')).toBe(true))
  })
  it('shows the one-click link when Tailscale needs HTTPS and Funnel', async () => {
    const { PhonePairing } = await import('../components/PhonePairing')
    mockFetch({
      '/api/pairing': { base: '', devices: [] },
      '/api/remote': { tailscale: { state: 'needs_funnel', auth_url: 'https://login.tailscale.com/f/funnel?node=n1' }, lan: { on: false } },
    })
    wrap(<PhonePairing />)
    expect(await screen.findByRole('link', { name: 'Ativar HTTPS e Funnel' })).toHaveAttribute('href', 'https://login.tailscale.com/f/funnel?node=n1')
  })
})
