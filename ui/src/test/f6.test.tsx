import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { People } from '../pages/People'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const owner = { id: 'owner', name: 'Você', role: 'owner', chat: 1, created: '', mail: true, calendar: true }

describe('People', () => {
  it('invites a guest answered for by the owner and shows the one-time code', async () => {
    const calls = mockFetch({
      'POST /api/people': { id: 'leo', name: 'Léo', role: 'guest', responsible: 'owner', invite: '12345678', created: '' },
      '/api/people': [owner],
    })
    wrap(<People />)
    expect(await screen.findByText('Só você por enquanto')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Convidar alguém' }))
    await userEvent.type(screen.getByLabelText('Nome'), 'Léo')
    await userEvent.click(screen.getByRole('radio', { name: /Convidado/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Criar convite' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Léo', role: 'guest', responsible: 'owner' }))
    expect(await screen.findByText(/\/start 12345678/)).toBeInTheDocument()
  })

  it('lists members with who answers for them and a pending invite', async () => {
    mockFetch({ '/api/people': [owner, { id: 'ana', name: 'Ana', role: 'member', responsible: 'owner', invite: '87654321', created: '', mail: false, calendar: false }] })
    wrap(<People />)
    expect(await screen.findByText('Ana')).toBeInTheDocument()
    expect(screen.getByText(/Membro · aprova: você/)).toBeInTheDocument()
    expect(screen.getByText('/start 87654321')).toBeInTheDocument()
  })
})
