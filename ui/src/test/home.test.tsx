import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { InboxPanel } from '../components/InboxPanel'
import { SystemPanel } from '../components/SystemPanel'
import { Home } from '../pages/Home'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const now = new Date()
// Later today, even when the test runs close to midnight.
const later = new Date(Math.min(now.getTime() + 60 * 60 * 1000, new Date(now).setHours(23, 59, 59, 0)))

describe('Home', () => {
  it('shows the day and starts a chat', async () => {
    const calls = mockFetch({
      '/api/state': { budget: { spent: 0.4, limit: 2 }, healthy: true, broken: 1, awaiting: 0, approvals: 2, claude: true },
      '/api/routines': [{ id: 'brief', name: 'Resumo matinal', state: 'active', next_run: later.toISOString(), runs: [] }],
      '/api/runs?outcome=&before=0&limit=50': [{ id: 1, routine: 'news', name: 'Notícias', started_at: now.toISOString(), outcome: 'failed', cost_usd: 0, calls: 1, version: 1 }],
      '/api/chats': [{ id: 'c1', title: 'Agenda de amanhã', updated_at: now.toISOString(), turns: 2 }],
      'POST /api/chats': { chat: 'c2', turn: 'e2' },
    })
    wrap(<Routes><Route path="/" element={<Home />} /><Route path="/chat/:id" element={<p>conversa aberta</p>} /></Routes>)
    expect(await screen.findByText('2 aprovações · 1 rotina parada')).toBeInTheDocument()
    expect(screen.getByText('Notícias')).toBeInTheDocument()
    expect(screen.getByText('falhou')).toBeInTheDocument()
    expect(screen.getByText('Resumo matinal')).toBeInTheDocument()
    expect(screen.getByText('a seguir')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Agenda de amanhã/ })).toHaveAttribute('href', '/chat/c1')
    await userEvent.type(screen.getByLabelText('Escreva uma mensagem…'), 'o que tenho amanhã?{Enter}')
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'o que tenho amanhã?', assistant: '', model: 'auto', effort: 'auto' }))
    expect(await screen.findByText('conversa aberta')).toBeInTheDocument()
  })
})

describe('Inbox panel', () => {
  it('answers an approval in place and filters by tab', async () => {
    const calls = mockFetch({
      '/api/approvals': [{ id: 'a1', text: 'Enviar e-mail para a Ana', reason: 'irreversível', created: now.toISOString(), action: { capability: 'gmail.send', args: {}, risk: 3, source: 'x' } }],
      '/api/explorations?state=ready': [],
      '/api/routines': [{ id: 'news', name: 'Notícias', state: 'broken', runs: [] }],
      '/api/system': { components: [{ id: 'signal', group: 'channel', name: 'Signal', state: 'error', detail: 'signal-cli is not answering' }] },
      'POST /api/approvals/a1/once': {},
    })
    wrap(<InboxPanel onClose={() => {}} />)
    expect(await screen.findByText('Enviar e-mail para a Ana')).toBeInTheDocument()
    expect(await screen.findByText('Notícias não rodou')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /Sistema/ }))
    expect(screen.getByText('signal-cli is not answering')).toBeInTheDocument()
    expect(screen.queryByText('Enviar e-mail para a Ana')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /Aprovações/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Permitir' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/approvals/a1/once')).toBe(true))
  })
})

describe('System panel', () => {
  it('shows load, disk and every part', async () => {
    mockFetch({ '/api/system': {
      process: { cpu_percent: 1.2, heap_bytes: 40e6, sys_bytes: 80e6, goroutines: 30, uptime_s: 7200 },
      host: { cpus: 10, load: [9.5, 8, 7], mem_total: 16e9, disk_free: 8e9, disk_size: 500e9 },
      activity: { explorations: 1, runs: 0, approvals: 2, runs_ok_today: 5, runs_failed_today: 1 },
      components: [{ id: 'telegram', group: 'channel', name: 'Telegram', state: 'ok' }, { id: 'cloud', group: 'backup', name: 'Backup na nuvem', state: 'error', detail: 'bucket not found' }],
      version: '0.5.0',
    } })
    wrap(<SystemPanel open onOpenChange={() => {}} />)
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('95%')).toBeInTheDocument()
    expect(within(dialog).getByText('O computador está muito ocupado; rotinas e respostas podem demorar mais.')).toBeInTheDocument()
    expect(within(dialog).getByText('Pouco espaço em disco: backups e atualizações podem falhar.')).toBeInTheDocument()
    expect(within(dialog).getByText('bucket not found')).toBeInTheDocument()
    expect(within(dialog).getByText('Ligado há 2 h 0 min · versão 0.5.0')).toBeInTheDocument()
  })
})
