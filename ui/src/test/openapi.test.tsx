import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OpenApiImport } from '../components/OpenApiImport'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const preview = {
  title: 'Swagger Petstore', description: 'Pets', base: 'https://petstore.example/api/v3', unsupported: [{ id: 'upload_file', why: 'uploads a file' }],
  keys: [{ name: 'API_KEY', description: 'api_key' }],
  operations: [
    { id: 'find_pets', method: 'GET', path: '/pet/findByStatus', summary: 'Finds pets by status', risk: 'read' },
    { id: 'delete_pet', method: 'DELETE', path: '/pet/{petId}', summary: 'Deletes a pet', risk: 'irreversible' },
    { id: 'search_pets', method: 'POST', path: '/pet/search', summary: 'Search', risk: 'irreversible' },
  ],
}

describe('OpenAPI import', () => {
  it('reads a description, chooses operations and installs them', async () => {
    const calls = mockFetch({ 'POST /api/connectors/openapi/preview': preview, 'POST /api/connectors/openapi/add': { loaded: 1 } })
    wrap(<OpenApiImport />)
    await userEvent.type(screen.getByLabelText(/Endereço da descrição/), 'https://petstore.example/openapi.json')
    await userEvent.click(screen.getByRole('button', { name: 'Ler a descrição' }))
    expect(await screen.findByText('Swagger Petstore')).toBeInTheDocument()
    expect(screen.getByText(/petstore\.example\/api\/v3 · operações: 3/)).toBeInTheDocument()
    expect(screen.getByLabelText('Incluir find_pets')).toBeChecked()
    expect(screen.getByLabelText('Incluir delete_pet')).not.toBeChecked()
    expect(screen.getByDisplayValue('swaggerpetstore')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText(/API_KEY/), 'k1')
    await userEvent.click(screen.getByLabelText('Incluir search_pets'))
    await userEvent.selectOptions(screen.getByLabelText('Risco de search_pets'), 'read')
    const name = screen.getByDisplayValue('swaggerpetstore')
    await userEvent.clear(name)
    await userEvent.type(name, 'pets')
    await userEvent.click(screen.getByRole('button', { name: /Instalar 2 operações/ }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/connectors/openapi/add')?.body).toEqual({
      url: 'https://petstore.example/openapi.json', header: '', name: 'pets', operations: { find_pets: 'read', search_pets: 'read' }, keys: { API_KEY: 'k1' },
    }))
    expect(await screen.findByText(/pets instalado/)).toBeInTheDocument()
  })

  it('adds a key the description does not declare', async () => {
    const calls = mockFetch({ 'POST /api/connectors/openapi/preview': { ...preview, keys: [] } })
    wrap(<OpenApiImport />)
    await userEvent.type(screen.getByLabelText(/Endereço da descrição/), 'https://api.example/spec.yaml')
    await userEvent.click(screen.getByRole('button', { name: 'Ler a descrição' }))
    await userEvent.click(await screen.findByText(/pede uma chave que a descrição não declara/))
    await userEvent.click(screen.getByRole('button', { name: 'Adicionar' }))
    await waitFor(() => expect(calls.filter((c) => c.url === '/api/connectors/openapi/preview').at(-1)?.body).toMatchObject({ url: 'https://api.example/spec.yaml', header: 'Authorization' }))
  })
})
