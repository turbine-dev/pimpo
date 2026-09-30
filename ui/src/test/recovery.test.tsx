import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SnapshotsCard } from '../components/SnapshotsCard'
import { api, ApiError } from '../lib/api'
import { Recovery } from '../pages/Recovery'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const snapshots = [
  { name: '20260903-100000-daily', label: 'daily', when: '2026-09-03T10:00:00Z', bytes: 1e6, damaged: true },
  { name: '20260902-100000-daily', label: 'daily', when: '2026-09-02T10:00:00Z', bytes: 1e6 },
  { name: '20260901-100000-daily', label: 'daily', when: '2026-09-01T10:00:00Z', bytes: 1e6 },
]
const state = { when: '2026-09-04T08:00:00Z', reason: 'the database is damaged: database disk image is malformed', folder: 'quarantine/20260904-080000', snapshots, newest_good: '20260902-100000-daily' }

// answer stubs fetch with a status of its own.
function answer(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })))
}

describe('Database recovery', () => {
  it('knows when Pimpo is recovering', async () => {
    answer(503, { error: 'Pimpo is recovering its database', recovery: true })
    const err = await api.state().catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.recovery).toBe(true)
  })

  it('offers the newest good copy, never a damaged one, and puts it back', async () => {
    const calls = mockFetch({ '/api/recovery': state, 'POST /api/recovery/restore': { restored: '20260902-100000-daily' } })
    wrap(<Recovery />)
    expect(await screen.findByText(/quarantine\/20260904-080000/)).toBeInTheDocument()
    const main = screen.getByRole('button', { name: /Voltar à cópia de 2 de set/ })
    await userEvent.click(screen.getByText('Outras cópias'))
    const others = screen.getAllByRole('button', { name: /Voltar à cópia de/ })
    expect(others).toHaveLength(2)
    expect(others.some((b) => /3 de set/.test(b.textContent ?? ''))).toBe(false)
    await userEvent.click(main)
    expect(await screen.findByRole('status')).toHaveTextContent('O Pimpo está abrindo')
    expect(calls.find((c) => c.method === 'POST')).toMatchObject({ url: '/api/recovery/restore', body: { name: '20260902-100000-daily' } })
  })

  it('starts fresh only once confirmed, and lets the damaged file be taken away', async () => {
    const calls = mockFetch({ '/api/recovery': { ...state, snapshots: [snapshots[0]], newest_good: '' }, 'POST /api/recovery/fresh': { fresh: true } })
    wrap(<Recovery />)
    expect(await screen.findByText(/Nenhuma cópia passa na verificação/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Baixar o arquivo danificado' })).toHaveAttribute('href', '/api/recovery/export')
    vi.stubGlobal('confirm', () => false)
    await userEvent.click(screen.getByRole('button', { name: 'Começar do zero' }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
    vi.stubGlobal('confirm', () => true)
    await userEvent.click(screen.getByRole('button', { name: 'Começar do zero' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/recovery/fresh')).toBe(true))
  })

  it('asks for the administrator when not signed in', async () => {
    answer(401, { error: 'open the login link' })
    wrap(<Recovery />)
    expect(await screen.findByText(/Só o administrador pode recuperar o Pimpo/)).toBeInTheDocument()
  })

  it('marks a damaged local copy and does not offer it', async () => {
    mockFetch({ '/api/snapshots': { snapshots, available: true } })
    wrap(<SnapshotsCard />)
    const damaged = (await screen.findByText('Danificada: não dá para voltar a esta')).closest('li')!
    expect(within(damaged).getByRole('button')).toBeDisabled()
    expect(screen.getAllByRole('button', { name: /Voltar a esta/ }).filter((b) => !(b as HTMLButtonElement).disabled)).toHaveLength(2)
  })
})
