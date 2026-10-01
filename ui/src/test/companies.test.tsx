import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import type { Org } from '../lib/api'
import { Companies } from '../pages/Companies'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const org: Org = {
  id: 'co_1', person: 'owner', name: 'Lume Moda', industry: 'Loja online', grant: 'configure', created: '', updated: '',
  departments: [{ id: 'vendas', name: 'Vendas', color: 'chart-2' }],
  roles: [{ id: 'gerente', title: 'Gerente' }, { id: 'atendente', title: 'Atendente', function: 'Responde clientes' }],
  members: [
    { id: 'ceo', kind: 'person', person: 'owner', title: 'CEO', name: 'Ana' },
    { id: 'bia', kind: 'agent', role: 'gerente', reports_to: 'ceo', name: 'Bia', state: 'active' },
    { id: 'clara', kind: 'agent', role: 'atendente', department: 'vendas', reports_to: 'bia', name: 'Clara', state: 'active' },
  ],
  contexts: [], rules: [], agent_routines: [],
}

const routes = () => <Routes><Route path="/companies" element={<Companies />} /><Route path="/companies/:id" element={<Companies />} /></Routes>

describe('Companies', () => {
  it('creates a company and opens it', async () => {
    const calls = mockFetch({ '/api/companies': [], 'POST /api/companies': { ...org, members: [org.members[0]], roles: [], departments: [] }, '/api/companies/co_1': org, '/api/state': { person: 'owner' } })
    wrap(routes(), '/companies')
    await userEvent.click((await screen.findAllByRole('button', { name: /Nova empresa/ }))[0])
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Lume Moda')
    await userEvent.type(within(dialog).getByLabelText('Ramo'), 'Loja online')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Criar empresa' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Lume Moda', industry: 'Loja online', mission: '' }))
    expect(await screen.findByRole('heading', { name: 'Lume Moda' })).toBeInTheDocument()
  })

  it('draws the tree and changes a boss from the member dialog', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, 'PUT /api/companies/co_1/members/clara': org })
    wrap(routes(), '/companies/co_1')
    const charts = await screen.findAllByLabelText('Organograma', { selector: '.org' })
    expect(within(charts[0]).getAllByRole('button').map((b) => b.textContent)).toEqual(['AnaCEO', 'BBiaGerente', 'CClaraAtendente'])
    await userEvent.click(within(charts[0]).getByRole('button', { name: 'Abrir Clara' }))
    const dialog = await screen.findByRole('dialog')
    const boss = within(dialog).getByLabelText('Responde a')
    // Clara cannot report to herself.
    expect(within(boss).getAllByRole('option').map((o) => o.textContent)).toEqual(['Ana', 'Bia'])
    await userEvent.selectOptions(boss, 'ceo')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect((calls.find((c) => c.method === 'PUT')?.body as { reports_to: string }).reports_to).toBe('ceo'))
  })

  it('hires into an existing role with an id of its own', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, 'PUT /api/companies/co_1/members/clara-2': org })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('button', { name: 'Contratar' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Clara')
    await userEvent.selectOptions(within(dialog).getByLabelText('Cargo'), 'atendente')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ id: 'clara-2', name: 'Clara', role: 'atendente', reports_to: 'ceo', kind: 'agent' }))
  })

  it('shows a partner what they may not change', async () => {
    mockFetch({ '/api/companies/co_1': { ...org, grant: 'view', person: 'ana' }, '/api/state': { person: 'owner' } })
    wrap(routes(), '/companies/co_1')
    expect(await screen.findByRole('heading', { name: 'Lume Moda' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Contratar' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Apagar/ })).toBeNull()
  })
})

describe('Company context and rules', () => {
  const caps = [{ name: 'gmail.send', risk: 'irreversible', signature: '', returns: '' }]

  it('asks before saving a rule that goes against a broader one, then saves it as an exception', async () => {
    const withRule: Org = { ...org, rules: [{ id: 'no-email', scope: 'company', text: 'Never email customers', when: { capabilities: ['gmail.send'] }, then: 'block' }] }
    const puts: unknown[] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
      if (method === 'PUT') {
        const body = JSON.parse(String(init?.body))
        puts.push(body)
        return body.exception ? json(withRule) : json({ error: 'this rule allows what a broader rule forbids (no-email); save it as an exception' }, 409)
      }
      if (url === '/api/capabilities') return json(caps)
      if (url === '/api/state') return json({ person: 'owner' })
      return json(withRule)
    }))
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Contexto e regras' }))
    expect(await screen.findByText('Never email customers')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Nova regra/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.selectOptions(within(dialog).getByLabelText('Vale para'), 'role:atendente')
    await userEvent.type(within(dialog).getByLabelText('A regra, em palavras'), 'Clerks may email')
    await userEvent.selectOptions(within(dialog).getByLabelText('O que acontece'), 'allow')
    await userEvent.click(within(dialog).getByRole('checkbox', { name: /gmail\.send/ }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await userEvent.click(await within(dialog).findByRole('button', { name: 'Salvar como exceção' }))
    await waitFor(() => expect(puts).toHaveLength(2))
    expect(puts[0]).toMatchObject({ scope: 'role', of: 'atendente', text: 'Clerks may email', then: 'allow', exception: false, when: { capabilities: ['gmail.send'] } })
    expect(puts[1]).toMatchObject({ id: 'clerks-may-email', exception: true })
  })

  it('writes a context for the whole company', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, 'PUT /api/companies/co_1/contexts/trocas': org })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Contexto e regras' }))
    await userEvent.click(screen.getByRole('button', { name: /Novo contexto/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Título'), 'Trocas')
    await userEvent.type(within(dialog).getByLabelText('Texto'), 'Até 30 dias.')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ id: 'trocas', scope: 'company', title: 'Trocas', body: 'Até 30 dias.' }))
  })
})

