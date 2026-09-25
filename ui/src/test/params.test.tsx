import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { parseCron, RoutineSettings, toCron } from '../components/RoutineSettings'
import type { RoutineSummary } from '../lib/api'
import { mockFetch, wrap } from './helpers'

afterEach(() => vi.unstubAllGlobals())

describe('schedule', () => {
  it('reads and writes the common shapes, keeping anything else as cron', () => {
    for (const c of ['0 7 * * *', '30 6 * * 1-5', '0 8 * * 1,3,5', '15 9 5 * *', '0 */3 * * *', '*/10 8-18 * * *']) {
      expect(toCron(parseCron(c))).toBe(c)
    }
    expect(parseCron('30 6 * * 1-5').freq).toBe('weekdays')
    expect(parseCron('*/10 8-18 * * *').freq).toBe('custom')
  })
})

const routine: RoutineSummary = {
  id: 'clima', name: 'Clima', description: '', state: 'active', version: 1, runs: [], cost_month_usd: 0, capabilities: ['notify.send'],
  schedule: '0 7 * * *', default_schedule: '0 7 * * *',
  params: [
    { name: 'cidade', label: 'Cidade', type: 'location' },
    { name: 'chuva', label: 'Avisar chuva a partir de (%)', type: 'number' },
    { name: 'destinos', label: 'Onde avisar', type: 'destinations' },
  ],
  values: { cidade: { name: 'São Paulo', latitude: -23.55, longitude: -46.63 }, chuva: 50, destinos: [] },
}

describe('RoutineSettings', () => {
  it('changes the city, the time and the destinations without code', async () => {
    const calls = mockFetch({
      '/api/destinations': [{ id: 'telegram', label: 'Telegram @zodim_bot', kind: 'telegram', ready: true }, { id: 'bot:ab', label: 'Telegram @familia_bot → Família', kind: 'telegram', ready: true }, { id: 'whatsapp', label: 'WhatsApp', kind: 'whatsapp', ready: false }],
      '/api/geocode?q=Lisboa': [{ name: 'Lisboa', latitude: 38.72, longitude: -9.14, country: 'Portugal' }],
      'PUT /api/routines/clima/settings': routine,
    })
    wrap(<RoutineSettings s={routine} />)
    expect(screen.getByText('São Paulo')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Salvar ajustes' })).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Cidade: Procure uma cidade'), 'Lisboa')
    await userEvent.click(await screen.findByRole('option', { name: /Lisboa/ }))
    await userEvent.selectOptions(screen.getByLabelText('Quando'), 'weekdays')
    fireEvent.change(screen.getByLabelText('às'), { target: { value: '06:30' } })
    await userEvent.click(await screen.findByRole('button', { name: /familia_bot/ }))
    await userEvent.click(screen.getByRole('button', { name: /zodim_bot/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Salvar ajustes' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({
      schedule: '30 6 * * 1-5',
      params: { cidade: { name: 'Lisboa', latitude: 38.72, longitude: -9.14, country: 'Portugal' }, chuva: 50, destinos: ['bot:ab', 'telegram'] },
    }))
  })
})

describe('gallery updates', () => {
  it('offers the new version and what it lets you change', async () => {
    const old: RoutineSummary = { ...routine, params: [], values: {}, gallery_update: { name: 'Clima da manhã', description: 'Previsão do dia na cidade que você escolher.', settings: ['Cidade', 'Onde avisar'] } }
    const calls = mockFetch({ 'POST /api/routines/clima/update': routine })
    wrap(<RoutineSettings s={old} onRedo={() => {}} />)
    expect(screen.getByText(/Nova versão na galeria/)).toBeInTheDocument()
    expect(screen.getByText('Agora dá para ajustar: Cidade, Onde avisar.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Refazer com o agente' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Atualizar' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url === '/api/routines/clima/update')).toBe(true))
  })
})
