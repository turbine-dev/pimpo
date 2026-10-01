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
  contexts: [], rules: [],
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
