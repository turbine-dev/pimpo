import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Inbox } from '../pages/Inbox'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Suggestions in the inbox', () => {
  it('shows what Pimpo would do and starts it only when accepted', async () => {
    const calls = mockFetch({
      '/api/explorations?state=ready': [], '/api/routines': [], '/api/approvals': [], '/api/media': [], '/api/questions': [],
      '/api/suggestions': [{ id: 's1', title: 'Conta de luz', why: 'Chega todo mês da Enel.', request: 'Todo mês, me avise o valor da conta da Enel.', made: '2026-09-29T09:00:00Z' }],
      'POST /api/suggestions/s1/dismiss': { state: 'dismissed' },
      'POST /api/suggestions/s1/accept': { exploration: 'e1' },
    })
    wrap(<Inbox />)
    expect(await screen.findByText('Conta de luz')).toBeInTheDocument()
    expect(screen.getByText(/Eu faria: “Todo mês, me avise o valor da conta da Enel.”/)).toBeInTheDocument()
    expect(calls.some((c) => c.url.includes('/accept'))).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Não, obrigado' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/suggestions/s1/dismiss' && c.method === 'POST')).toBe(true))
    await userEvent.click(screen.getByRole('button', { name: 'Sim, aprenda' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/suggestions/s1/accept')).toBe(true))
  })
})
