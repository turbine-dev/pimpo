import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { RepoPanel } from '../components/RepoPanel'
import { LocaleProvider } from '../lib/i18n'
import { RoutinePage } from '../pages/RoutinePage'
import { Route, Routes } from 'react-router-dom'
import { mockFetch } from './helpers'

afterEach(() => vi.unstubAllGlobals())

function wrap(ui: React.ReactNode, path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}><LocaleProvider locale="pt"><MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter></LocaleProvider></QueryClientProvider>)
}

const view = {
  path: '/tmp/rotinas', git: true, remote: true, head: 'abc123', broken: { 'Pasta Ruim': 'folder names are lowercase letters, digits and dashes' },
  changes: [
    { id: 'bom-dia', name: 'Bom dia', new: false, added: ['gmail.search'], removed: [], tests: 2, problems: [], hash: 'h1' },
    { id: 'quebrada', name: 'Quebrada', new: true, added: [], removed: [], tests: 1, problems: ['test manda: telegram.send never mentions "x"'], hash: 'h2' },
  ],
}

describe('Repository', () => {
  it('shows what changed, what each routine gains, and blocks what fails', async () => {
    const calls = mockFetch({ '/api/repo': view, 'POST /api/repo/apply/bom-dia': { applied: true, view: { ...view, changes: [view.changes[1]] } } })
    wrap(<RepoPanel />)
    expect(await screen.findByText(/Repositório git com remoto · commit abc123/)).toBeInTheDocument()
    expect(screen.getByText(/Passa a poder:/)).toBeInTheDocument()
    expect(screen.getByText('Não passou nas verificações')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Atualizar|Instalar/ })).toHaveLength(1)
    await userEvent.click(screen.getByRole('button', { name: /Atualizar/ }))
    expect(calls.some((c) => c.method === 'POST' && c.url === '/api/repo/apply/bom-dia')).toBe(true)
  })
})

describe('Routine memory', () => {
  it('shows what the routine kept and clears it', async () => {
    const routine = { name: 'Preço', description: 'Avisa se o preço caiu', code: 'async function run() { state.set("p", 1) }', tests: [], manifest: { schedule: '0 9 * * *', capabilities: ['notify.send'], uses: ['agenda'] } }
    const summary = { id: 'preco', name: 'Preço', description: '', state: 'active', version: 1, runs: [], cost_month_usd: 0, capabilities: ['notify.send'], schedule: '0 9 * * *', default_schedule: '0 9 * * *', params: [], values: {} }
    const calls = mockFetch({ '/api/routines/preco': { summary, routine, versions: [], runs: [], state: { preco: 99.9 }, used_by: ['resumo'] }, 'POST /api/routines/preco/forget': {} })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    wrap(<Routes><Route path="/routines/:id" element={<RoutinePage />} /></Routes>, '/routines/preco')
    await userEvent.click(await screen.findByRole('tab', { name: 'Memória' }))
    expect(screen.getByText(/99.9/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Apagar a memória/ }))
    expect(calls.some((c) => c.method === 'POST' && c.url === '/api/routines/preco/forget')).toBe(true)
    await userEvent.click(screen.getByRole('tab', { name: 'O que ela pode fazer' }))
    expect(screen.getByRole('link', { name: 'agenda' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'resumo' })).toBeInTheDocument()
  })
})
