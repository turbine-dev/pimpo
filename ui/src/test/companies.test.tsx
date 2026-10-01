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

describe('Company memory and meetings', () => {
  it('keeps a decision and calls a meeting', async () => {
    const notes = [{ id: 'n_1', company: 'co_1', kind: 'minutes', title: 'Planejamento', body: 'Lançar sexta.', by: 'member:co_1/bia', created: new Date().toISOString() }]
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/notes': notes, '/api/companies/co_1/meetings': [],
      'POST /api/companies/co_1/notes': notes[0], 'POST /api/companies/co_1/meetings': { id: 'm_1' } })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Memória' }))
    expect(await screen.findByText(/Ata · Bia/)).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Título'), 'Trocas')
    await userEvent.type(screen.getByLabelText('Texto'), 'Até 30 dias.')
    await userEvent.click(screen.getByRole('button', { name: /Guardar/ }))
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/notes') && c.method === 'POST')?.body).toEqual({ scope: 'company', kind: 'decision', title: 'Trocas', body: 'Até 30 dias.' }))
  })

  it("approves an agent's note and keeps notes after each piece of work", async () => {
    const now = new Date().toISOString()
    const notes = [
      { id: 'n_1', company: 'co_1', scope: 'company', pending: true, kind: 'fact', title: 'Fornecedor é a Acme', body: 'Acme.', by: 'member:co_1/bia', created: now },
      { id: 'n_2', company: 'co_1', scope: 'member', of: 'clara', kind: 'lesson', title: 'Responder em uma hora', body: 'Clientes esperam pouco.', by: 'member:co_1/clara', created: now },
      { id: 'n_3', company: 'co_1', scope: 'member', of: 'bia', kind: 'progress', title: 'Fechou o pedido', body: 'Feito.', by: 'system', created: now },
    ]
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/notes': notes, '/api/companies/co_1/tasks': [],
      'POST /api/companies/co_1/notes/n_1/approve': notes[0], 'PUT /api/companies/co_1': org })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Memória' }))
    expect(await screen.findByRole('heading', { name: 'Aguardando aprovação' })).toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: /Guardar$/ })[0])
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/notes/n_1/approve') && c.method === 'POST')).toBe(true))
    await userEvent.click(within(screen.getByRole('group', { name: 'De quem é a memória' })).getByRole('button', { name: 'Agentes' }))
    expect(screen.getByText(/Lição · Clara/)).toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Agente'), 'bia')
    expect(screen.queryByText('Responder em uma hora')).not.toBeInTheDocument()
    expect(screen.getByText(/Progresso · Pimpo/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('switch', { name: 'Notas automáticas em Agentes' }))
    await userEvent.selectOptions(screen.getByLabelText('Como os agentes escrevem na memória de Empresa'), 'free')
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect((calls.find((c) => c.method === 'PUT')?.body as { memory: unknown }).memory).toEqual({
      company: { write: 'free' }, member: { write: 'free', auto: true }, task: { write: 'free' },
    }))
  })

  it('opens a meeting with the CEO and talks to one agent in it', async () => {
    const room = { id: 'm_1', company: 'co_1', title: 'Coleção', agenda: 'Quando lançar?', chair: 'ceo', participants: ['bia', 'clara'], rounds: 0, max_usd: 1, state: 'open', with_ceo: true,
      transcript: [{ member: 'ceo', text: 'Bom dia' }, { member: 'bia', text: 'Bom dia! Sexta.' }], cost_usd: 0.01, called_by: 'human:owner', created: new Date().toISOString() }
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/meetings': [], 'POST /api/companies/co_1/meetings': room,
      '/api/companies/co_1/meetings/m_1': room, 'POST /api/companies/co_1/meetings/m_1/say': room })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Reuniões' }))
    await userEvent.click(screen.getByRole('button', { name: /Nova reunião/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Assunto'), 'Coleção')
    await userEvent.type(within(dialog).getByLabelText('Pauta'), 'Quando lançar?')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Começar reunião' }))
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/meetings') && c.method === 'POST')?.body).toEqual({ title: 'Coleção', agenda: 'Quando lançar?', participants: ['bia', 'clara'], max_usd: 1, with_ceo: true }))
    expect(await screen.findByText('Bom dia! Sexta.')).toBeInTheDocument()
    await userEvent.click(within(screen.getByRole('group', { name: 'Falar com' })).getByRole('button', { name: 'Clara' }))
    await userEvent.type(screen.getByLabelText('Sua mensagem'), 'E as fotos?{Enter}')
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/say'))?.body).toEqual({ text: 'E as fotos?', to: ['clara'] }))
  })
})

