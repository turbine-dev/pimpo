import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Account } from '../pages/Account'
import { Assistants } from '../pages/Assistants'
import { Chat } from '../pages/Chat'
import { People } from '../pages/People'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const house = {
  '/api/models': { keys: {}, claude_code: true, providers: [], auto: { light: 'haiku', strong: 'opus', base: 'sonnet', weigher: 'rules' } },
  '/api/models/detect': { claude_code: '2.0' },
  '/api/settings': { explore_model: 'sonnet', compile_model: 'sonnet', judge_model: 'haiku', models: [{ id: 'openai:gpt-5-mini', price_in: 0.25, price_out: 2 }] },
}
const ana = { id: 'ana', name: 'Ana', role: 'member', created: '', mail: false, calendar: false, daily_limit: 0.5, limit_reached: true }

describe('Models and spending per person', () => {
  it('lets the owner choose a person’s models and daily limit, and shows only whether it was reached', async () => {
    const calls = mockFetch({ ...house, '/api/people': [{ id: 'owner', name: 'Você', role: 'owner', created: '', mail: false, calendar: false }, ana], 'PUT /api/people/ana/limits': ana })
    wrap(<People />)
    expect(await screen.findByText('Chegou ao limite de hoje')).toBeInTheDocument()
    expect(screen.getByText('Limite de $0.50 por dia')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Modelos e gastos' }))
    const models = screen.getByRole('group', { name: 'Modelos que Ana pode usar' })
    await userEvent.click(within(models).getByRole('checkbox', { name: 'Todos os modelos da casa' }))
    const save = screen.getByRole('button', { name: 'Salvar' })
    expect(save).toBeDisabled()
    await userEvent.click(within(models).getByRole('checkbox', { name: 'Claude Code · haiku' }))
    await userEvent.click(within(models).getByRole('checkbox', { name: 'openai:gpt-5-mini' }))
    await userEvent.type(screen.getByLabelText('Limite diário (US$)'), '0,4')
    await userEvent.click(save)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ models: ['haiku', 'openai:gpt-5-mini'], daily_usd: 0.4 }))
  })

  it('shows each person their own limits and spending in their account', async () => {
    mockFetch({ '/api/passkeys': [], '/api/me/devices': [], '/api/me/limits': { models: ['haiku'], all_models: false, daily_usd: 0.5, house_usd: 2, spent_today: 0.5, reached: true } })
    wrap(<Account />)
    expect(await screen.findByText('Só: Claude Code · haiku')).toBeInTheDocument()
    expect(screen.getByText('Seu limite diário: $0.50 · gasto hoje: $0.50')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Você chegou ao seu limite de hoje')
  })

  it('offers a person only the models they and the assistant may use', async () => {
    mockFetch({
      '/api/chats': [],
      '/api/chats/c1': { chat: { id: 'c1', title: 'x', assistant: 'escritor' }, model: 'auto', turns: [] },
      '/api/assistants': [{ id: 'escritor', name: 'Escritor', emoji: '✍️', instructions: '', capabilities: [], models: ['haiku', 'opus'] }],
      '/api/models': null, '/api/settings': null,
      '/api/me/limits': { models: ['haiku', 'sonnet'], all_models: false, daily_usd: 0, house_usd: 1, spent_today: 0, reached: false },
    })
    wrap(<Routes><Route path="/chat/:id" element={<Chat />} /></Routes>, '/chat/c1')
    await userEvent.click(await screen.findByRole('button', { name: 'Modelo da conversa' }))
    expect(await screen.findByRole('menuitemradio', { name: 'Claude Code · haiku' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitemradio', { name: 'Claude Code · sonnet' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitemradio', { name: 'Claude Code · opus' })).not.toBeInTheDocument()
  })

  it('limits an assistant to some models', async () => {
    const calls = mockFetch({ ...house, '/api/assistants': [], '/api/capabilities': [], 'PUT /api/assistants/escritor': (b: unknown) => b })
    wrap(<Assistants />)
    await userEvent.click(await screen.findByRole('button', { name: /Novo assistente/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Escritor')
    await userEvent.click(within(dialog).getByRole('checkbox', { name: 'Todas as ferramentas' }))
    const models = within(dialog).getByRole('group', { name: 'Modelos que ele pode usar' })
    await userEvent.click(within(models).getByRole('checkbox', { name: 'Todos os modelos da casa' }))
    await userEvent.click(within(models).getByRole('checkbox', { name: 'Claude Code · opus' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ id: 'escritor', capabilities: [], models: ['opus'] }))
  })
})
