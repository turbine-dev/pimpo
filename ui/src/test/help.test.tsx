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
    await userEvent.click(screen.getAllByRole('button', { name: 'Salvar' }).at(-1)!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.url === '/api/settings')?.body).toMatchObject({ mute: ['task'], labs_off: ['memory_organize'] }))
  })
})

describe('Models', () => {
  it('adds an API model with its price and uses it for tasks', async () => {
    const settings = { zone: 'America/Sao_Paulo', locale: 'pt-BR', judge_backend: 'local', ollama_model: '', local_judge_url: '', explore_model: 'sonnet', compile_model: 'sonnet', judge_model: 'haiku' }
    const calls = mockFetch({
      '/api/settings': settings, 'PUT /api/settings': (b: unknown) => b, 'PUT /api/budget': {},
      '/api/state': { budget: { limit: 2, spent: 0 }, healthy: true }, '/api/pairing': { base: '', devices: [] }, '/api/remote': { tailscale: { state: 'off' }, lan: { on: false } },
      '/api/backup/cloud': { config: { kind: '', every: 'daily', keep: 7 }, has_keys: false, has_passphrase: false, google: { connected: false, drive: false } },
      '/api/models': { keys: { anthropic: false }, claude_code: false }, 'PUT /api/models/keys/anthropic': { set: true },
    })
    wrap(<Settings />)
    expect(await screen.findByText(/O Claude Code não está instalado/)).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Chave da API anthropic'), 'sk-ant-1')
    await userEvent.click(screen.getAllByRole('button', { name: 'Salvar' })[0])
    await waitFor(() => expect(calls.find((c) => c.url === '/api/models/keys/anthropic')?.body).toEqual({ key: 'sk-ant-1' }))
    const add = screen.getByRole('button', { name: /Adicionar/ })
    await userEvent.type(screen.getByLabelText('Nome do modelo'), 'claude-sonnet-5')
    expect(add).toBeDisabled()
    await userEvent.type(screen.getByLabelText('US$ entrada'), '3')
    await userEvent.type(screen.getByLabelText('US$ saída'), '15')
    await userEvent.click(add)
    await userEvent.selectOptions(screen.getByLabelText('Fazer tarefas e conversar'), 'anthropic:claude-sonnet-5')
    const saves = screen.getAllByRole('button', { name: 'Salvar' })
    await userEvent.click(saves[saves.length - 1])
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.url === '/api/settings')?.body).toMatchObject({
      explore_model: 'anthropic:claude-sonnet-5', models: [{ id: 'anthropic:claude-sonnet-5', price_in: 3, price_out: 15 }],
    }))
  })
})
