import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RoutineApprovals, describeGrant } from '../components/RoutineApprovals'
import { Inbox } from '../pages/Inbox'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const now = new Date().toISOString()

describe('Approve for this routine', () => {
  it('is offered only where it applies and sends the limit the person set', async () => {
    const calls = mockFetch({
      '/api/media': [],
      '/api/needs': { total: 2, counts: { approval: 2 }, items: [
        { kind: 'approval', id: 'ab12', title: 'contas quer pagar 80', detail: 'Sempre me pergunte', created: now, urgency: 3, risk: 3, amount: 80, actions: ['once', 'run', 'always', 'deny', 'routine'] },
        { kind: 'approval', id: 'cd34', title: 'contas quer mandar WhatsApp', detail: 'sempre pergunta', created: now, urgency: 3, risk: 3, actions: ['once', 'run', 'always', 'deny'] },
      ] },
      'POST /api/approvals/ab12/routine': {},
    })
    wrap(<Inbox />)
    expect(await screen.findByText('contas quer pagar 80')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Para esta rotina' })).toHaveLength(1)
    const limit = screen.getByLabelText('Até')
    await userEvent.clear(limit)
    await userEvent.type(limit, '120')
    await userEvent.click(screen.getByRole('button', { name: 'Para esta rotina' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/approvals/ab12/routine')?.body).toEqual({ limit: 120 }))
  })

  it('lists the approvals and takes one back', async () => {
    const calls = mockFetch({
      '/api/grants': [{ id: 'g1', routine: 'brief', routine_name: 'Resumo', version: 3, capability: 'gmail.send', match: { to: 'ana@x.com,bia@x.com' }, created: now }],
    })
    wrap(<RoutineApprovals />)
    expect(await screen.findByText('Aprovado para rotinas')).toBeInTheDocument()
    expect(screen.getByText(/to: ana@x.com, bia@x.com/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /^Desfazer: Resumo/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url === '/api/grants/g1')).toBe(true))
  })

  it('describes hosts and limits', () => {
    expect(describeGrant({ id: 'g', routine: 'r', routine_name: 'R', version: 1, capability: 'shop.pay', scope: 'pay.example', match: { url: 'pay.example' }, limits: { amount: 120 }, created: now }))
      .toBe('url: pay.example · pay.example · amount ≤ 120')
  })
})
