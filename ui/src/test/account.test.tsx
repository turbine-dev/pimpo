import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Account } from '../pages/Account'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Account', () => {
  it('lists only what opens this account and signs one out', async () => {
    const calls = mockFetch({
      '/api/passkeys': [],
      '/api/me/devices': [
        { id: 'a1', name: 'Celular da Ana', created: new Date().toISOString(), last_seen: new Date().toISOString(), current: true },
        { id: 'a2', name: 'Tablet', created: new Date().toISOString(), pending: true },
      ],
      'DELETE /api/me/devices/a2': { revoked: 'a2' },
    })
    wrap(<Account />)
    expect(await screen.findByText('Celular da Ana')).toBeInTheDocument()
    expect(screen.getByText('este aparelho')).toBeInTheDocument()
    expect(screen.getByText('convite ainda não aberto')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Encerrar Tablet' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/me/devices/a2')).toBe(true))
  })
})