describe('Company finance', () => {
  it('shows what looks wrong and applies a proposed budget', async () => {
    const costs = { company: { day: 1, month: 9 }, members: {}, departments: {}, by_day: {}, forecast_month: 20, per_member: {}, per_role: {}, budget: {}, limits: {}, outcomes: {} }
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/costs': costs,
      '/api/companies/co_1/month': { month: '2026-10', total: 9, members: {}, departments: {}, previous_total: 8, subscription: 0, anomalies: ['2026-10-01: $9.00 spent, against a usual $0.50 a day'] },
      '/api/companies/co_1/proposals': [{ id: 'p_1', company: 'co_1', by: 'bia', scope: 'member', of: 'clara', month_usd: 30, was: 10, reason: 'Clara atende o dobro', state: 'proposed', created: '' }],
      'POST /api/companies/co_1/proposals/p_1': {} })
    wrap(routes(), '/companies/co_1?tab=costs')
    expect(await screen.findByText(/\$9.00 spent, against a usual/)).toBeInTheDocument()
    expect(screen.getByText('Clara: $30.00 por mês (hoje $10.00)')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Aplicar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url.endsWith('/proposals/p_1'))?.body).toEqual({ accept: true }))
  })
})

describe('Company media', () => {
  it('shows each video with what its checks found', async () => {
    const maker: Org = { ...org, roles: [...org.roles, { id: 'video', title: 'Vídeo', capabilities: ['media.render'] }] }
    const now = new Date().toISOString()
    mockFetch({ '/api/companies/co_1': maker, '/api/state': { person: 'owner' }, '/api/companies/co_1/media': [
      { id: 'm_000000000001', company: 'co_1', member: 'bia', kind: 'video', file: 'm_000000000001.mp4', title: 'Como rodar uma rotina', format: 'short', seconds: 42, check: { seconds: 42, width: 1080, height: 1920, captions: false, problems: ['it has no captions'] }, created: now },
      { id: 'm_000000000002', company: 'co_1', member: 'bia', kind: 'video', file: 'm_000000000002.mp4', title: 'Tour', format: 'tutorial', seconds: 300, check: { seconds: 300, width: 1920, height: 1080, captions: true, problems: [] }, created: now },
      { id: 'm_000000000003', company: 'co_1', member: 'bia', kind: 'image', file: 'm_000000000003.png', title: 'example.com/', created: now },
    ] })
    wrap(routes(), '/companies/co_1?tab=media')
    expect(await screen.findByText('it has no captions')).toBeInTheDocument()
    expect(screen.getByText('Passa nas verificações')).toBeInTheDocument()
    expect(screen.getByText(/Curto \(9:16\) · 42 s · Bia/)).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'example.com/' })).toHaveAttribute('src', '/api/companies/co_1/media/m_000000000003')
  })
})

