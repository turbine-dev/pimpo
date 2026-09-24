import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Gallery } from '../pages/Gallery'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const routine = (name: string, caps: string[]) => ({ name, description: name + ' todo dia', manifest: { schedule: '0 7 * * *', capabilities: caps }, code: 'async function run() {}', tests: [{ name: 't' }] })
const items = [
  { id: 'agenda', author: 'dener', author_name: 'Dener', hash: 'a'.repeat(64), published: '', installed: false, routine: routine('Agenda do dia', ['calendar.events', 'telegram.send']), report: { verified: true, uses: ['calendar.events', 'telegram.send'], sends: false, risk: 'notify' } },
  { id: 'cobra', author: 'x', author_name: 'X', hash: 'b'.repeat(64), published: '', installed: false, routine: routine('Cobrar clientes', ['gmail.search', 'gmail.send']), report: { verified: false, problems: ['calls gmail.delete without declaring it'], uses: [], sends: true, risk: 'irreversible' } },
]

describe('Gallery', () => {
  it('filters by what routines can touch and refuses unverified ones', async () => {
    const calls = mockFetch({ '/api/gallery': items, 'POST /api/gallery/agenda/install': { id: 'agenda' } })
    wrap(<Gallery />)
    expect(await screen.findByText('Agenda do dia')).toBeInTheDocument()
    expect(screen.getByText('Cobrar clientes')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'Não enviam nada para fora' }))
    expect(screen.queryByText('Cobrar clientes')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'Todas' }))

    await userEvent.click(screen.getByText('Cobrar clientes'))
    expect(await screen.findByText('calls gmail.delete without declaring it')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Instalar esta rotina' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Fechar' }))

    await userEvent.click(screen.getByText('Agenda do dia'))
    expect(await screen.findByText(/Assinatura confere/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Instalar esta rotina' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/gallery/agenda/install')).toBe(true))
  })
})
