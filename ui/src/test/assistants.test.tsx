import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Assistants } from '../pages/Assistants'
import { Chat } from '../pages/Chat'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const caps = [
  { name: 'gmail.search', risk: 'read', signature: '', returns: '' },
  { name: 'gmail.send', risk: 'irreversible', signature: '', returns: '' },
  { name: 'todoist.add', risk: 'reversible', signature: '', returns: '' },
]

describe('Assistants', () => {
  it('creates an assistant limited to some tools', async () => {
    const calls = mockFetch({ '/api/assistants': [], '/api/capabilities': caps, 'PUT /api/assistants/financas': (b: unknown) => b })
    wrap(<Assistants />)
    await userEvent.click(await screen.findByRole('button', { name: /Novo assistente/ }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(within(dialog).getByLabelText('Nome'), 'Finanças')
    await userEvent.type(within(dialog).getByLabelText('Qual é o trabalho dele'), 'Cuida das contas.')
    const save = within(dialog).getByRole('button', { name: 'Salvar' })
    expect(save).toBeDisabled()
    await userEvent.click(within(dialog).getByRole('checkbox', { name: /gmail\.search/ }))
    await userEvent.click(within(dialog).getByRole('checkbox', { name: /todoist\.add/ }))
    await userEvent.click(save)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({
      id: 'financas', name: 'Finanças', emoji: '🤖', instructions: 'Cuida das contas.', capabilities: ['gmail.search', 'todoist.add'],
    }))
  })

  it('starts a chat with the chosen assistant', async () => {
    const calls = mockFetch({ '/api/chats': [], '/api/assistants': [{ id: 'financas', name: 'Finanças', emoji: '💰', instructions: '', capabilities: ['gmail.search'] }], 'POST /api/chats': { chat: 'c1', turn: 'e1' } })
    wrap(<Routes><Route path="/" element={<Chat />} /><Route path="/chat/:id" element={<Chat />} /></Routes>)
    await userEvent.click(await screen.findByRole('radio', { name: /Finanças/ }))
    await userEvent.type(screen.getByLabelText('Escreva uma mensagem…'), 'Quais contas vencem?{Enter}')
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'Quais contas vencem?', assistant: 'financas' }))
  })
})
