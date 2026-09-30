import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Memory } from '../pages/Memory'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const chat = { kind: 'conversation', ref: 'c1', turn: 'e1', label: 'Planos de sexta' }
const mail = { kind: 'email', ref: '<m1@x>', sender: 'bob@x.com', label: 'Jantar' }
const fact = (id: string, text: string, origins: unknown[]) => ({ id, text, topic: 'geral', source: 'x', trust: 'low', created: new Date().toISOString(), origins })

describe('Where memory came from', () => {
  it('links each fact to its source and forgets a whole source after confirming', async () => {
    const a = fact('1', 'Jantar na sexta com o Bob', [chat, mail])
    const b = fact('2', 'Bob é vegetariano', [mail])
    const c = fact('3', 'Moro em Lisboa', [{ kind: 'typed' }])
    const calls = mockFetch({
      '/api/memory': { facts: [a, b, c], history: [] },
      '/api/people': [],
      '/api/memory/organized': { checked: 0, merged: [] },
      '/api/memory/sources': { sources: [
        { key: 'conversation:c1', origin: chat, facts: [a], topics: ['geral'] },
        { key: 'email:bob@x.com', origin: mail, facts: [a, b], topics: ['geral'] },
        { key: 'typed', origin: { kind: 'typed' }, facts: [c], topics: ['geral'] },
      ] },
      'POST /api/memory/sources/forget': { removed: [a, b] },
    })
    wrap(<Memory />)
    const link = await screen.findByRole('link', { name: 'Conversa: Planos de sexta' })
    expect(link).toHaveAttribute('href', '/chat/c1')
    expect(screen.getAllByText('E-mail de bob@x.com').length).toBe(2)
    expect(screen.getByText(/Você escreveu/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Origens/ }))
    const sources = await screen.findByRole('region', { name: 'Origens' })
    expect(within(sources).getByText('2 fatos · geral')).toBeInTheDocument()
    const buttons = within(sources).getAllByRole('button', { name: /Apagar tudo desta origem/ })
    await userEvent.click(buttons[1])
    const ask = screen.getByRole('alertdialog', { name: 'Isto apaga 2 fatos:' })
    expect(within(ask).getByText('Bob é vegetariano')).toBeInTheDocument()
    expect(within(ask).getByText('Jantar na sexta com o Bob')).toBeInTheDocument()
    expect(calls.some((x) => x.url === '/api/memory/sources/forget')).toBe(false)
    await userEvent.click(within(ask).getByRole('button', { name: 'Apagar 2 fatos' }))
    await waitFor(() => expect(calls.find((x) => x.method === 'POST' && x.url === '/api/memory/sources/forget')?.body).toEqual({ key: 'email:bob@x.com' }))
  })

  it('shows old facts without sources as before', async () => {
    mockFetch({ '/api/memory': { facts: [{ id: '1', text: 'Academia às terças', topic: 'geral', source: 'owner', trust: 'high', created: new Date().toISOString() }], history: [] }, '/api/people': [], '/api/memory/organized': { checked: 0, merged: [] } })
    wrap(<Memory />)
    expect(await screen.findByText('Academia às terças')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
