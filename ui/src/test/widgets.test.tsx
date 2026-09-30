import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { WidgetSnap, WidgetView } from '../lib/api'
import { LocaleProvider } from '../lib/i18n'
import { formatValue, WidgetBody, WidgetCard } from '../components/widgets/Widget'
import { mockFetch, wrap } from './helpers'
import { Dashboards } from '../pages/Dashboards'
import { MakeWidget } from '../components/MakeWidget'
import { FloatingWidget } from '../pages/FloatingWidget'
import { canPinToHome, pinToHome } from '../components/widgets/homescreen'

const view = (snapshot: WidgetSnap, extra: Partial<WidgetView> = {}): WidgetView => ({
  id: 'w_' + snapshot.kind, source: 'routine', kind: snapshot.kind, title: snapshot.title, snapshot, updated: new Date().toISOString(), mine: true, ...extra,
})

const draw = (w: WidgetView) => wrap(<LocaleProvider locale="pt"><WidgetCard w={w}><WidgetBody w={w} size="wide" /></WidgetCard></LocaleProvider>)

describe('widgets', () => {
  it('draws every kind', () => {
    draw(view({ kind: 'metric', title: 'Dólar', value: 5.18, unit: 'BRL', trend: -0.8 }, { history: [{ t: '', v: 5.1 }, { t: '', v: 5.2 }] }))
    expect(screen.getByText('Dólar')).toBeInTheDocument()
    expect(screen.getByText(/5,18/)).toBeInTheDocument()
    expect(screen.getByText('0,8%')).toBeInTheDocument()
    draw(view({ kind: 'progress', title: 'Meta', value: 17000, goal: 22000, unit: 'BRL' }))
    expect(screen.getByText('77%')).toBeInTheDocument()
    draw(view({ kind: 'status', title: 'Backup', status: 'alert', subtitle: 'falhou às 03:12' }))
    expect(screen.getByText('Alerta')).toBeInTheDocument()
    draw(view({ kind: 'list', title: 'E-mails', items: [{ title: 'Contrato', badge: 'urgente', status: 'alert' }] }))
    expect(screen.getByText('urgente')).toBeInTheDocument()
    draw(view({ kind: 'table', title: 'Estoque', columns: ['Produto', 'Qtd'], rows: [['Caneca', '4']] }))
    expect(screen.getByText('Caneca')).toBeInTheDocument()
    draw(view({ kind: 'chart', title: 'Vendas', chart: 'donut', series: [{ points: [{ label: 'Site', y: 3 }, { label: 'Loja', y: 1 }] }] }))
    expect(screen.getByText('75%')).toBeInTheDocument()
    draw(view({ kind: 'chart', title: 'Pedidos', chart: 'bar', series: [{ points: [{ label: 'S1', y: 3 }, { label: 'S2', y: 5 }] }] }))
    expect(screen.getByText('S2')).toBeInTheDocument()
    draw(view({ kind: 'text', title: 'Tempo', text: 'Sol entre nuvens' }))
    expect(screen.getByText('Sol entre nuvens')).toBeInTheDocument()
  })

  it('words built-in widgets in the person\'s language', () => {
    draw(view({ kind: 'text', title: 'Reminders', text: '-', meta: { empty: 'reminders' } }, { id: 'builtin:reminders', source: 'builtin' }))
    expect(screen.getByText('Lembretes')).toBeInTheDocument()
    expect(screen.getByText('Nenhum lembrete marcado.')).toBeInTheDocument()
  })

  it('formats money, percent and big numbers', () => {
    expect(formatValue(1843.2, 'USD')).toMatch(/1[.,]843/)
    expect(formatValue(12.5, '%')).toMatch(/12[.,]5%/)
    expect(formatValue(1250000)).toMatch(/1[.,]3|1[.,]2/)
  })

  it('opens a dashboard in tabs', async () => {
    mockFetch({
      '/api/dashboards': [{ id: 'd1', name: 'Casa', emoji: '🏠', position: 0, layout: [{ id: 'w_metric', x: 0, y: 0, w: 4, h: 3 }], shared: false, mine: true, updated: '' },
        { id: 'd2', name: 'Loja', emoji: '🛒', position: 1, layout: [], shared: true, mine: false, updated: '' }],
      '/api/dashboards/d1/widgets': { w_metric: view({ kind: 'metric', title: 'Saldo', value: 10, unit: 'BRL' }) },
    })
    wrap(<LocaleProvider locale="pt"><Dashboards /></LocaleProvider>, '/dashboards')
    expect(await screen.findByRole('tab', { name: /Casa/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: /Loja/ })).toBeInTheDocument()
    expect(await screen.findByText('Saldo')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Editar/ })).toBeInTheDocument()
  })

  it('turns a routine into a widget of the chosen kind', async () => {
    const calls = mockFetch({ 'POST /api/routines/dolar/widget': { exploration: 'e1' } })
    wrap(<LocaleProvider locale="pt"><MakeWidget id="dolar" shows={false} /></LocaleProvider>)
    await userEvent.click(screen.getByRole('button', { name: /Transformar em widget/ }))
    await userEvent.click(screen.getByRole('radio', { name: /Gráfico/ }))
    await userEvent.click(screen.getByRole('button', { name: /Refazer com widget/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ kind: 'chart' }))
  })

  it('links to the dashboards when the routine already shows a widget', () => {
    wrap(<LocaleProvider locale="pt"><MakeWidget id="dolar" shows /></LocaleProvider>)
    expect(screen.getByRole('link', { name: /Ver nos painéis/ })).toHaveAttribute('href', '/dashboards')
  })

  it('shows one widget alone in a floating window', async () => {
    mockFetch({ '/api/widgets/w_metric': view({ kind: 'metric', title: 'Dólar', value: 5.18, unit: 'BRL' }) })
    wrap(<LocaleProvider locale="pt"><FloatingWidget id="w_metric" /></LocaleProvider>, '/float/w_metric')
    expect(await screen.findByText('Dólar')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Abrir os painéis' })).toBeInTheDocument()
  })

  it('offers floating a widget only in the desktop app', async () => {
    const w = view({ kind: 'metric', title: 'Saldo', value: 10, unit: 'BRL' })
    const onFloat = vi.fn()
    wrap(<LocaleProvider locale="pt"><WidgetCard w={w} onFloat={onFloat} floating={false}><WidgetBody w={w} size="small" /></WidgetCard></LocaleProvider>)
    await userEvent.click(screen.getByRole('button', { name: 'Opções do widget' }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Flutuar na área de trabalho/ }))
    expect(onFloat).toHaveBeenCalledWith(true)
  })

  it('puts a widget on the Android home screen with the phone\'s widget key', async () => {
    expect(canPinToHome()).toBe(false)
    const bridge = { available: () => true, setup: vi.fn(() => true), pin: vi.fn(() => true) }
    Object.assign(window, { PimpoWidgets: bridge })
    const calls = mockFetch({ 'POST /api/phone/widgets': { key: 'wk_abc', pins: ['w_metric'] } })
    expect(canPinToHome()).toBe(true)
    expect(await pinToHome('w_metric')).toBe(true)
    expect(calls[0].body).toEqual({ pin: 'w_metric' })
    expect(bridge.setup).toHaveBeenCalledWith('wk_abc')
    expect(bridge.pin).toHaveBeenCalledWith('w_metric')
    delete (window as { PimpoWidgets?: unknown }).PimpoWidgets
  })

  it('offers the home screen in the widget menu when the phone can', async () => {
    const w = view({ kind: 'metric', title: 'Saldo', value: 10, unit: 'BRL' })
    const onPinHome = vi.fn()
    wrap(<LocaleProvider locale="pt"><WidgetCard w={w} onPinHome={onPinHome}><WidgetBody w={w} size="small" /></WidgetCard></LocaleProvider>)
    await userEvent.click(screen.getByRole('button', { name: 'Opções do widget' }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Adicionar à tela inicial/ }))
    expect(onPinHome).toHaveBeenCalled()
  })
})
