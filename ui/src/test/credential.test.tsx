import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Credential } from '../pages/Credential'
import { Inbox } from '../pages/Inbox'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const request = { id: 'c1', connector: 'notion', field: 'token', routine: 'r1', asked: '2026-09-30T09:00:00Z', expires: '2026-10-07T09:00:00Z', title: 'Notion', label: 'Token da integração', description: 'Notion precisa da sua Token da integração.' }

describe('Private credential prompts', () => {
  it('saves the key through its own form and never shows it again', async () => {
    const calls = mockFetch({
      '/api/credentials/c1': request,
      'POST /api/credentials/c1': { saved: true, routine: 'r1' },
      'POST /api/routines/r1/run': { run: { id: 1 } },
    })
    wrap(<Routes><Route path="/credentials/:id" element={<Credential />} /><Route path="/routines/:id" element={<p>routine page</p>} /></Routes>, '/credentials/c1')
    expect(await screen.findByRole('heading', { name: 'Uma chave para Notion' })).toBeInTheDocument()
    const input = screen.getByLabelText('Token da integração')
    expect(input).toHaveAttribute('type', 'password')
    await userEvent.type(input, 'ntn_secretvalue')
    await userEvent.click(screen.getByRole('button', { name: 'Guardar em privado' }))
    expect(await screen.findByText('Guardado')).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'POST' && c.url === '/api/credentials/c1')?.body).toEqual({ value: 'ntn_secretvalue' })
    expect(screen.queryByDisplayValue('ntn_secretvalue')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Rodar a rotina de novo' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/routines/r1/run')).toBe(true))
    expect(await screen.findByText('routine page')).toBeInTheDocument()
  })

  it('lists open requests in the inbox with a link to the form', async () => {
    mockFetch({
      '/api/media': [],
      '/api/needs': { total: 1, counts: { credential_request: 1 }, items: [
        { kind: 'credential_request', id: 'c1', title: request.description, created: request.asked, urgency: 3, link: '/credentials/c1', actions: ['open'] },
      ] },
    })
    wrap(<Routes><Route path="/" element={<Inbox />} /><Route path="/credentials/:id" element={<p>form page</p>} /></Routes>)
    expect(await screen.findByText('Notion precisa da sua Token da integração.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Informar em privado' }))
    expect(await screen.findByText('form page')).toBeInTheDocument()
  })
})
