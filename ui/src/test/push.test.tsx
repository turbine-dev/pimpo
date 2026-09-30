import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GitHubCard } from '../components/GitHubCard'
import { RoutineSettings } from '../components/RoutineSettings'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const watcher = (capability: string) => ({
  id: 'chefe', name: 'E-mail do chefe', description: '', state: 'active', version: 1, runs: [], cost_month_usd: 0, capabilities: [capability, 'notify.send'],
  schedule: '', default_schedule: '', params: [], values: {}, watch: { capability, key: 'id', every: '10m' },
}) as never

describe('push triggers', () => {
  it('turns Gmail push on and shows until when it is live', async () => {
    const calls = mockFetch({
      '/api/destinations': [],
      '/api/routines/chefe/push': { kind: 'gmail', on: false, live: false, gmail: { configured: true, signed_in: true } },
      'POST /api/routines/chefe/push/on': { kind: 'gmail', on: true, live: true, gmail: { configured: true, signed_in: true, until: '2026-10-07T09:00:00Z' } },
    })
    wrap(<RoutineSettings s={watcher('gmail.search')} />)
    const sw = await screen.findByRole('switch', { name: 'Saber na hora' })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    await userEvent.click(sw)
    expect(await screen.findByText(/Push ligado: o Gmail avisa o Pimpo/)).toBeInTheDocument()
    expect(screen.getByText(/uma vez por hora/)).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'POST' && c.url === '/api/routines/chefe/push/on')).toBe(true)
    // Turning push on is not a change to the routine's saved settings.
    expect(screen.getByRole('button', { name: 'Salvar ajustes' })).toBeDisabled()
  })

  it('lets the administrator set up Gmail push once', async () => {
    const calls = mockFetch({
      '/api/state': { role: 'owner' },
      '/api/destinations': [],
      '/api/routines/chefe/push': { kind: 'gmail', on: true, live: false, gmail: { configured: false, signed_in: true } },
      '/api/push/gmail': { topic: '', account: '', endpoint: 'https://pimpo.example.ts.net/push/gmail', ready: false },
      'PUT /api/push/gmail': { topic: 'projects/casa-123/topics/gmail', account: 'push@casa-123.iam.gserviceaccount.com', endpoint: 'https://pimpo.example.ts.net/push/gmail', ready: true },
      'POST /api/routines/chefe/push/on': { kind: 'gmail', on: true, live: true, gmail: { configured: true, signed_in: true, until: '2026-10-07T09:00:00Z' } },
    })
    wrap(<RoutineSettings s={watcher('gmail.search')} />)
    const setup = await screen.findByRole('group', { name: 'Configurar push do Gmail' })
    expect(setup).toHaveTextContent('https://pimpo.example.ts.net/push/gmail')
    expect(screen.getByText(/verificando a cada poucos minutos/)).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Tópico'), 'projects/casa-123/topics/gmail')
    await userEvent.type(screen.getByLabelText('Conta de serviço da assinatura push'), 'push@casa-123.iam.gserviceaccount.com')
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.url === '/api/push/gmail')?.body).toEqual({ topic: 'projects/casa-123/topics/gmail', account: 'push@casa-123.iam.gserviceaccount.com' }))
    expect(await screen.findByText(/Push ligado/)).toBeInTheDocument()
  })

  it('tells a member the administrator sets it up', async () => {
    const calls = mockFetch({
      '/api/state': { role: 'member' },
      '/api/destinations': [],
      '/api/routines/chefe/push': { kind: 'gmail', on: true, live: false, gmail: { configured: false, signed_in: false } },
    })
    wrap(<RoutineSettings s={watcher('gmail.search')} />)
    expect(await screen.findByText(/configuração única no Google Cloud, feita pelo administrador/)).toBeInTheDocument()
    expect(calls.some((c) => c.url === '/api/push/gmail')).toBe(false)
  })

  it('says when Slack push is waiting for Slack', async () => {
    mockFetch({ '/api/destinations': [], '/api/routines/chefe/push': { kind: 'slack', on: true, live: false, slack: { connected: false, owner_only: false } } })
    wrap(<RoutineSettings s={watcher('slack.messages')} />)
    expect(await screen.findByText('O Slack não está conectado; conecte em Conexões.')).toBeInTheDocument()
    expect(screen.getByText(/lê canais do slack/i)).toBeInTheDocument()
  })

  it('offers nothing for routines that can only poll', async () => {
    mockFetch({ '/api/destinations': [], '/api/routines/chefe/push': { kind: '', on: false, live: false } })
    wrap(<RoutineSettings s={watcher('phone.photos')} />)
    await waitFor(() => expect(screen.getByLabelText('Verificar a cada')).toBeInTheDocument())
    expect(screen.queryByRole('switch', { name: 'Saber na hora' })).toBeNull()
  })
})

describe('GitHub webhook', () => {
  it('shows the secret once, when it is made', async () => {
    const calls = mockFetch({
      '/api/routines/pr/github': { on: false },
      'POST /api/routines/pr/github/on': { on: true, urls: { public: 'https://pimpo.example.ts.net/github-hook/pr' }, secret: 'abc123secret' },
    })
    wrap(<GitHubCard id="pr" />)
    await userEvent.click(await screen.findByRole('switch', { name: 'Começar pelo GitHub' }))
    expect(await screen.findByText('abc123secret')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('só aparece uma vez')
    expect(screen.getByText('https://pimpo.example.ts.net/github-hook/pr')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copiar segredo' })).toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(1)
  })
})