describe('Company product', () => {
  it('accepts a brief, shows its flagged claims and marks it shipped', async () => {
    const po: Org = { ...org, roles: [...org.roles, { id: 'po', title: 'PO', capabilities: ['company.brief', 'company.signal'] }] }
    const brief = { id: 'b_1', company: 'co_1', author: 'bia', title: 'Modo escuro', problem: 'Quem compra à noite sai', proposal: 'Um tema escuro', created: new Date().toISOString(),
      claims: [{ text: 'Clientes pedem', source: 'https://github.com/ana/shop/issues/4', quote: 'Coloquem modo escuro' }, { text: 'Metade das visitas é à noite', source: 'analytics', quote: 'inventado', flag: 'Jev doubts the quote supports it (10% sure)' }],
      scores: { value: 4, differentiation: 2, adoption: 4, build_risk: 2, safety_risk: 1 }, score: 15, predictions: [{ metric: 'conversão noturna', expected: '+5%' }], state: 'proposed' }
    let current = { briefs: [brief], signals: [{ id: 's_1', company: 'co_1', source: 'issue', title: 'Modo escuro', count: 3, seen: ['fórum'], by: 'x', created: '' }], accuracy: {} }
    const calls = mockFetch({ '/api/companies/co_1': po, '/api/state': { person: 'owner' }, '/api/companies/co_1/product': () => current,
      'POST /api/companies/co_1/briefs/b_1/state': (body: { state: string }) => { current = { ...current, briefs: [{ ...brief, state: body.state }] }; return current.briefs[0] } })
    wrap(routes(), '/companies/co_1?tab=product')
    expect(await screen.findByText('Nota 15')).toBeInTheDocument()
    expect(screen.getByText(/Jev doubts the quote/)).toBeInTheDocument()
    expect(screen.getByText(/issue, fórum · ouvido 3×/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Aceitar' }))
    await userEvent.type(await screen.findByLabelText(/Onde foi entregue/), 'https://github.com/ana/shop/pull/9')
    await userEvent.click(screen.getByRole('button', { name: 'Marcar como entregue' }))
    await waitFor(() => expect(calls.filter((c) => c.method === 'POST' && c.url.endsWith('/state')).map((c) => c.body)).toEqual([{ state: 'accepted' }, { state: 'shipped', ref: 'https://github.com/ana/shop/pull/9' }]))
  })
})

describe('Company coding', () => {
  it('gives a developer a coding CLI, its sandbox and the variables it codes with', async () => {
    const coding: Org = { ...org, roles: [...org.roles, { id: 'dev', title: 'Dev', capabilities: ['code.workspace'] }], members: [...org.members, { id: 'rui', kind: 'agent', role: 'dev', reports_to: 'ceo', name: 'Rui', state: 'active' }] }
    const calls = mockFetch({ '/api/companies/co_1': coding, '/api/state': { person: 'owner' }, 'PUT /api/companies/co_1/members/rui': coding, 'PUT /api/companies/co_1': coding,
      '/api/companies/co_1/coders': [{ id: 'claude', name: 'Claude Code', sandbox: true, installed: true }, { id: 'codex', name: 'Codex', sandbox: true, installed: false }, { id: 'opencode', name: 'opencode', sandbox: false, installed: true }] })
    wrap(routes(), '/companies/co_1')
    const chart = (await screen.findAllByLabelText('Organograma', { selector: '.org' }))[0]
    await userEvent.click(within(chart).getByRole('button', { name: 'Abrir Clara' }))
    expect(within(await screen.findByRole('dialog')).queryByText('Programação')).not.toBeInTheDocument()
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Fechar/ }))
    await userEvent.click(within(chart).getByRole('button', { name: 'Abrir Rui' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.selectOptions(await within(dialog).findByLabelText('CLI de código'), 'codex')
    expect(within(dialog).getByText('Codex não está instalado neste computador.')).toBeInTheDocument()
    await userEvent.click(within(dialog).getByRole('switch', { name: 'Programar no sandbox do CLI' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.url.endsWith('/members/rui'))?.body).toMatchObject({ coder: 'codex', code_sandbox: true }))
    await userEvent.click(await screen.findByRole('button', { name: /Editar empresa/ }))
    await userEvent.type(within(await screen.findByRole('dialog')).getByLabelText('Variáveis para programar'), 'API_URL=http://localhost:8080')
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect((calls.find((c) => c.method === 'PUT' && c.url.endsWith('/co_1'))?.body as { code_env: unknown }).code_env).toEqual({ API_URL: 'http://localhost:8080' }))
  })
})

describe('Company autonomy and decision levels', () => {
  it("gives a member Jev to decide what it would ask", async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, 'PUT /api/companies/co_1/members/clara': org })
    wrap(routes(), '/companies/co_1')
    const chart = (await screen.findAllByLabelText('Organograma', { selector: '.org' }))[0]
    await userEvent.click(within(chart).getByRole('button', { name: 'Abrir Clara' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: /Mais uma linha/ }))
    await userEvent.selectOptions(within(dialog).getByLabelText('Quem decide'), 'jev')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect((calls.find((c) => c.method === 'PUT')?.body as { autonomy: unknown }).autonomy).toEqual([{ min_risk: 'irreversible', decider: { kind: 'jev', threshold: 0.9 } }]))
  })

  it('sets the suggested levels and simulates one', async () => {
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/decisions': [], 'PUT /api/companies/co_1': org,
      'POST /api/companies/co_1/levels/simulate': { level: 4, name: 'Estratégica', decides: 'ceo', decider: 'Ana', why: 'kind price' } })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Alçadas' }))
    await userEvent.click(screen.getByRole('button', { name: 'Usar os quatro níveis sugeridos' }))
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => {
      const body = calls.find((c) => c.method === 'PUT')?.body as { levels: { list: { decides: string }[] } }
      expect(body.levels.list.map((l) => l.decides)).toEqual(['self', 'boss', 'head', 'ceo'])
    })
    await userEvent.type(screen.getByLabelText('A decisão'), 'Mudar o preço?')
    await userEvent.type(screen.getByLabelText('Tipo'), 'price')
    await userEvent.click(screen.getByRole('button', { name: 'Ver o nível' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Nível 4 (Estratégica): decide Ana')
  })
})

