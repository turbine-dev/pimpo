import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Inbox } from '../pages/Inbox'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const now = new Date()
const soon = new Date(now.getTime() + 4 * 60 * 1000).toISOString()

const needs = {
  total: 6,
  counts: { approval: 1, question: 1, failed_routine: 1, job_error: 1, exploration_ready: 1, lesson: 1 },
  items: [
    { kind: 'approval', id: 'a1', title: 'cobranca quer enviar um e-mail', detail: 'Sempre me pergunte', created: now.toISOString(), expires: soon, urgency: 4, actions: ['once', 'run', 'deny'], risk: 3 },
    { kind: 'question', id: 'q1', title: 'Treinou hoje?', created: now.toISOString(), urgency: 2, actions: ['answer'], options: ['Sim', 'Não'], link: '/routines/treino' },
    { kind: 'failed_routine', id: 'news', title: 'Notícias', detail: 'the site is down', created: now.toISOString(), urgency: 1, actions: ['run', 'repair', 'open'], link: '/routines/news' },
    { kind: 'job_error', id: 'j1', title: 'Resumo do mês', detail: 'Extratos: no access', created: now.toISOString(), urgency: 1, actions: ['open'], link: '/jobs/j1' },
    { kind: 'exploration_ready', id: 'e1', title: 'Resumo semanal', created: now.toISOString(), urgency: 0, actions: ['open'], link: '/explorations/e1' },
    // A kind this UI does not know yet still shows, with a way to open it.
    { kind: 'lesson', id: 'l1', title: 'Você prefere resumos curtos', urgency: 0, actions: ['open'], link: '/lessons' },
  ],
}

function page() {
  return wrap(
    <Routes>
      <Route path="/" element={<Inbox />} />
      <Route path="/jobs/:id" element={<p>trabalho aberto</p>} />
      <Route path="/explorations/:id" element={<p>exploração aberta</p>} />
    </Routes>,
  )
}

describe('One list of what needs you', () => {
  it('shows every kind in the order the server gives, with filters', async () => {
    mockFetch({ '/api/needs': needs, '/api/media': [] })
    page()
    const list = await screen.findByRole('list', { name: 'Precisa de você' })
    const rows = within(list).getAllByRole('listitem')
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining('cobranca quer enviar um e-mail'),
      expect.stringContaining('Treinou hoje?'),
      expect.stringContaining('Notícias não rodou'),
      expect.stringContaining('O trabalho “Resumo do mês” teve um problema'),
      expect.stringContaining('Resumo semanal'),
      expect.stringContaining('Você prefere resumos curtos'),
    ])
    // An approval about to expire says so; a member is not offered "always".
    expect(within(rows[0]).getByText(/^Expira em 4 minutos/)).toBeInTheDocument()
    expect(within(rows[0]).queryByRole('button', { name: 'Sempre' })).not.toBeInTheDocument()
    expect(within(rows[3]).getByText('Extratos: no access')).toBeInTheDocument()

    const tabs = screen.getByRole('tablist', { name: 'Filtrar por tipo' })
    expect(within(tabs).getByRole('tab', { name: /Todas/ })).toHaveAttribute('aria-selected', 'true')
    await userEvent.click(within(tabs).getByRole('tab', { name: /Perguntas/ }))
    expect(within(screen.getByRole('list', { name: 'Precisa de você' })).getAllByRole('listitem')).toHaveLength(1)
    expect(screen.getByText('Treinou hoje?')).toBeInTheDocument()
    expect(screen.queryByText('Notícias não rodou')).not.toBeInTheDocument()
    await userEvent.click(within(tabs).getByRole('tab', { name: /Rotinas paradas/ }))
    expect(screen.getByText('Notícias não rodou')).toBeInTheDocument()
    expect(screen.queryByText('Treinou hoje?')).not.toBeInTheDocument()
  })

  it('answers each kind in place', async () => {
    const calls = mockFetch({
      '/api/needs': needs, '/api/media': [],
      'POST /api/approvals/a1/deny': {},
      'POST /api/questions/q1/answer': { text: 'ok' },
      'POST /api/routines/news/run': { run: {} },
      'POST /api/routines/news/repair': { exploration: 'e9' },
    })
    page()
    const list = await screen.findByRole('list', { name: 'Precisa de você' })
    const row = (text: string) => within(list).getByText(text).closest('li')!
    const posted = (url: string, body?: unknown) => waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === url && (body === undefined || JSON.stringify(c.body) === JSON.stringify(body)))).toBe(true))

    await userEvent.click(within(row('cobranca quer enviar um e-mail')).getByRole('button', { name: 'Negar' }))
    await posted('/api/approvals/a1/deny')
    await userEvent.click(within(row('Treinou hoje?')).getByRole('button', { name: 'Não' }))
    await posted('/api/questions/q1/answer', { index: 1 })
    await userEvent.click(within(row('Notícias não rodou')).getByRole('button', { name: 'Rodar de novo' }))
    await posted('/api/routines/news/run')
    // Each answer refreshes the list.
    expect(calls.filter((c) => c.url === '/api/needs').length).toBeGreaterThan(1)
    await userEvent.click(within(row('Notícias não rodou')).getByRole('button', { name: 'Refazer com o agente' }))
    await posted('/api/routines/news/repair')
    expect(await screen.findByText('exploração aberta')).toBeInTheDocument()
  })

  it('opens a job from the list', async () => {
    mockFetch({ '/api/needs': needs, '/api/media': [] })
    page()
    const list = await screen.findByRole('list', { name: 'Precisa de você' })
    const job = within(list).getByText('O trabalho “Resumo do mês” teve um problema').closest('li')!
    await userEvent.click(within(job).getByRole('button', { name: 'Abrir' }))
    expect(await screen.findByText('trabalho aberto')).toBeInTheDocument()
  })

  it('says when nothing waits', async () => {
    mockFetch({ '/api/needs': { total: 0, counts: {}, items: [] }, '/api/media': [] })
    page()
    expect(await screen.findByText('Nada esperando por você')).toBeInTheDocument()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })
})