describe('Company work', () => {
  it('shows who is working and what waits', async () => {
    mockFetch({ '/api/companies/co_1': { ...org, activity: { clara: { state: 'working', task: 'Answer WhatsApp' }, bia: { state: 'queued', queue: 2 } } }, '/api/state': { person: 'owner' } })
    wrap(routes(), '/companies/co_1')
    const chart = (await screen.findAllByLabelText('Organograma', { selector: '.org' }))[0]
    expect(await within(chart).findByLabelText('Trabalhando em: Answer WhatsApp')).toBeInTheDocument()
    expect(within(chart).getByLabelText('Na fila: 2')).toBeInTheDocument()
  })

  it('gives a member work and makes it a routine', async () => {
    const calls = mockFetch({
      '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/routines': [],
      'POST /api/companies/co_1/members/clara/work': { id: 'w_1', state: 'queued' },
      'PUT /api/companies/co_1/agent-routines/manha': org,
    })
    wrap(routes(), '/companies/co_1')
    const chart = (await screen.findAllByLabelText('Organograma', { selector: '.org' }))[0]
    await userEvent.click(within(chart).getByRole('button', { name: 'Abrir Clara' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByText('Trabalho e rotinas'))
    await userEvent.type(within(dialog).getByLabelText('Dar uma tarefa agora'), 'Responda o WhatsApp')
    await userEvent.click(within(dialog).getByRole('button', { name: /Entregar/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ request: 'Responda o WhatsApp' }))
    await userEvent.click(within(dialog).getByRole('button', { name: /Nova rotina/ }))
    const routine = (await screen.findAllByRole('dialog')).at(-1)!
    await userEvent.type(within(routine).getByLabelText('Nome da rotina'), 'Manhã')
    await userEvent.type(within(routine).getByLabelText('O que fazer'), 'Leia os pedidos da noite')
    await userEvent.selectOptions(within(routine).getByLabelText('Quando'), '0 8 * * *')
    await userEvent.click(within(routine).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.url.includes('agent-routines'))?.body).toEqual({ id: 'manha', member: 'clara', name: 'Manhã', instructions: 'Leia os pedidos da noite', schedule: '0 8 * * *', max_usd: 0.5 }))
  })

  it('lists the work and stops what is running', async () => {
    const calls = mockFetch({
      '/api/companies/co_1': org, '/api/state': { person: 'owner' },
      '/api/companies/co_1/work': [
        { id: 'w_1', company: 'co_1', member: 'clara', request: 'Answer WhatsApp', from: 'person:owner', max_usd: 0.5, state: 'running', cost_usd: 0.03, queued: '' },
        { id: 'w_0', company: 'co_1', member: 'bia', request: 'Weekly report', from: 'routine:weekly', max_usd: 1, state: 'done', summary: 'Sent.', cost_usd: 0.2, queued: '' },
      ],
      'POST /api/companies/co_1/work/w_1/stop': { id: 'w_1', state: 'stopped' },
    })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Trabalho' }))
    expect((await screen.findAllByText('Weekly report')).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('button', { name: 'Parar' })).toHaveLength(1)
    await userEvent.click(screen.getByRole('button', { name: 'Parar' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/w_1/stop'))).toBe(true))
  })
})

describe('Company tasks and questions', () => {
  const tasks = [
    { id: 't_1', company: 'co_1', root: 't_1', depth: 1, requester: 'ceo', assignee: 'bia', title: 'Lançar a coleção', objective: 'Vender', acceptance: 'No ar', state: 'doing', cost_usd: 0, created: '', updated: '' },
    { id: 't_2', company: 'co_1', parent: 't_1', root: 't_1', depth: 2, requester: 'bia', assignee: 'clara', title: 'Fotos dos produtos', objective: 'Fotos', acceptance: '20 fotos', state: 'waiting', drift: true, dossier: [{ kind: 'task', ref: 't_1', title: 'Lançar a coleção' }], cost_usd: 0, created: '', updated: '' },
  ]
  const questions = [{ id: 'q_1', company: 'co_1', from: 'bia', to: 'ceo', kind: 'decide', text: 'Dar 15% de desconto?', options: ['Sim', 'Não'], recommendation: 'Sim, só hoje', asked: new Date().toISOString() }]

  it('shows tasks by state and answers what was asked of the CEO', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/tasks': tasks, '/api/companies/co_1/questions': questions,
      'POST /api/companies/co_1/questions/q_1/answer': { ...questions[0], answer: 'Não' } })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Tarefas' }))
    const doing = await screen.findByRole('region', { name: 'Fazendo' })
    expect(within(doing).getByText('Lançar a coleção')).toBeInTheDocument()
    const waiting = screen.getByRole('region', { name: 'Esperando ou travada' })
    expect(within(waiting).getByText('Pode ter fugido do objetivo')).toBeInTheDocument()
    expect(screen.getByText('Recomenda: Sim, só hoje')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Não' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ choice: 'Não' }))
  })

  it('hands a member a task with what it needs', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/tasks': [], '/api/companies/co_1/questions': [], 'POST /api/companies/co_1/tasks': tasks[0] })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Tarefas' }))
    await userEvent.click(screen.getByRole('button', { name: /Nova tarefa/ }))
    const dialog = await screen.findByRole('dialog')
    const send = within(dialog).getByRole('button', { name: 'Passar a tarefa' })
    await userEvent.selectOptions(within(dialog).getByLabelText('Para quem'), 'clara')
    await userEvent.type(within(dialog).getByLabelText('Título'), 'Responder clientes')
    await userEvent.type(within(dialog).getByLabelText('Objetivo'), 'Ninguém sem resposta')
    expect(send).toBeDisabled()
    await userEvent.type(within(dialog).getByLabelText('Pronta quando'), 'Caixa vazia')
    await userEvent.click(send)
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ assignee: 'clara', title: 'Responder clientes', objective: 'Ninguém sem resposta', acceptance: 'Caixa vazia' }))
  })
})
