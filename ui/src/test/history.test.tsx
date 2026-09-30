import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SettingsHistory } from '../components/SettingsHistory'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const ts = new Date(Date.now() - 5 * 60000).toISOString()
const budget = { id: 7, ts, actor: 'human:owner', who: 'Dener', area: 'budget', fields: [{ field: 'budget.daily_usd', before: '1.00', after: '5.00' }], undoable: true, undone: false, reenter: [] }
const key = { id: 6, ts, actor: 'human:owner', who: 'Dener', area: 'models', target: 'openai', fields: [{ field: 'vault:model.openai.key', secret: 'replaced' }], undoable: false, undone: false, reenter: ['model.openai.key'] }

describe('SettingsHistory', () => {
  it('lists changes with who and when, hides secrets, and filters by area', async () => {
    const calls = mockFetch({ '/api/history?area=': [budget, key], '/api/history?area=budget': [budget] })
    wrap(<SettingsHistory />)
    expect(await screen.findByText('1.00')).toBeInTheDocument()
    expect(screen.getByText('5.00')).toBeInTheDocument()
    expect(screen.getAllByText(/por Dener/)).toHaveLength(2)
    expect(screen.getByText('trocado (oculto)')).toBeInTheDocument()
    expect(screen.getByText('Um segredo: digite de novo para voltar atrás.')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('sk-')
    await userEvent.click(screen.getByRole('button', { name: 'Orçamento' }))
    expect(screen.getByRole('button', { name: 'Orçamento' })).toHaveAttribute('aria-pressed', 'true')
    await waitFor(() => expect(calls.some((c) => c.url === '/api/history?area=budget')).toBe(true))
  })

  it('asks before undoing, then undoes', async () => {
    const calls = mockFetch({ '/api/history?area=': [budget], 'POST /api/history/7/undo': { state: 'undone', reenter: [] } })
    wrap(<SettingsHistory />)
    await userEvent.click(await screen.findByRole('button', { name: /Desfazer/ }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
    expect(screen.getByRole('group', { name: /Desfazer esta mudança/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Sim, desfazer' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/history/7/undo')).toBe(true))
  })

  it('says why an undo was refused', async () => {
    mockFetch({ '/api/history?area=': [budget] })
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init?: RequestInit) => init?.method === 'POST'
      ? new Response(JSON.stringify({ error: 'changed', problem: 'changed_since' }), { status: 409 })
      : new Response(JSON.stringify([budget]), { status: 200 })))
    wrap(<SettingsHistory />)
    await userEvent.click(await screen.findByRole('button', { name: /Desfazer/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Sim, desfazer' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('desfaça primeiro a mudança mais recente')
  })

  it('shows a member only their own, without area filters', async () => {
    mockFetch({ '/api/history?area=': [] })
    wrap(<SettingsHistory mine />)
    expect(await screen.findByText('Nenhuma mudança ainda.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Orçamento' })).not.toBeInTheDocument()
  })
})
