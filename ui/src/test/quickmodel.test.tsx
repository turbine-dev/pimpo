import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QuickModel } from '../components/QuickModel'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const providers = [
  { id: 'anthropic', name: 'Anthropic', key_url: 'https://console.anthropic.com/settings/keys', needs_key: true },
  { id: 'openai', name: 'OpenAI', key_url: 'https://platform.openai.com/api-keys', needs_key: true },
  { id: 'ollama', name: 'Ollama', needs_key: false, local: true },
]
const nothing = { ollama: [], ollama_url: '', lmstudio: [], lmstudio_url: '' }

describe('Quick model setup', () => {
  it('sets everything up with only an API key', async () => {
    const calls = mockFetch({
      '/api/models/detect': nothing,
      '/api/models': { keys: {}, claude_code: false, providers },
      'POST /api/setup/model': { explore: 'openai:gpt-5', judge: 'openai:gpt-5-mini', text: 'ok', cost_usd: 0.0001, ms: 900 },
    })
    wrap(<QuickModel />)
    await userEvent.click(await screen.findByRole('radio', { name: /Uma chave de API/ }))
    expect(screen.queryByRole('radio', { name: /Claude Code/ })).toBeNull()
    await userEvent.selectOptions(screen.getByLabelText('Provedor'), 'openai')
    expect(screen.getByRole('link', { name: /Pegar uma chave da OpenAI/ })).toHaveAttribute('href', 'https://platform.openai.com/api-keys')
    expect(screen.queryByRole('option', { name: 'Ollama' })).toBeNull()
    await userEvent.type(screen.getByLabelText('Chave de API'), 'sk-proj-123456789')
    await userEvent.click(screen.getByRole('button', { name: 'Usar este' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/setup/model')?.body).toEqual({ kind: 'provider', provider: 'openai', key: 'sk-proj-123456789' }))
    expect(await screen.findByText(/gpt-5 aprende e compila tarefas, gpt-5-mini responde/)).toBeInTheDocument()
  })

  it('offers what the computer already has', async () => {
    const calls = mockFetch({
      '/api/models/detect': { ...nothing, claude_code: '/usr/local/bin/claude', ollama_up: true, ollama: [{ id: 'qwen3:8b', name: 'qwen3:8b', price_in: 0, price_out: 0, priced: true }] },
      '/api/models': { keys: {}, claude_code: true, providers },
      'POST /api/setup/model': { explore: 'ollama:qwen3:8b', judge: 'ollama:qwen3:8b', cost_usd: 0 },
    })
    wrap(<QuickModel />)
    expect(await screen.findByRole('radio', { name: /Claude Code/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('radio', { name: /Ollama/ }))
    expect(screen.getByLabelText('Modelo')).toHaveValue('qwen3:8b')
    await userEvent.click(screen.getByRole('button', { name: 'Usar este' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/setup/model')?.body).toEqual({ kind: 'ollama', model: 'qwen3:8b' }))
  })
})
