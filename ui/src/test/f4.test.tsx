import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Memory } from '../pages/Memory'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Memory', () => {
  it('separates what the owner said from what the agent noted, and confirms', async () => {
    const calls = mockFetch({
      '/api/memory': {
        facts: [
          { id: 'a', text: 'Minha chefe é a Ana', topic: 'trabalho', source: 'owner', trust: 'high', created: new Date().toISOString() },
          { id: 'b', text: 'A reunião de sexta foi cancelada', topic: 'trabalho', source: 'exploration:e1', trust: 'low', created: new Date().toISOString() },
        ],
        history: [],
      },
    })
    wrap(<Memory />)
    expect(await screen.findByText('Minha chefe é a Ana')).toBeInTheDocument()
    expect(screen.getByText('1 fato anotado pelo agente espera sua confirmação.')).toBeInTheDocument()
    expect(screen.getByText(/anotado pelo agente numa tarefa/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Confirmar/ }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/memory/b/confirm')).toBe(true))
    await userEvent.type(screen.getByLabelText('Novo fato'), 'Prefiro resumos curtos')
    await userEvent.click(screen.getByRole('button', { name: /Lembrar/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url === '/api/memory')?.body).toEqual({ text: 'Prefiro resumos curtos', topic: '', shared: false }))
  })
})
