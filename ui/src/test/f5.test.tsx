import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Import } from '../pages/Import'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const plan = {
  from: 'hermes', home: '/h', memories: [{ text: 'Gym on Tuesdays', topic: 'memória' }],
  tasks: [{ name: 'daily-brief', prompt: 'Summarize my unread email', schedule: 'cron 0 9 * * *', timezone: 'America/Sao_Paulo', deliver: 'telegram:42', enabled: true }],
  rules: [{ file: 'SOUL.md', text: 'Be terse' }],
  skills: [
    { name: 'triage', description: '', capabilities: ['gmail.search'], missing: [], verdict: 'works' },
    { name: 'deploy', description: '', capabilities: [], missing: ['roda comandos no terminal'], verdict: 'no' },
  ],
  telegram: { has_bot: true }, mail: {}, warnings: ['tarefa "backup" só roda um script'],
}

describe('Import', () => {
  it('shows what would come over before importing, and keeps tokens and trust off by default', async () => {
    const calls = mockFetch({ '/api/migrate/preview': plan, '/api/migrate/apply': { memories: 1, rules: 1, tasks: 1, telegram: false, mail: false } })
    wrap(<Import />)
    await userEvent.click(screen.getByRole('radio', { name: /Hermes/ }))
    await userEvent.click(screen.getByRole('button', { name: /Ver o que vem/ }))
    expect(await screen.findByText('daily-brief')).toBeInTheDocument()
    expect(screen.getByText('Vira rotina')).toBeInTheDocument()
    expect(screen.getByText('roda comandos no terminal')).toBeInTheDocument()
    expect(screen.getByText(/backup/)).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: /Copiar tokens/ })).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByRole('switch', { name: /Confiar/ })).toHaveAttribute('aria-checked', 'false')
    await userEvent.click(screen.getByRole('switch', { name: /Regras/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Importar' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/migrate/apply')?.body).toEqual({ from: 'hermes', home: '', memories: true, rules: false, tasks: true, secrets: false, trust: false }))
    expect(await screen.findByText('Pronto')).toBeInTheDocument()
  })
})
