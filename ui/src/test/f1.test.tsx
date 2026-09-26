import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe as describeAction } from '../lib/actions'
import { cronText, usd } from '../lib/format'
import { RoutineCard } from '../components/RoutineCard'
import { NewTask } from '../components/NewTask'
import { Connections } from '../pages/Connections'

function wrap(ui: ReactNode, path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/" element={ui} />
          <Route path="/explorations/:id" element={<div>exploration page</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockFetch(routes: Record<string, unknown>) {
  const calls: { url: string; method: string; body?: unknown }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, method: init?.method ?? 'GET', body: init?.body ? JSON.parse(String(init.body)) : undefined })
    const key = `${init?.method ?? 'GET'} ${url}`
    const body = routes[key] ?? routes[url] ?? null
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  return calls
}

afterEach(() => vi.unstubAllGlobals())

describe('formatting', () => {
  it('describes schedules in words', () => {
    expect(cronText('0 7 * * *')).toBe('todo dia às 07:00')
    expect(cronText('30 18 * * 1-5')).toBe('dias úteis às 18:30')
    expect(cronText('*/15 * * * *')).toBe('a cada 15 min')
    expect(cronText('0 9 * * 1')).toBe('toda segunda às 09:00')
    expect(cronText('weird')).toBe('weird')
    expect(usd(0.0312)).toBe('$0.031')
    expect(usd(1.5)).toBe('$1.50')
  })

  it('describes actions as a person would', () => {
    expect(describeAction({ source: '', capability: 'gmail.search', risk: 'read', args: { query: 'is:unread' }, result: [1, 2], verdict: 'allow', ms: 1 })).toBe('Buscou e-mails “is:unread” · 2 encontrados')
    expect(describeAction({ source: '', capability: 'gmail.archive', risk: 'reversible', args: { id: 'x' }, dry_run: true, verdict: 'allow', ms: 1 })).toBe('Arquivaria um e-mail')
    expect(describeAction({ source: '', capability: 'telegram.send', risk: 'notify', args: { text: 'Bom dia!' }, verdict: 'allow', ms: 1 })).toBe('Te mandou: “Bom dia!”')
  })
})

describe('RoutineCard', () => {
  it('shows what the routine may touch, its health and cost', async () => {
    const open = vi.fn()
    render(
      <RoutineCard
        onOpen={open}
        r={{ id: 'b', name: 'Resumo matinal', description: 'Agenda e e-mails', state: 'broken', version: 2, schedule: '0 7 * * *', runs: ['ok', 'failed'], cost_month_usd: 0.004, capabilities: ['gmail.search', 'gmail.archive', 'telegram.send'] }}
      />,
    )
    expect(screen.getByText('Precisa de atenção')).toBeInTheDocument()
    expect(screen.getByText('Arquiva e-mails')).toBeInTheDocument()
    expect(screen.getByLabelText('1 de 2 execuções bem-sucedidas')).toBeInTheDocument()
    expect(screen.getByText('$0.004')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Rotina Resumo matinal' }))
    expect(open).toHaveBeenCalled()
  })
})

describe('NewTask', () => {
  it('starts an exploration and opens it', async () => {
    const calls = mockFetch({ 'POST /api/explorations': { id: 'e42' } })
    wrap(<NewTask open onOpenChange={() => {}} />)
    await userEvent.click(screen.getByText(/contas que vencem/))
    await userEvent.click(screen.getByRole('button', { name: /Fazer agora/ }))
    await waitFor(() => expect(screen.getByText('exploration page')).toBeInTheDocument())
    expect(calls[0].body).toEqual({ request: expect.stringContaining('contas que vencem') })
  })
})

describe('Connections', () => {
  it('shows the pairing command once the bot token is saved', async () => {
    mockFetch({
      '/api/connections': [
        { kind: 'telegram', configured: true, paired: false, pairing_code: '482913', bot: 'meu_pimpo_bot' },
        { kind: 'mail', configured: false },
        { kind: 'calendar', configured: true, detail: '2 agendas' },
        { kind: 'jev', configured: false },
        { kind: 'claude', configured: true },
      ],
    })
    wrap(<Connections />)
    expect(await screen.findByText('/start 482913')).toBeInTheDocument()
    expect(screen.getByText('2 agendas')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@meu_pimpo_bot' })).toHaveAttribute('href', 'https://t.me/meu_pimpo_bot?start=482913')
  })
})
