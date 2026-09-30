import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { ProgressCard, RunningNow } from '../components/ProgressCard'
import type { Progress } from '../lib/api'
import { useLiveEvents } from '../lib/live'
import { Jobs } from '../pages/Jobs'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const now = new Date().toISOString()
const job: Progress = { id: 'job:j1', person: 'owner', kind: 'job', job: 'j1', title: 'Comparar fornecedores', label: 'Fornecedor B', done: 1, total: 2, state: 'running', cost_usd: 0.42, started_at: now, updated_at: now }
const run: Progress = { id: 'run:brief:3', person: 'owner', kind: 'run', routine: 'brief', run: 3, title: 'Resumo matinal', label: 'gmail.search', done: 4, total: 0, state: 'running', cost_usd: 0, started_at: now, updated_at: now }

// A socket the test speaks through, standing in for /api/ws.
class FakeSocket {
  static last?: FakeSocket
  onopen?: () => void
  onclose?: () => void
  onmessage?: (m: { data: string }) => void
  constructor() { FakeSocket.last = this }
  close() {}
  send(data: string) { this.onmessage?.({ data }) }
}

function Live() {
  useLiveEvents()
  return <RunningNow />
}

describe('ProgressCard', () => {
  it('shows parts, the step, the cost and a labelled bar', () => {
    wrap(<ProgressCard p={{ ...job, resumed: true }} />)
    expect(screen.getByRole('link', { name: 'Comparar fornecedores' })).toHaveAttribute('href', '/jobs/j1')
    expect(screen.getByText('· 1 de 2 partes')).toBeInTheDocument()
    expect(screen.getByText('· Fornecedor B')).toBeInTheDocument()
    expect(screen.getByText('· $0.42')).toBeInTheDocument()
    expect(screen.getByText('Retomado depois de um reinício')).toBeInTheDocument()
    const bar = screen.getByRole('progressbar', { name: 'Andamento de Comparar fornecedores' })
    expect(bar).toHaveAttribute('aria-valuenow', '50')
  })

  it('reads a routine run by its steps, without a total', () => {
    wrap(<ProgressCard p={run} />)
    expect(screen.getByText('· Passo 4')).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow')
  })
})

describe('Running now', () => {
  it('shows what the server kept after a reload and follows the stream', async () => {
    vi.stubGlobal('WebSocket', FakeSocket)
    mockFetch({ '/api/progress': [job, { ...run, state: 'done' }] })
    wrap(<Live />)
    expect(await screen.findByText('Comparar fornecedores')).toBeInTheDocument()
    expect(screen.queryByText('Resumo matinal')).not.toBeInTheDocument()
    // Someone else's progress comes bare and changes nothing.
    act(() => FakeSocket.last!.send(JSON.stringify({ id: 0, type: 'progress.updated', ts: now, actor: 'system', data: {} })))
    act(() => FakeSocket.last!.send(JSON.stringify({ id: 0, type: 'progress.updated', ts: now, actor: 'system', data: { ...job, done: 2, label: 'Fornecedor C' } })))
    expect(await screen.findByText('· 2 de 2 partes')).toBeInTheDocument()
    expect(screen.getByText('· Fornecedor C')).toBeInTheDocument()
  })
})

describe('Following a job', () => {
  it('starts a job followed on the channels, and stops following it', async () => {
    let follow = false
    const calls = mockFetch({
      '/api/jobs/j1': () => ({ id: 'j1', request: 'Comparar fornecedores', state: follow ? 'running' : 'planned', follow, budget_usd: 2, spent_usd: 0.1, parts: [], created: now, updated: now }),
      '/api/progress': [],
      'POST /api/jobs/j1/start': (b: unknown) => { follow = (b as { follow: boolean }).follow; return {} },
      'POST /api/jobs/j1/follow': (b: unknown) => { follow = (b as { follow: boolean }).follow; return {} },
    })
    wrap(<Routes><Route path="/jobs/:id" element={<Jobs />} /></Routes>, '/jobs/j1')
    const toggle = await screen.findByRole('switch', { name: 'Acompanhar nos meus canais' })
    await userEvent.click(toggle)
    await userEvent.click(screen.getByRole('button', { name: /Começar/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url === '/api/jobs/j1/start')?.body).toEqual({ follow: true }))
    await waitFor(() => expect(screen.getByRole('switch', { name: 'Acompanhar nos meus canais' })).toHaveAttribute('aria-checked', 'true'))
    await userEvent.click(screen.getByRole('switch', { name: 'Acompanhar nos meus canais' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/jobs/j1/follow')?.body).toEqual({ follow: false }))
  })
})
