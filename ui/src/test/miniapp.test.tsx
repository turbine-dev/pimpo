import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { setBearer } from '../lib/api'
import { LocaleProvider } from '../lib/i18n'
import { MiniApp } from '../pages/MiniApp'
import { wrap } from './helpers'

type Seen = { url: string; method: string; auth?: string; body?: unknown }

// serve answers like mockFetch, but keeps each request's Authorization
// and can answer with an error status.
function serve(routes: Record<string, unknown>, status: Record<string, number> = {}) {
  const seen: Seen[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const headers = (init?.headers ?? {}) as Record<string, string>
    seen.push({ url, method, auth: headers.Authorization, body: init?.body ? JSON.parse(String(init.body)) : undefined })
    const key = `${method} ${url}`
    const out = routes[key] ?? routes[url] ?? null
    return new Response(JSON.stringify(out), { status: status[key] ?? 200, headers: { 'Content-Type': 'application/json' } })
  }))
  return seen
}

const widget = { id: 'w1', source: 'routine', kind: 'metric', title: 'Dólar hoje', snapshot: { kind: 'metric', title: 'Dólar hoje', value: 5.1 }, updated: new Date().toISOString(), mine: true }
const house = (role: 'member' | 'guest') => ({
  'POST /api/tg/session': { token: 'mini-token', expires: new Date(Date.now() + 3600e3).toISOString(), person: 'ana', role, name: 'Ana' },
  '/api/questions': [{ id: 'q1', question: 'Qual mercado?', options: ['Perto', 'Barato'], asked: '' }],
  '/api/approvals': [{ id: 'a1', action: { capability: 'gmail.send', args: {}, risk: 3, source: 'routine:r1' }, text: 'Enviar e-mail ao síndico', reason: 'envia para fora', created: '' }],
  '/api/routines': [{ id: 'r1', name: 'Resumo do dia', description: '', state: 'active', version: 1, runs: ['ok'], cost_month_usd: 0.25, capabilities: [], schedule: '' }],
  '/api/state': { budget: { spent: 0.42, limit: 2 }, healthy: true, broken: 0, awaiting: 0, telegram_paired: true, log_intact: true, claude: false },
  '/api/cost': { today: 0.42, limit: 2, month: 3.1, projected_month: 5, by_day: {}, by_source: {} },
  '/api/dashboards': [{ id: 'd1', name: 'Casa', emoji: '🏠', position: 0, layout: [{ id: 'w1', x: 0, y: 0, w: 4, h: 3 }], shared: false, mine: true, updated: '' }],
  '/api/widgets': [widget],
  '/api/dashboards/d1/widgets': { w1: widget },
})

const open = (initData?: string) => wrap(<LocaleProvider locale="pt"><MiniApp initData={initData} /></LocaleProvider>)

describe('telegram mini app', () => {
  afterEach(() => setBearer(''))

  it('signs in with initData and shows each tab with the session in a header', async () => {
    const seen = serve(house('member'))
    open('query_id=x&user=%7B%22id%22%3A1%7D&hash=abc')
    expect(await screen.findByText('Oi, Ana')).toBeInTheDocument()
    expect(seen[0]).toMatchObject({ method: 'POST', url: '/api/tg/session', body: { init_data: 'query_id=x&user=%7B%22id%22%3A1%7D&hash=abc' } })
    expect(seen[0].auth).toBeUndefined()

    const tabs = screen.getAllByRole('tab').map((t) => t.textContent)
    expect(tabs).toEqual(['Precisa de você', 'Rotinas', 'Gastos', 'Widgets'])
    expect(await screen.findByText('Qual mercado?')).toBeInTheDocument()
    expect(await screen.findByText('Enviar e-mail ao síndico')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Permitir' }))
    await userEvent.click(screen.getByRole('button', { name: 'Barato' }))
    await waitFor(() => expect(seen.some((s) => s.method === 'POST' && s.url === '/api/approvals/a1/once')).toBe(true))
    await waitFor(() => expect(seen.find((s) => s.url === '/api/questions/q1/answer')?.body).toEqual({ index: 1 }))

    await userEvent.click(screen.getByRole('tab', { name: /Rotinas/ }))
    await userEvent.click(await screen.findByRole('button', { name: 'Rodar agora: Resumo do dia' }))
    await userEvent.click(screen.getByRole('button', { name: 'Pausar: Resumo do dia' }))
    await waitFor(() => expect(seen.some((s) => s.url === '/api/routines/r1/pause')).toBe(true))
    expect(seen.some((s) => s.url === '/api/routines/r1/run')).toBe(true)

    await userEvent.click(screen.getByRole('tab', { name: /Gastos/ }))
    expect(await screen.findByText('$0.42')).toBeInTheDocument()
    expect(screen.getByText('do limite diário da casa de $2.00')).toBeInTheDocument()
    expect(await screen.findByText('$3.10')).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '21')

    await userEvent.click(screen.getByRole('tab', { name: /Widgets/ }))
    expect(await screen.findByText('Dólar hoje')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '🏠 Casa' }))
    await waitFor(() => expect(seen.some((s) => s.url === '/api/dashboards/d1/widgets')).toBe(true))

    // Every call after the exchange carries the Mini App's own session.
    const after = seen.slice(1)
    expect(after.length).toBeGreaterThan(5)
    expect(after.every((s) => s.auth === 'Bearer mini-token')).toBe(true)
  })

  it('shows a guest only what a guest may use', async () => {
    const seen = serve(house('guest'))
    open('signed')
    await screen.findByText('Oi, Ana')
    expect(screen.getAllByRole('tab').map((t) => t.textContent)).toEqual(['Precisa de você', 'Gastos', 'Widgets'])
    await screen.findByText('Qual mercado?')
    expect(seen.some((s) => s.url === '/api/approvals')).toBe(false)
  })

  it('opens nothing for an unpaired account or outside Telegram', async () => {
    serve({ 'POST /api/tg/session': { error: 'not paired' } }, { 'POST /api/tg/session': 403 })
    open('signed')
    expect(await screen.findByRole('alert')).toHaveTextContent('não está pareada')
    expect(screen.queryByRole('tab')).toBeNull()
    const seen = serve({})
    open('')
    expect(await screen.findByText('Abra esta página pelo bot do Pimpo no Telegram.')).toBeInTheDocument()
    expect(seen).toHaveLength(0)
  })

  it('says when the session ran out', async () => {
    serve(house('member'), { 'GET /api/questions': 401, 'GET /api/approvals': 401 })
    open('signed')
    expect(await screen.findByText(/Esta sessão terminou/)).toBeInTheDocument()
  })
})
