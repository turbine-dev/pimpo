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
      '/api/needs': { total: 4, counts: { approval: 2, failed_routine: 1, exploration_ready: 1 }, items: [
        { kind: 'approval', id: 'a1', title: 'Enviar e-mail para a Ana', urgency: 3, actions: ['once', 'run', 'always', 'deny'], risk: 3 },
        { kind: 'approval', id: 'a2', title: 'Arquivar 3 e-mails', urgency: 3, actions: ['once', 'run', 'always', 'deny'], risk: 2 },
        { kind: 'failed_routine', id: 'news', title: 'Notícias', urgency: 1, actions: ['run', 'repair', 'open'], link: '/routines/news' },
        { kind: 'exploration_ready', id: 'e1', title: 'Resumo semanal', urgency: 0, actions: ['open'], link: '/explorations/e1' },
      ] },
    })
    wrap(<Routes><Route path="/" element={<Home />} /><Route path="/chat/:id" element={<p>conversa aberta</p>} /></Routes>)
    // The three most urgent, and how many more wait.
    const top = await screen.findByRole('list', { name: 'Precisa de você' })
    expect(within(top).getByText('Enviar e-mail para a Ana')).toBeInTheDocument()
    expect(within(top).getByText('Notícias não rodou')).toBeInTheDocument()
    expect(within(top).queryByText('Resumo semanal')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Mais 1/ })).toHaveAttribute('href', '/inbox')
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

describe('Home running now', () => {
  it('shows what runs now above the day', async () => {
    mockFetch({
      '/api/needs': { total: 0, counts: {}, items: [] },
      '/api/progress': [{ id: 'run:brief:3', person: 'owner', kind: 'run', routine: 'brief', run: 3, title: 'Resumo matinal', label: 'gmail.search', done: 4, total: 0, state: 'running', cost_usd: 0, started_at: now.toISOString(), updated_at: now.toISOString() }],
    })
    wrap(<Home />)
    const section = await screen.findByRole('region', { name: 'Rodando agora' })
    expect(within(section).getByText('Resumo matinal')).toBeInTheDocument()
  })
})

describe('Inbox panel', () => {
  it('answers an approval in place and filters by kind', async () => {
    const calls = mockFetch({
      '/api/needs': { total: 3, counts: { approval: 1, failed_routine: 1, system: 1 }, items: [
        { kind: 'approval', id: 'a1', title: 'Enviar e-mail para a Ana', detail: 'irreversível', created: now.toISOString(), urgency: 3, actions: ['once', 'run', 'always', 'deny'], risk: 3 },
        { kind: 'failed_routine', id: 'news', title: 'Notícias', urgency: 1, actions: ['run', 'repair', 'open'], link: '/routines/news' },
        { kind: 'system', id: 'signal', title: 'Signal', detail: 'signal-cli is not answering', urgency: 1, actions: [], link: '/settings' },
      ] },
      'POST /api/approvals/a1/once': {},
    })
    wrap(<InboxPanel onClose={() => {}} />)
    expect(await screen.findByText('Enviar e-mail para a Ana')).toBeInTheDocument()
    expect(screen.getByText('Notícias não rodou')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /Sistema/ }))
    expect(screen.getByText('signal-cli is not answering')).toBeInTheDocument()
    expect(screen.queryByText('Enviar e-mail para a Ana')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /Aprovações/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Permitir' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/approvals/a1/once' && c.method === 'POST')).toBe(true))
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
