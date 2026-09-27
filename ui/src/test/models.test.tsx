import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModelSetup } from '../components/ModelSetup'
import { LocaleProvider } from '../lib/i18n'
import { mockFetch } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const providers = [
  { id: 'anthropic', name: 'Anthropic', key_url: 'https://console.anthropic.com/settings/keys', needs_key: true },
  { id: 'ollama', name: 'Ollama', needs_key: false, local: true },
  { id: 'lmstudio', name: 'LM Studio', needs_key: false, local: true },
  { id: 'custom', name: 'OpenAI-compatible', needs_key: false },
]
const settings = { zone: 'UTC', locale: 'pt-BR', judge_backend: 'local', ollama_model: '', local_judge_url: '', explore_model: 'sonnet', compile_model: 'sonnet', judge_model: 'haiku', models: [] }

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={qc}><LocaleProvider locale="pt"><ModelSetup /></LocaleProvider></QueryClientProvider>)
}

describe('Model setup', () => {
  it('finds what is here, and adds a provider model only after it answers', async () => {
    let saved: Record<string, unknown> | undefined
    const calls = mockFetch({
      '/api/settings': settings,
      'PUT /api/settings': (b: unknown) => { saved = b as Record<string, unknown>; return b },
      '/api/models': { keys: { anthropic: true }, claude_code: true, providers },
      '/api/models/detect': { claude_code: '2.1.0 (Claude Code)', ollama: [{ id: 'qwen3:8b', name: 'qwen3:8b', price_in: 0, price_out: 0, priced: true, free: true }], ollama_url: 'http://127.0.0.1:11434', lmstudio: [], lmstudio_url: 'http://127.0.0.1:1234' },
      '/api/models/catalog/anthropic': [
        { id: 'claude-sonnet-5', name: 'Claude Sonnet 5', price_in: 3, price_out: 15, priced: true },
        { id: 'claude-mystery', name: 'claude-mystery', price_in: 0, price_out: 0, priced: false },
      ],
      'POST /api/models/test': { ok: true, text: 'ok', cost_usd: 0.0001, ms: 640 },
    })
    wrap()
    expect(await screen.findByText(/Encontrado \(2.1.0/)).toBeInTheDocument()
    expect(screen.getByText(/1 modelo em http:\/\/127.0.0.1:11434/)).toBeInTheDocument()
    expect(screen.getByText(/Não está rodando em http:\/\/127.0.0.1:1234/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Anthropic/ }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('Claude Sonnet 5')).toBeInTheDocument()
    expect(within(dialog).getByText(/\$3 \/ \$15/)).toBeInTheDocument()
    // No price in the catalog: the owner must give one before it can run.
    const rows = within(dialog).getAllByRole('listitem')
    const mystery = rows.find((r) => r.textContent?.includes('claude-mystery'))!
    expect(within(mystery).getByRole('button', { name: /Testar e usar/ })).toBeDisabled()

    const sonnet = rows.find((r) => r.textContent?.includes('Claude Sonnet 5'))!
    await userEvent.click(within(sonnet).getByRole('button', { name: /Testar e usar/ }))
    expect(await within(dialog).findByText(/Respondeu em 640 ms/)).toBeInTheDocument()
    await waitFor(() => expect(saved?.models).toEqual([{ id: 'anthropic:claude-sonnet-5', price_in: 3, price_out: 15 }]))
    expect(calls.find((c) => c.url === '/api/models/test')?.body).toEqual({ id: 'anthropic:claude-sonnet-5', price_in: 3, price_out: 15 })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Tudo' }))
    await waitFor(() => expect(saved?.explore_model).toBe('anthropic:claude-sonnet-5'))
  })

  it('explains a failed test in plain words and adds nothing', async () => {
    let saved = false
    mockFetch({
      '/api/settings': settings,
      'PUT /api/settings': (b: unknown) => { saved = true; return b },
      '/api/models': { keys: { anthropic: true }, claude_code: false, providers },
      '/api/models/detect': { ollama: [], ollama_url: '', lmstudio: [], lmstudio_url: '' },
      '/api/models/catalog/anthropic': [{ id: 'claude-sonnet-5', name: 'Claude Sonnet 5', price_in: 3, price_out: 15, priced: true }],
      'POST /api/models/test': { ok: false, problem: 'credits', error: 'anthropic answered 402: insufficient credits' },
    })
    wrap()
    await userEvent.click(await screen.findByRole('button', { name: /Anthropic/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(await within(dialog).findByRole('button', { name: /Testar e usar/ }))
    expect(await within(dialog).findByText(/A conta ficou sem créditos/)).toBeInTheDocument()
    expect(saved).toBe(false)
  })
})
