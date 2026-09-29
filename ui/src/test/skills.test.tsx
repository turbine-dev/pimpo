import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Skills } from '../components/Skills'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const preview = { token: 'abcdef012345abcdef012345', exists: false, skill: { id: 'inbox-zero', name: 'Inbox Zero', description: 'Sort the inbox', body: 'Archive newsletters.', files: ['SKILL.md', 'scripts/run.py'], scripts: ['scripts/run.py'], suggested: ['gmail.search'], unsupported: [], secrets: [] } }

describe('Skills', () => {
  it('reads a skill from GitHub, shows what it asks and installs it with the chosen capabilities', async () => {
    const calls = mockFetch({
      '/api/skills': [],
      '/api/capabilities': [{ name: 'gmail.search', risk: 'read', signature: '', returns: '' }, { name: 'gmail.send', risk: 'irreversible', signature: '', returns: '' }],
      'POST /api/skills/preview': preview,
      'POST /api/skills/install': { id: 'inbox-zero', name: 'Inbox Zero', description: '', source: '', capabilities: ['gmail.search'], scripts: [], unsupported: [], installed: '' },
    })
    wrap(<Skills />)
    await userEvent.type(screen.getByLabelText(/Link do GitHub/), 'https://github.com/acme/skills/tree/main/inbox-zero')
    await userEvent.click(screen.getByRole('button', { name: 'Ler' }))
    expect(await screen.findByText('Inbox Zero')).toBeInTheDocument()
    expect(screen.getByText(/Scripts que não vão rodar: scripts\/run.py/)).toBeInTheDocument()
    expect(screen.getByLabelText('gmail.search')).toBeChecked()
    expect(screen.getByLabelText('gmail.send')).not.toBeChecked()
    await userEvent.click(screen.getByRole('button', { name: 'Instalar com 1 capacidade' }))
    await waitFor(() => expect(calls.find((c) => c.url === '/api/skills/install')?.body).toEqual({ token: preview.token, capabilities: ['gmail.search'] }))
  })
})
