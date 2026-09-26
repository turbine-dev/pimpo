import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CloudBackup } from '../components/CloudBackup'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const off = { config: { kind: '', every: 'daily', keep: 7 }, has_keys: false, has_passphrase: false, google: { connected: true, drive: false } }

describe('CloudBackup', () => {
  it('sets up S3, backs up now and restores a copy', async () => {
    let state: Record<string, unknown> = off
    const on = (body: unknown) => (state = { ...off, config: body, has_keys: true, has_passphrase: true, last: { at: new Date().toISOString(), ok: true, name: 'pimpo-20260925-100000.pimpo', size: 3_300_000 } })
    const calls = mockFetch({
      '/api/backup/cloud': () => state,
      'PUT /api/backup/cloud': on,
      'POST /api/backup/cloud/run': { ok: true },
      '/api/backup/cloud/files': [{ name: 'pimpo-20260925-100000.pimpo', size: 3_300_000, modified: '2026-09-25T10:00:05Z' }],
      'POST /api/backup/cloud/restore': { secrets: 4, restart: true },
    })
    wrap(<CloudBackup />)
    await userEvent.click(await screen.findByRole('radio', { name: 'Amazon S3 ou compatível' }))
    const save = screen.getByRole('button', { name: /Salvar e testar/ })
    expect(save).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Bucket'), 'minha-casa')
    await userEvent.type(screen.getByLabelText('Região'), 'sa-east-1')
    await userEvent.type(screen.getByLabelText('Access key'), 'AKIA1')
    await userEvent.type(screen.getByLabelText('Secret key'), 'shh')
    await userEvent.type(screen.getByLabelText('Senha dos backups'), 'correct horse')
    await userEvent.click(save)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ kind: 's3', bucket: 'minha-casa', region: 'sa-east-1', access_key: 'AKIA1', secret_key: 'shh', passphrase: 'correct horse', every: 'daily', keep: 7 }))
    expect(await screen.findByText(/Último backup/)).toHaveTextContent('3.1 MB')

    await userEvent.click(screen.getByRole('button', { name: 'Fazer backup agora' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/backup/cloud/run')).toBe(true))

    await userEvent.click(screen.getByRole('button', { name: 'Ver backups' }))
    await userEvent.click(await screen.findByRole('button', { name: /Restaurar o backup de/ }))
    await userEvent.click(screen.getAllByRole('button', { name: /Restaurar o backup de/ }).at(-1)!)
    await waitFor(() => expect(calls.find((c) => c.url === '/api/backup/cloud/restore')?.body).toEqual({ name: 'pimpo-20260925-100000.pimpo' }))
    expect(await screen.findByText(/Arquivo conferido \(4 chaves\)/)).toBeInTheDocument()
  })

  it('asks to reconnect Google before using Drive', async () => {
    mockFetch({ '/api/backup/cloud': off })
    wrap(<CloudBackup />)
    await userEvent.click(await screen.findByRole('radio', { name: 'Google Drive' }))
    expect(screen.getByText(/Reconecte o Google em Conexões e permita o Drive/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Abrir Conexões' })).toHaveAttribute('href', '/connections')
    await userEvent.type(screen.getByLabelText('Senha dos backups'), 'correct horse')
    expect(screen.getByRole('button', { name: /Salvar e testar/ })).toBeDisabled()
  })
})
