import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { diffLines } from '../components/Diff'
import { Palette } from '../components/Palette'
import { Welcome } from '../pages/Welcome'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Welcome', () => {
  it('applies the chosen autonomy and budget, then opens the first task', async () => {
    const calls = mockFetch({ '/api/setup': { done: false, demo: true, telegram: false, mail: false, calendar: false, preset: '', claude: true }, 'PUT /api/rules/preset': [], 'PUT /api/budget': null, 'POST /api/setup/done': null })
    const first = vi.fn()
    wrap(<Welcome onFirstTask={first} />)
    expect(await screen.findByText(/Modo demonstração/)).toBeInTheDocument()
    for (const title of ['Onde eu falo com você', 'O que eu posso ler', 'O que eu posso fazer sem te perguntar?']) {
      await userEvent.click(screen.getByRole('button', { name: /Continuar/ }))
      await screen.findByRole('heading', { name: title })
    }
    await userEvent.click(await screen.findByRole('button', { name: /Liberal/ }))
    await userEvent.click(screen.getByRole('button', { name: /Continuar/ }))
    const budget = await screen.findByLabelText('Limite diário em dólares')
    await userEvent.clear(budget)
    await userEvent.type(budget, '3')
    await userEvent.click(screen.getByRole('button', { name: /Pedir a primeira tarefa/ }))
    await waitFor(() => expect(first).toHaveBeenCalled())
    const order = calls.filter((c) => c.method !== 'GET').map((c) => `${c.method} ${c.url}`)
    expect(order).toEqual(['PUT /api/rules/preset', 'PUT /api/budget', 'POST /api/setup/done'])
    expect(calls.find((c) => c.url === '/api/rules/preset')?.body).toEqual({ preset: 'liberal' })
    expect(calls.find((c) => c.url === '/api/budget')?.body).toEqual({ daily_usd: 3 })
  })
})

describe('Palette', () => {
  it('finds routines and pages and starts a task', async () => {
    mockFetch({ '/api/routines': [{ id: 'brief', name: 'Resumo matinal', description: '', state: 'active', version: 1, runs: [], cost_month_usd: 0, capabilities: [], schedule: '0 7 * * *' }] })
    const newTask = vi.fn()
    wrap(<Palette open onOpenChange={() => {}} onNewTask={newTask} />)
    expect(await screen.findByText('Resumo matinal')).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'Buscar' }), 'nova')
    expect(screen.queryByText('Resumo matinal')).not.toBeInTheDocument()
    await userEvent.keyboard('{Enter}')
    expect(newTask).toHaveBeenCalled()
  })
})

describe('diffLines', () => {
  it('marks added and removed lines', () => {
    const d = diffLines('a\nb\nc', 'a\nx\nc')
    expect(d.map((l) => `${l.kind}:${l.text}`)).toEqual(['same:a', 'del:b', 'add:x', 'same:c'])
  })
})
