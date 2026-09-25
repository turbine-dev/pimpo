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
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'O que eu tenho amanhã na agenda?' }))
    expect(await screen.findByText('Trabalhando…')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Ver passo a passo' })).toHaveAttribute('href', '/explorations/e1')
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
    await waitFor(() => expect(calls.find((c) => c.url === '/api/chats/c1/messages')?.body).toEqual({ text: 'e a de água' }))
  })
})
