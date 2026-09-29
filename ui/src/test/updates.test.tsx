import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UpdateBanner, UpdateCard } from '../components/Updates'
import { wrap } from './helpers'

const assign = vi.fn()
beforeEach(() => {
  ;(window as { __PIMPO_DESKTOP__?: string }).__PIMPO_DESKTOP__ = 'mac'
  localStorage.clear()
  assign.mockReset()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
})
afterEach(() => {
  delete (window as { __PIMPO_DESKTOP__?: string }).__PIMPO_DESKTOP__
})

const tell = (s: object) => act(() => {
  localStorage.setItem('pimpo.update', JSON.stringify(s))
  window.dispatchEvent(new Event('pimpo:update'))
})

describe('Desktop updates', () => {
  it('shows a found update and asks the app to install it', async () => {
    wrap(<><UpdateBanner /><UpdateCard /></>)
    expect(screen.queryByText(/está pronto/)).toBeNull()
    tell({ current: '0.6.0', found: '0.7.0', notes: 'Novidades', beta: false, previous: null, busy: false, error: '' })
    expect(screen.getAllByText('O Pimpo 0.7.0 está pronto.')).toHaveLength(2)
    await userEvent.click(screen.getAllByRole('button', { name: 'Atualizar e reiniciar' })[0])
    expect(assign).toHaveBeenCalledWith('/desktop/update?do=install')
    await userEvent.click(screen.getByRole('button', { name: 'Fechar' }))
    expect(screen.getAllByText('O Pimpo 0.7.0 está pronto.')).toHaveLength(1)
  })

  it('switches to beta and goes back only after confirming', async () => {
    wrap(<UpdateCard />)
    tell({ current: '0.7.0', found: null, beta: false, previous: '0.6.0', busy: false, error: '' })
    await userEvent.click(screen.getByRole('switch', { name: 'Versões beta' }))
    expect(assign).toHaveBeenLastCalledWith('/desktop/update?do=beta')
    await userEvent.click(screen.getByRole('button', { name: /Voltar para a 0.6.0/ }))
    expect(assign).toHaveBeenCalledTimes(1)
    await userEvent.click(screen.getByRole('button', { name: 'Voltar' }))
    expect(assign).toHaveBeenLastCalledWith('/desktop/update?do=rollback')
  })

  it('shows nothing outside the desktop app', () => {
    delete (window as { __PIMPO_DESKTOP__?: string }).__PIMPO_DESKTOP__
    localStorage.setItem('pimpo.update', JSON.stringify({ current: '0.6.0', found: '0.7.0', beta: false, busy: false, error: '' }))
    const { container } = wrap(<><UpdateBanner /><UpdateCard /></>)
    expect(container).toBeEmptyDOMElement()
  })
})
