import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Memory } from '../pages/Memory'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const fact = (id: string, text: string) => ({ id, text, topic: 'geral', source: 'owner', trust: 'high', created: new Date().toISOString() })

describe('Memory search and organizing', () => {
  it('searches by meaning and merges repeated facts', async () => {
    const calls = mockFetch({
      '/api/memory': { facts: [fact('1', 'Tenho alergia a amendoim'), fact('2', 'Alergia a amendoim')], history: [] },
      '/api/people': [],
      '/api/memory/organized': { checked: 0, merged: [] },
      '/api/memory/search?q=o%20que%20n%C3%A3o%20posso%20comer%3F': { facts: [{ ...fact('1', 'Tenho alergia a amendoim'), by: 'meaning', score: 0.99 }], meaning: true },
      'POST /api/memory/organize': { at: new Date().toISOString(), checked: 1, merged: [{ kept: 'Tenho alergia a amendoim', dropped: 'Alergia a amendoim' }] },
    })
    wrap(<Memory />)
    await userEvent.type(await screen.findByLabelText('Buscar na memória'), 'o que não posso comer?')
    expect(await screen.findByText('pelo sentido')).toBeInTheDocument()
    expect(screen.queryByText('Alergia a amendoim')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Organizar/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/memory/organize')).toBe(true))
    expect(await screen.findByText(/1 fato repetido juntado/)).toBeInTheDocument()
    expect(screen.getByText('“Alergia a amendoim” → “Tenho alergia a amendoim”')).toBeInTheDocument()
  })
  it('opens before memory was ever organized', async () => {
    mockFetch({ '/api/memory': { facts: [fact('1', 'Academia às terças')], history: [] }, '/api/people': [], '/api/memory/organized': { checked: 0, merged: null } })
    wrap(<Memory />)
    expect(await screen.findByText('Academia às terças')).toBeInTheDocument()
    expect(screen.queryByText(/Organizada/)).not.toBeInTheDocument()
  })
})
