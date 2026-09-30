import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Inbox } from '../pages/Inbox'
import { wrap, type Call } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const empty = { '/api/explorations?state=ready': [], '/api/routines': [], '/api/approvals': [], '/api/media': [], '/api/suggestions': [] }

// stub answers like mockFetch, but a typed answer that is none of the
// options gets the 422 the server sends.
function stub() {
  const calls: Call[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ url, method, body })
    let out: unknown = (empty as Record<string, unknown>)[url] ?? null
    let status = 200
    if (url === '/api/questions') out = [{ id: 'q1', question: 'Treinou hoje?', options: ['Sim', 'Não'], asked: new Date().toISOString() }]
    if (method === 'POST' && body?.text === 'talvez') {
      status = 422
      out = { error: '“Treinou hoje?”: essa não é uma das opções. Responda com uma delas: 1 = Sim · 2 = Não' }
    } else if (method === 'POST') out = { text: '✅ Anotado' }
    return new Response(JSON.stringify(out), { status, headers: { 'Content-Type': 'application/json' } })
  }))
  return calls
}

describe('Questions as cards', () => {
  it('offers the options as buttons and checks a typed answer', async () => {
    const calls = stub()
    wrap(<Inbox />)
    const options = await screen.findByRole('group', { name: 'Treinou hoje?' })
    await userEvent.click(within(options).getByRole('button', { name: 'Não' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/questions/q1/answer' && (c.body as { index: number }).index === 1)).toBe(true))

    const field = screen.getByRole('textbox', { name: 'Ou digite sua resposta' })
    const send = screen.getByRole('button', { name: 'Responder' })
    expect(send).toBeDisabled()
    await userEvent.type(field, 'talvez')
    await userEvent.click(send)
    expect(await screen.findByRole('alert')).toHaveTextContent('1 = Sim · 2 = Não')

    await userEvent.clear(field)
    await userEvent.type(field, 'sim{Enter}')
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && (c.body as { text?: string }).text === 'sim')).toBe(true))
  })
})
