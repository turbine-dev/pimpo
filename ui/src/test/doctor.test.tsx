import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SystemPanel } from '../components/SystemPanel'
import { LocaleProvider } from '../lib/i18n'
import { mockFetch } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const system = {
  process: { cpu_percent: 1, heap_bytes: 2e7, sys_bytes: 4e7, goroutines: 30, uptime_s: 60 },
  host: { cpus: 8, load: [1, 1, 1], mem_total: 16e9, disk_free: 50e9, disk_size: 500e9 },
  activity: { explorations: 0, runs: 0, approvals: 0, runs_ok_today: 0, runs_failed_today: 0 },
  components: [], version: 'dev',
}

describe('Check-up', () => {
  it('tests everything and says how to fix each failure, with a way there', async () => {
    const calls = mockFetch({
      '/api/system': system,
      'POST /api/doctor': [
        { id: 'model:groq:llama-4', group: 'brain', name: 'groq:llama-4', state: 'fail', detail: 'groq refused the API key', fix: 'doc.fix.model.key', link: '/settings#modelos' },
        { id: 'routines', group: 'system', name: 'Rotinas', state: 'warn', detail: '1', fix: 'doc.fix.routines', link: '/routines' },
        { id: 'telegram', group: 'channel', name: 'Telegram', state: 'ok', detail: '@pimpo_bot' },
      ],
    })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={qc}><LocaleProvider locale="pt"><MemoryRouter><SystemPanel open onOpenChange={() => {}} /></MemoryRouter></LocaleProvider></QueryClientProvider>)
    await userEvent.click(await screen.findByRole('button', { name: 'Verificar tudo' }))
    expect(await screen.findByText('1 com problema · 1 para ver · 1 funcionando')).toBeInTheDocument()
    const groq = screen.getByText('groq:llama-4').closest('li')!
    expect(within(groq).getByText(/A chave foi recusada/)).toBeInTheDocument()
    expect(within(groq).getByRole('link', { name: 'Abrir' })).toHaveAttribute('href', '/settings#modelos')
    expect(screen.getByText('Rotinas').closest('li')).toHaveTextContent('Há rotinas paradas')
    expect(calls.some((c) => c.method === 'POST' && c.url === '/api/doctor')).toBe(true)
  })
})
