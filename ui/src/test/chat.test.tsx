import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Chat } from '../pages/Chat'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const turn = (extra = {}) => ({
  id: 'e1', request: 'Anota: pagar a conta de luz', state: 'ready', summary: 'Salvo a nota.', cost_usd: 0.02, created_at: new Date().toISOString(),
  actions: [{ capability: 'note.save', text: 'Salvar a nota "pagar a conta de luz"', risk: 'reversible', args: {} }], ...extra,
})

describe('Chat', () => {
  it('starts a conversation from a suggestion', async () => {
    const calls = mockFetch({ '/api/chats': [], 'POST /api/chats': { chat: 'c1', turn: 'e1' }, '/api/chats/c1': { chat: { id: 'c1', title: 'x' }, turns: [turn({ state: 'running', actions: [] })] } })
    wrap(<Routes><Route path="/" element={<Chat />} /><Route path="/chat/:id" element={<Chat />} /></Routes>)
    await userEvent.click(await screen.findByRole('button', { name: 'O que eu tenho amanhã na agenda?' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'O que eu tenho amanhã na agenda?', assistant: '', model: 'auto', effort: 'auto' }))
    expect(await screen.findByText('Trabalhando…')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Ver passo a passo' })).toHaveAttribute('href', '/explorations/e1')
  })

  it('searches the conversations', async () => {
    const calls = mockFetch({
      '/api/chats': [{ id: 'c1', title: 'Conta de luz', updated_at: new Date().toISOString(), turns: 2 }],
      '/api/chats/search?q=conta%20de%20luz': [{ chat: 'c1', title: 'Conta de luz', turn: 'e1', snippet: 'Anota: pagar a conta de luz', at: new Date().toISOString() }],
      '/api/chats/search?q=nada': [],
    })
    wrap(<Routes><Route path="/" element={<Chat />} /></Routes>)
    const box = await screen.findByRole('searchbox', { name: 'Buscar nas conversas' })
    await userEvent.type(box, 'conta de luz')
    const results = await screen.findByRole('list', { name: 'Resultados da busca' })
    expect(results).toHaveTextContent('Anota: pagar a conta de luz')
    expect(screen.getAllByRole('link', { name: /Anota: pagar a conta de luz/ })[0]).toHaveAttribute('href', '/chat/c1')
    await userEvent.clear(box)
    await userEvent.type(box, 'nada')
    expect(await screen.findByText('Nada encontrado nas suas conversas.')).toBeInTheDocument()
    expect(calls.some((c) => c.url === '/api/chats/search?q=conta%20de%20luz')).toBe(true)
  })

  it('shows what it would do and confirms it', async () => {
    let done = false
    const calls = mockFetch({
      '/api/chats': [{ id: 'c1', title: 'Anota: pagar a conta de luz', updated_at: new Date().toISOString(), turns: 1 }],
      '/api/chats/c1': () => ({ chat: { id: 'c1', title: 'x' }, turns: [turn(done ? { done: { state: 'done', at: '', results: [{ capability: 'note.save', ok: true }] } } : {})] }),
      'POST /api/chats/c1/turns/e1/do': () => { done = true; return turn() },
      'POST /api/chats/c1/messages': { chat: 'c1', turn: 'e2' },
    })
    wrap(<Routes><Route path="/chat/:id" element={<Chat />} /></Routes>, '/chat/c1')
    expect(await screen.findByText('Salvo a nota.')).toBeInTheDocument()
    expect(screen.getByText('Salvar a nota "pagar a conta de luz"')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Confirmar e fazer' }))
    expect(await screen.findByText('Feito.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Confirmar e fazer' })).not.toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Escreva uma mensagem…'), 'e a de água{Enter}')
    await waitFor(() => expect(calls.find((c) => c.url === '/api/chats/c1/messages')?.body).toEqual({ text: 'e a de água', model: '', effort: '' }))
  })

  it('picks the model for the conversation and says which one answered', async () => {
    const calls = mockFetch({
      '/api/chats': [],
      '/api/chats/c1': { chat: { id: 'c1', title: 'x' }, model: 'auto', turns: [turn({ actions: [], model: { model: 'haiku', tier: 'simple', by: 'jev', effort: 'low', effort_by: 'auto' } })] },
      '/api/models': { keys: {}, claude_code: true, providers: [], auto: { light: 'haiku', strong: 'opus', base: 'sonnet', weigher: 'jev' } },
      '/api/models/detect': { claude_code: '2.0' },
      '/api/settings': { explore_model: 'sonnet', compile_model: 'sonnet', judge_model: 'haiku', models: [] },
      'POST /api/chats/c1/messages': { chat: 'c1', turn: 'e2' },
    })
    wrap(<Routes><Route path="/chat/:id" element={<Chat />} /></Routes>, '/chat/c1')
    expect(await screen.findByText('· Claude Code · haiku · automático: pedido simples · raciocínio baixo')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Modelo da conversa' }))
    expect(await screen.findByText('Pedidos simples: Claude Code · haiku. Difíceis: Claude Code · opus.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('menuitemradio', { name: 'Claude Code · opus' }))
    await userEvent.click(screen.getByRole('button', { name: 'Modelo da conversa' }))
    await userEvent.click(await screen.findByRole('menuitemradio', { name: /^alto/ }))
    expect(screen.getByRole('button', { name: 'Modelo da conversa' })).toHaveTextContent('Claude Code · opus · alto')
    await userEvent.type(screen.getByLabelText('Escreva uma mensagem…'), 'planeje minha semana{Enter}')
    await waitFor(() => expect(calls.find((c) => c.url === '/api/chats/c1/messages')?.body).toEqual({ text: 'planeje minha semana', model: 'opus', effort: 'high' }))
  })

})
