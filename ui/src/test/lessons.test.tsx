import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Lessons } from '../pages/Lessons'
import { Inbox } from '../pages/Inbox'
import type { Lesson } from '../lib/api'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

const pref: Lesson = {
  id: 'l1', kind: 'preference', from: 'learned', title: 'Respostas curtas, em tópicos.', detail: 'pediu curto 3 vezes', change: 'Respostas curtas, em tópicos.', ref: 'f1',
  evidence: [{ kind: 'memory', to: '/memory' }, { kind: 'exploration', to: '/explorations/e1' }], state: 'proposed', created: '2026-09-29T09:00:00Z',
}
const task: Lesson = {
  id: 'l2', kind: 'routine', from: 'repeated', title: 'Me mande o clima de Lisboa', change: 'Me mande o clima de Lisboa todo dia', ref: 'e2',
  evidence: [{ kind: 'exploration', to: '/explorations/e2' }], state: 'proposed', created: '2026-09-29T08:00:00Z',
}

describe('Lessons', () => {
  it('shows each lesson with where it came from and applies nothing until a choice', async () => {
    const calls = mockFetch({
      '/api/lessons': { proposed: [pref, task], decided: [{ ...pref, id: 'l0', title: 'Nunca antes das 8', state: 'rejected', decided: '2026-09-28T09:00:00Z' }] },
      'POST /api/lessons/l1/accept': { ...pref, state: 'accepted', result: 'f1' },
      'POST /api/lessons/l1/reject': { ...pref, state: 'rejected' },
      'POST /api/lessons/l2/edit': (b: unknown) => ({ ...task, state: 'edited', edited: (b as { text: string }).text, result: 'e3' }),
    })
    wrap(<Lessons />, '/lessons')
    const card = (await screen.findByText('Respostas curtas, em tópicos.')).closest('div.p-4') as HTMLElement
    expect(within(card).getByText(/Preferência · aprendida dos seus pedidos/)).toBeInTheDocument()
    expect(within(card).getByRole('link', { name: 'A tarefa' })).toHaveAttribute('href', '/explorations/e1')
    expect(screen.getByText(/Viraria uma rotina que faz:/)).toBeInTheDocument()
    expect(screen.getByText('Nunca antes das 8')).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    await userEvent.click(within(card).getByRole('button', { name: 'Aceitar' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/lessons/l1/accept' && c.method === 'POST')).toBe(true))

    // Editing changes the text first; the request goes with the new words.
    const other = screen.getByText('Me mande o clima de Lisboa').closest('div.p-4') as HTMLElement
    await userEvent.click(within(other).getByRole('button', { name: 'Editar' }))
    const box = within(other).getByLabelText('Como você quer guardar')
    await userEvent.clear(box)
    await userEvent.type(box, 'Me mande o clima do Porto todo dia')
    await userEvent.click(within(other).getByRole('button', { name: 'Salvar e aceitar' }))
    await waitFor(() => {
      const edit = calls.find((c) => c.url === '/api/lessons/l2/edit')
      expect(edit?.body).toEqual({ text: 'Me mande o clima do Porto todo dia' })
    })
  })

  it('rejects without applying', async () => {
    const calls = mockFetch({ '/api/lessons': { proposed: [pref], decided: [] }, 'POST /api/lessons/l1/reject': { ...pref, state: 'rejected' } })
    wrap(<Lessons />, '/lessons')
    await userEvent.click(await screen.findByRole('button', { name: 'Rejeitar' }))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/lessons/l1/reject')).toBe(true))
    expect(calls.some((c) => c.url.endsWith('/accept'))).toBe(false)
  })

  it('says so when nothing waits', async () => {
    mockFetch({ '/api/lessons': { proposed: [], decided: [] } })
    wrap(<Lessons />, '/lessons')
    expect(await screen.findByText('Nenhuma lição esperando')).toBeInTheDocument()
  })

  it('shows in the inbox how many wait', async () => {
    mockFetch({
      '/api/media': [],
      '/api/needs': { total: 1, counts: { lesson: 1 }, items: [
        { kind: 'lesson', id: 'lessons', title: pref.title, created: pref.created, urgency: 0, count: 2, link: '/lessons', actions: ['open'] },
      ] },
    })
    wrap(<Routes><Route path="/inbox" element={<Inbox />} /><Route path="/lessons" element={<p>lessons page</p>} /></Routes>, '/inbox')
    expect(await screen.findByText('2 lições para revisar')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Revisar' }))
    expect(await screen.findByText('lessons page')).toBeInTheDocument()
  })
})
