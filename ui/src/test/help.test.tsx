import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Help } from '../pages/Help'
import { Settings } from '../pages/Settings'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('Help and settings', () => {
  it('asks Zodim about itself', async () => {
    const calls = mockFetch({ 'POST /api/chats': { chat: 'c9', turn: 'e1' } })
    wrap(<Routes><Route path="/" element={<Help />} /><Route path="/chat/:id" element={<p>chat aberto</p>} /></Routes>)
    await userEvent.click(screen.getByRole('button', { name: 'Como abro o Zodim no celular?' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'Como abro o Zodim no celular?', assistant: '' }))
    expect(await screen.findByText('chat aberto')).toBeInTheDocument()
  })

  it('silences a kind of notice and turns off a lab feature', async () => {
    const settings = { zone: 'America/Sao_Paulo', locale: 'pt-BR', judge_backend: 'local', ollama_model: '', local_judge_url: '', explore_model: '', compile_model: '', judge_model: '' }
    const calls = mockFetch({
      '/api/settings': settings, 'PUT /api/settings': (b: unknown) => b, 'PUT /api/budget': {},
      '/api/state': { budget: { limit: 2, spent: 0 }, healthy: true }, '/api/pairing': { base: '', devices: [] }, '/api/remote': { tailscale: { state: 'off' }, lan: { on: false } },
      '/api/backup/cloud': { config: { kind: '', every: 'daily', keep: 7 }, has_keys: false, has_passphrase: false, google: { connected: false, drive: false } },
    })
    wrap(<Settings />)
    expect(await screen.findByRole('switch', { name: 'Pedidos de aprovação' })).toBeDisabled()
    await userEvent.click(screen.getByRole('switch', { name: 'Resultado de tarefas' }))
    await userEvent.click(screen.getByRole('switch', { name: 'Organizar a memória toda noite' }))
    await userEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.url === '/api/settings')?.body).toMatchObject({ mute: ['task'], labs_off: ['memory_organize'] }))
  })
})
