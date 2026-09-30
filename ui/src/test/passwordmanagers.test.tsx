import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PasswordManagers } from '../components/PasswordManagers'
import { TelegramBots } from '../components/TelegramBots'
import { Account } from '../pages/Account'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const none = { house: true, op_installed: true, onepassword: { mode: '' }, hashicorp: { auth: '' } }

describe('Password managers', () => {
  it('keeps a reference in a secret field and checks it without showing the value', async () => {
    const calls = mockFetch({
      '/api/telegram/bots': [],
      'POST /api/secrets/check': { found: true },
      'POST /api/telegram/bots': { id: 'b1', name: 'Casa', username: 'casa_bot' },
    })
    wrap(<TelegramBots />)
    const token = await screen.findByLabelText('Token do bot')
    expect(token).toHaveAttribute('type', 'password')
    await userEvent.click(screen.getByRole('switch', { name: 'Usar uma referência' }))
    expect(token).toHaveAttribute('type', 'text')
    await userEvent.type(token, 'op://Casa/Telegram/credential')
    await userEvent.click(screen.getByRole('button', { name: 'Conferir' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Encontrado')
    expect(calls.find((c) => c.url === '/api/secrets/check')?.body).toEqual({ reference: 'op://Casa/Telegram/credential' })
    await userEvent.click(screen.getByRole('button', { name: 'Adicionar bot' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url === '/api/telegram/bots')?.body).toEqual({ name: '', token: 'op://Casa/Telegram/credential' }))
  })

  it('shows why a reference was not found', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => url === '/api/secrets/check'
      ? new Response(JSON.stringify({ error: 'the password manager reference op://Casa/Nada/x could not be read: 1Password is not set up for the house' }), { status: 400 })
      : new Response('[]', { status: 200 })))
    wrap(<TelegramBots />)
    await userEvent.click(await screen.findByRole('switch', { name: 'Usar uma referência' }))
    await userEvent.type(screen.getByLabelText('Token do bot'), 'op://Casa/Nada/x')
    await userEvent.click(screen.getByRole('button', { name: 'Conferir' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('op://Casa/Nada/x could not be read')
  })

  it('sets up the house 1Password and tests it; tokens never come back', async () => {
    let state: Record<string, unknown> = none
    const calls = mockFetch({
      '/api/password-managers': () => state,
      'PUT /api/password-managers/onepassword': () => (state = { ...none, onepassword: { mode: 'service' } }),
      'POST /api/password-managers/onepassword/test': { ok: true },
    })
    wrap(<PasswordManagers />)
    expect(await screen.findByText(/Estes são os da casa/)).toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Configurar' })[0])
    expect(screen.getByRole('radio', { name: 'O app 1Password neste computador' })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Token da conta de serviço'), 'ops_secret')
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ mode: 'service', token: 'ops_secret' }))
    expect(await screen.findByText('Conta de serviço')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('ops_secret')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Testar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Funciona')
  })

  it('lets a member keep their own, without the house app', async () => {
    const own = { ...none, house: false }
    const calls = mockFetch({
      '/api/state': { role: 'member' },
      '/api/passkeys': [],
      '/api/me/devices': [],
      '/api/password-managers': own,
      'PUT /api/password-managers/hashicorp': { ...own, hashicorp: { auth: 'approle', addr: 'https://vault.ana.example' } },
    })
    wrap(<Account />)
    expect(await screen.findByText(/Seu próprio 1Password/)).toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Configurar' })[0])
    expect(screen.queryByRole('radio', { name: 'O app 1Password neste computador' })).not.toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Configurar' })[1])
    await userEvent.type(screen.getByLabelText('Endereço do Vault'), 'https://vault.ana.example')
    await userEvent.click(screen.getByRole('radio', { name: 'AppRole' }))
    await userEvent.type(screen.getByLabelText('Role ID'), 'role')
    await userEvent.type(screen.getByLabelText('Secret ID'), 'sid')
    await userEvent.click(screen.getAllByRole('button', { name: 'Salvar' }).at(-1)!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ addr: 'https://vault.ana.example', auth: 'approle', role_id: 'role', secret_id: 'sid' }))
    expect(await screen.findByText('https://vault.ana.example · AppRole')).toBeInTheDocument()
  })
})