describe('Company costs', () => {
  it('shows spending, subscription use and sets a salary', async () => {
    const costs = { company: { day: 0.4, month: 3.2 }, members: { clara: { day: 0.4, month: 3.2 } }, departments: {}, by_day: {}, subscription: { company: { day: 0, month: 12 }, members: { clara: { day: 0, month: 12 } }, departments: {}, by_day: {} },
      forecast_month: 9.5, per_member: { clara: { done: 4, cost_per_done: 0.8, forecast_month: 9.5 } }, per_role: {}, budget: {}, limits: {}, outcomes: {} }
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/costs': costs, 'PUT /api/companies/co_1': org, 'PUT /api/companies/co_1/members/clara': org })
    wrap(routes(), '/companies/co_1')
    await userEvent.click(await screen.findByRole('tab', { name: 'Custos' }))
    expect(await screen.findByText('$9.50', { selector: 'p' })).toBeInTheDocument()
    expect(screen.getByText('$12.00', { selector: 'p' })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Salário de Clara'), '20')
    await userEvent.click(screen.getByRole('checkbox', { name: /Contar o trabalho por assinatura/ }))
    await userEvent.click(screen.getAllByRole('button', { name: 'Salvar' })[0])
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/members/clara'))?.body).toMatchObject({ budget: { month_usd: 20 } }))
    expect(calls.find((c) => c.method === 'PUT' && c.url === '/api/companies/co_1')?.body).toMatchObject({ budget: { subscription: true } })
  })
})

describe('Member accounts', () => {
  it("connects a member's own account and warns about its kind", async () => {
    const accounts = { missing: ['github'], accounts: [
      { kind: 'mail', name: 'mail', fields: [{ name: 'addr', label: 'IMAP server' }, { name: 'password', label: 'Password', secret: true }], configured: false, values: {} },
      { kind: 'github', name: 'GitHub', fields: [{ name: 'token', label: 'Token', secret: true }], configured: false, values: {} },
    ] }
    const calls = mockFetch({ '/api/companies/co_1': org, '/api/state': { person: 'owner' }, '/api/companies/co_1/members/clara/accounts': accounts,
      'PUT /api/companies/co_1/members/clara/accounts/github': { kind: 'github', warning: "This service's terms may not allow automation on this kind of account." } })
    wrap(routes(), '/companies/co_1')
    const chart = (await screen.findAllByLabelText('Organograma', { selector: '.org' }))[0]
    await userEvent.click(within(chart).getByRole('button', { name: 'Abrir Clara' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByText('Contas pessoais'))
    expect(await within(dialog).findByText('Faltam contas: github')).toBeInTheDocument()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Conectar' }))
    await userEvent.type(within(dialog).getByLabelText('Token'), 'ghp_x')
    await userEvent.selectOptions(within(dialog).getByLabelText('Tipo de conta'), 'brand')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Conectar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ token: 'ghp_x', account_kind: 'brand' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('terms')
  })
})
