import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LocalModels } from '../components/LocalModels'
import { LocaleProvider } from '../lib/i18n'
import { mockFetch } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const MB = 1024 * 1024
const view = (jobs: unknown[] = []) => ({
  engine: { id: 'sherpa-onnx', kind: 'engine', name: 'sherpa-onnx', size: 19 * MB, installed: false },
  voices: [
    { id: 'kokoro-multi', kind: 'voice', name: 'Kokoro', about: 'Kokoro v1.0', languages: ['pt-BR', 'en-US'], size: 126 * MB, installed: false },
    { id: 'piper-pt_BR-faber-medium', kind: 'voice', name: 'Faber', about: 'Piper', languages: ['pt-BR'], size: 64 * MB, installed: true },
  ],
  jobs, free: 14 * 1024 * MB, memory: 16 * 1024 * MB,
  suggestions: [{ model: 'qwen3:4b', about: 'tarefas', size: 2600 * MB, min_ram: 8 * 1024 * MB }, { model: 'qwen3:14b', about: 'forte', size: 9300 * MB, min_ram: 32 * 1024 * MB }],
  ollama: { url: 'http://127.0.0.1:11434', up: true, models: [] },
})

describe('Local models', () => {
  it('asks with the size, then shows the download as it happens', async () => {
    let started = false
    const calls = mockFetch({
      '/api/local': () => view(started ? [{ id: 'dl-1', item: 'kokoro-multi', name: 'Kokoro', state: 'downloading', done: 73 * MB, total: 145 * MB, started: new Date().toISOString() }] : []),
      'POST /api/local/install/kokoro-multi': () => { started = true; return { id: 'dl-1' } },
    })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={qc}><LocaleProvider locale="pt"><LocalModels /></LocaleProvider></QueryClientProvider>)
    expect(await screen.findByText('Faber')).toBeInTheDocument()
    expect(screen.getByText('instalada')).toBeInTheDocument()
    expect(screen.queryByText('qwen3:14b')).not.toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Baixar' })[0])
    expect(await screen.findByText('Baixar Kokoro?')).toBeInTheDocument()
    expect(screen.getByText(/São cerca de 145 MB/)).toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Baixar' }).at(-1)!)
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/local/install/kokoro-multi')).toBe(true))
    expect((await screen.findAllByText('73 MB / 145 MB · 50%')).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('progressbar', { name: 'Kokoro' })[0]).toHaveAttribute('aria-valuenow', '50')
  })
})
