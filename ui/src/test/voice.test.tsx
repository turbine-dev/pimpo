import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { Chat } from '../pages/Chat'
import { mockFetch, wrap } from './helpers'

class FakeRecognition {
  lang = ''
  interimResults = false
  continuous = false
  onresult: ((e: unknown) => void) | null = null
  onend: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  start() {
    setTimeout(() => {
      const partial = Object.assign([{ transcript: 'o que tenho' }], { isFinal: false })
      this.onresult?.({ results: [partial] })
      const full = Object.assign([{ transcript: 'o que tenho amanhã' }], { isFinal: true })
      this.onresult?.({ results: [full] })
      this.onend?.()
    }, 10)
  }
  stop() {}
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('voice in the chat', () => {
  it('dictates a message and reads the answer aloud', async () => {
    const spoken: string[] = []
    vi.stubGlobal('webkitSpeechRecognition', FakeRecognition)
    vi.stubGlobal('speechSynthesis', { cancel: () => {}, speak: (u: { text: string }) => spoken.push(u.text) })
    vi.stubGlobal('SpeechSynthesisUtterance', class { lang = ''; text: string; constructor(text: string) { this.text = text } })
    let state = 'running'
    const calls = mockFetch({
      '/api/chats': [],
      'POST /api/chats': { chat: 'c1', turn: 'e1' },
      '/api/chats/c1': () => ({ chat: { id: 'c1', title: 'x' }, turns: [{ id: 'e1', request: 'o que tenho amanhã', state, summary: state === 'ready' ? 'Amanhã você tem dentista às 10h.' : '', cost_usd: 0, created_at: '', actions: [] }] }),
    })
    wrap(<Routes><Route path="/" element={<Chat />} /><Route path="/chat/:id" element={<Chat />} /></Routes>)
    await userEvent.click(await screen.findByRole('button', { name: 'Falar' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ text: 'o que tenho amanhã', assistant: '' }))
    state = 'ready'
    await waitFor(() => expect(spoken).toEqual(['Amanhã você tem dentista às 10h.']), { timeout: 4000 })
    await userEvent.click(await screen.findByRole('button', { name: 'Ouvir' }))
    expect(spoken).toHaveLength(2)
  })
})
