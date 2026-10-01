import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Inbox } from '../pages/Inbox'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Earned autonomy', () => {
  it('gives a member the kind it earned from the needs list', async () => {
    const calls = mockFetch({
      '/api/media': [],
      '/api/needs': { total: 1, counts: { company_autonomy: 1 }, items: [
        { kind: 'company_autonomy', id: 'earn:co_1:clara:github.pr_create', title: 'Lume Moda: Clara', detail: 'github.pr_create', count: 10, created: new Date().toISOString(), urgency: 1, link: '/companies/co_1', actions: ['accept', 'dismiss'] },
      ] },
      'POST /api/companies/co_1/members/clara/earn': {},
    })
    wrap(<Inbox />)
    expect(await screen.findByText(/Abre um pull request no GitHub · 10 aprovadas seguidas/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Deixar fazer sozinho' }))
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/earn'))?.body).toEqual({ capability: 'github.pr_create', accept: true }))
  })
})
