import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { McpExplore, McpManual } from '../components/McpServers'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const weather = { id: 'io.github.acme/weather-mcp', name: 'weather', title: 'weather-mcp', description: 'Forecasts', version: '1.0.0', kind: 'remote', url: 'https://mcp.acme.dev/mcp',
  inputs: [{ kind: 'header', name: 'X-Api-Key', secret: true, required: true }] }

describe('MCP servers', () => {
  it('finds a server, reviews its tools and installs the chosen ones', async () => {
    const calls = mockFetch({
      '/api/connectors/registry?q=&cursor=': { servers: [weather, { ...weather, id: 'x/docker', title: 'docker-only', kind: 'unsupported', inputs: [] }], next: '' },
      '/api/connectors/registry?q=weather&cursor=': { servers: [weather], next: '' },
      'POST /api/connectors/probe': { tools: [
        { tool: 'get-forecast', capability: 'weather.get_forecast', description: 'Forecast for a city', risk: 'read' },
        { tool: 'delete-station', capability: 'weather.delete_station', description: '', risk: 'irreversible' },
        { tool: 'rename-station', capability: 'weather.rename_station', description: '', risk: 'irreversible' },
      ] },
      'POST /api/connectors/add': { loaded: 1 },
    })
    wrap(<McpExplore />)
    expect(await screen.findByRole('button', { name: 'Adicionar docker-only' })).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Buscar no registro MCP'), 'weather')
    await waitFor(() => expect(calls.some((c) => c.url.includes('q=weather'))).toBe(true))
    await userEvent.click(await screen.findByRole('button', { name: 'Adicionar weather-mcp' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/roda no servidor do autor/)).toBeInTheDocument()
    const check = within(dialog).getByRole('button', { name: 'Ver as ferramentas' })
    expect(check).toBeDisabled()
    await userEvent.type(within(dialog).getByLabelText(/X-Api-Key/), 'k-123')
    await userEvent.click(check)
    await waitFor(() => expect(calls.find((c) => c.url === '/api/connectors/probe')?.body).toMatchObject({ name: 'weather', url: 'https://mcp.acme.dev/mcp', headers: { 'X-Api-Key': 'k-123' } }))
    await userEvent.click(await within(dialog).findByLabelText('Incluir delete-station'))
    await userEvent.selectOptions(within(dialog).getByLabelText('Risco de rename-station'), 'reversible')
    await userEvent.click(within(dialog).getByRole('button', { name: /Instalar 2 ferramentas/ }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/connectors/add')?.body).toMatchObject({
      name: 'weather', source: 'io.github.acme/weather-mcp@1.0.0', tools: { 'get-forecast': 'read', 'rename-station': 'reversible' },
    }))
    expect((calls.find((c) => c.url === '/api/connectors/add')?.body as { tools: object }).tools).not.toHaveProperty('delete-station')
    expect(await within(dialog).findByText(/weather instalado/)).toBeInTheDocument()
  })

  it('adds a server from a typed command', async () => {
    const calls = mockFetch({ 'POST /api/connectors/probe': { tools: [] } })
    wrap(<McpManual />)
    await userEvent.type(screen.getByLabelText('Comando para iniciar'), 'npx -y @acme/files@1.2.0 --root "/Users/me/My Docs"')
    await userEvent.type(screen.getByLabelText(/Variáveis/), 'FILES_TOKEN=abc')
    await userEvent.click(screen.getByRole('button', { name: 'Ver as ferramentas' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Ver as ferramentas' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/connectors/probe')?.body).toMatchObject({
      command: 'npx', args: ['-y', '@acme/files@1.2.0', '--root', '/Users/me/My Docs'], env: { FILES_TOKEN: 'abc' }, name: 'files',
    }))
  })
})
