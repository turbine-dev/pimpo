import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Cost } from '../pages/Cost'
import { Inbox } from '../pages/Inbox'
import { Receipts } from '../pages/Receipts'
import { Rules, describeRule } from '../pages/Rules'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const now = new Date().toISOString()

describe('Receipts', () => {
  it('lists actions in words and undoes one', async () => {
    const calls = mockFetch({
      '/api/receipts': [
        { id: 7, ts: now, type: 'action.done', actor: 'routine:limpeza#3', hash: 'h', data: {}, undoable: true, undone: false,
          action: { source: 'routine:limpeza#3', capability: 'gmail.delete', done: 'gmail.trash', risk: 'irreversible', args: { id: 'INBOX/9' }, result: { moved_to: 'Trash' }, verdict: 'reversible', reason: 'Apagar vira lixeira', ms: 5 } },
        { id: 6, ts: now, type: 'action.done', actor: 'routine:limpeza#3', hash: 'h', data: {}, undoable: false, undone: false,
          action: { source: 'routine:limpeza#3', capability: 'gmail.send', risk: 'irreversible', args: { to: 'x@y.com' }, verdict: 'ask', approved: 'no', error: 'not approved', ms: 1 } },
      ],
    })
    wrap(<Receipts />)
    expect(await screen.findByText(/Bloqueado:/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Desfazer' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/actions/7/undo')).toBe(true))
  })
})

describe('Rules', () => {
  it('turns a sentence into a rule, shows it, tests it and saves it', async () => {
    const draft = { id: 'r-1', text: 'Nunca apague e-mail sem me perguntar', when: { capabilities: ['gmail.delete', 'gmail.trash'] }, then: 'ask' }
    const calls = mockFetch({
      '/api/rules': [{ id: 'preset-irreversible', text: 'Sempre me pergunte antes de qualquer coisa que não dá para desfazer.', when: { min_risk: 'irreversible' }, then: 'ask' }],
      'POST /api/rules/compile': { rule: draft, summary: 'Pergunto antes de apagar ou mandar para a lixeira.' },
      'POST /api/rules/test': { matches: [{ event: 1 }, { event: 2 }] },
      'PUT /api/rules': (b: unknown) => b,
    })
    wrap(<Rules />)
    await userEvent.type(await screen.findByLabelText('Nova regra'), 'Nunca apague e-mail sem me perguntar')
    await userEvent.click(screen.getByRole('button', { name: /Criar/ }))
    expect(await screen.findByText('Entendido assim — confira antes de salvar:')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Testar na última semana/ }))
    expect(await screen.findByText(/teria se aplicado a 2 ações/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Salvar regra/ }))
    await waitFor(() => expect((calls.find((c) => c.method === 'PUT')?.body as unknown[]).length).toBe(2))
  })

  it('describes rules in words', () => {
    expect(describeRule({ id: 'x', text: '', when: { capabilities: ['gmail.archive'], source: 'routine:triagem' }, then: 'allow' })).toBe('Arquiva e-mails · em rotina triagem')
    expect(describeRule({ id: 'x', text: '', when: {}, then: 'block' })).toBe('qualquer ação')
  })
})

describe('Inbox approvals', () => {
  it('answers an approval request', async () => {
    const calls = mockFetch({
      '/api/approvals': [{ id: 'ab12', action: { capability: 'gmail.send', risk: 3, source: 'routine:cobranca#2', args: {} }, text: 'cobranca quer enviar um e-mail para cliente@acme.com', reason: 'Sempre me pergunte', created: now }],
      '/api/explorations?state=ready': [],
      '/api/routines': [],
    })
    wrap(<Inbox />)
    expect(await screen.findByText('cobranca quer enviar um e-mail para cliente@acme.com')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Permitir' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/approvals/ab12/once')).toBe(true))
  })
})

describe('Cost', () => {
  it('shows today against the limit and the month projection', async () => {
    mockFetch({ '/api/cost': { today: 0.42, limit: 1, month: 3.1, projected_month: 7.75, by_day: { '2026-09-24': 0.42 }, by_source: { exploration: 2.5, 'routine:brief': 0.01 } } })
    wrap(<Cost />)
    expect(await screen.findByText('$7.75')).toBeInTheDocument()
    expect(screen.getByText('Rotina brief')).toBeInTheDocument()
    expect(screen.getByText('de $1.00 por dia')).toBeInTheDocument()
  })
})
