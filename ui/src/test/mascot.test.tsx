import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Cat, Mascot, mascotOn, setMascotOn } from '../components/Mascot'
import { LocaleProvider } from '../lib/i18n'
import { mockFetch } from './helpers'

afterEach(() => { vi.unstubAllGlobals(); localStorage.clear() })

function send(type: string, data: Record<string, unknown>) {
  act(() => { window.dispatchEvent(new CustomEvent('pimpo:event', { detail: { id: 1, ts: '', type, actor: 'system', data, hash: '' } })) })
}

function wrap(onOpen = vi.fn(), on = true) {
  localStorage.setItem('pimpo.mascot', on ? 'on' : 'off')
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={qc}><LocaleProvider locale="pt"><Mascot onOpen={onOpen} /></LocaleProvider></QueryClientProvider>)
  return onOpen
}

describe('Pimpo', () => {
  it('is off until turned on', () => {
    mockFetch({ '/api/state': null })
    wrap(vi.fn(), false)
    expect(screen.queryByRole('button', { name: 'Pimpo, o mascote' })).not.toBeInTheDocument()
    localStorage.clear()
    expect(mascotOn()).toBe(false)
  })

  it('draws its games', () => {
    const { container } = render(<Cat mood="idle" petting={false} play="butterfly" />)
    expect(container.querySelector('.pimpo-butterfly')).not.toBeNull()
    expect(container.querySelector('.pimpo-paw')).not.toBeNull()
  })

  it('raises an approval in a bubble and opens it', async () => {
    mockFetch({ '/api/state': { approvals: 1, awaiting: 0, broken: 0, budget: { spent: 0, limit: 1 } } })
    const onOpen = wrap()
    send('approval.requested', {})
    send('notice.sent', { text: 'limpeza quer apagar um e-mail', actions: [{ label: 'Permitir', data: 'approve:9' }] })
    expect(screen.getByRole('status')).toHaveTextContent('Precisa de você')
    expect(screen.getByRole('status')).toHaveTextContent('limpeza quer apagar um e-mail')
    await userEvent.click(screen.getByRole('button', { name: 'Ver' }))
    expect(onOpen).toHaveBeenCalledWith('/inbox')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('stays quiet when muted, and can be hidden', async () => {
    mockFetch({ '/api/state': { approvals: 0, awaiting: 0, broken: 0, budget: { spent: 0, limit: 1 } } })
    wrap()
    await userEvent.click(screen.getByRole('button', { name: 'Pimpo, o mascote' }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Silenciar por 1 hora/ }))
    send('notice.sent', { text: 'Pronto: resumo', kind: 'task' })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Pimpo, o mascote' }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Esconder o Pimpo/ }))
    expect(screen.queryByRole('button', { name: 'Pimpo, o mascote' })).not.toBeInTheDocument()
    act(() => setMascotOn(true))
    expect(screen.getByRole('button', { name: 'Pimpo, o mascote' })).toBeInTheDocument()
  })
})
