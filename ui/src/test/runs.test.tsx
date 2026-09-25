import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunHistory } from '../components/RunHistory'
import { parseCron, toCron } from '../components/RoutineSettings'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const run = (id: number, name: string, outcome: string, extra = {}) => ({ id, routine: name.toLowerCase(), name, version: 1, started_at: new Date(Date.now() - id * 60000).toISOString(), ended_at: new Date(Date.now() - id * 60000 + 2300).toISOString(), outcome, cost_usd: 0, calls: 3, ...extra })

describe('RunHistory', () => {
  it('lists every run and filters the failed ones', async () => {
    const calls = mockFetch({
      '/api/runs?outcome=&before=0&limit=50': [run(1, 'Marés', 'failed', { error: 'the tide API timed out' }), run(2, 'Resumo', 'ok')],
      '/api/runs?outcome=failed&before=0&limit=50': [run(1, 'Marés', 'failed', { error: 'the tide API timed out' })],
    })
    wrap(<RunHistory />)
    expect(await screen.findByRole('link', { name: 'Resumo' })).toHaveAttribute('href', '/routines/resumo')
    expect(screen.getByText('the tide API timed out')).toBeInTheDocument()
    expect(screen.getAllByText(/3 chamadas · levou 2.3 s/)).toHaveLength(2)
    await userEvent.click(screen.getByRole('button', { name: 'Com falha' }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('outcome=failed'))).toBe(true))
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Resumo' })).not.toBeInTheDocument())
  })
})

describe('minute schedules', () => {
  it('reads and writes every few minutes', () => {
    const s = parseCron('*/15 * * * *')
    expect(s.freq).toBe('minutes')
    expect(s.minutes).toBe(15)
    expect(toCron({ ...s, minutes: 5 })).toBe('*/5 * * * *')
    expect(parseCron('*/7 * * * *').freq).toBe('custom')
  })
})
