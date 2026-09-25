import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { LocaleProvider, translate, type Locale } from '../lib/i18n'
import { en } from '../lib/locales/en'
import { pt } from '../lib/locales/pt'
import { Routines } from '../pages/Routines'
import { Settings } from '../pages/Settings'
import { mockFetch } from './helpers'

afterEach(() => vi.unstubAllGlobals())

function wrapIn(ui: ReactNode, locale?: Locale) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <LocaleProvider locale={locale}>
        <MemoryRouter>{ui}</MemoryRouter>
      </LocaleProvider>
    </QueryClientProvider>,
  )
}

const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()

const settings = { zone: 'America/Sao_Paulo', locale: 'en-US', judge_backend: 'local', ollama_model: '', local_judge_url: 'http://127.0.0.1:8765', explore_model: '', compile_model: '', judge_model: '' }
const empty = { '/api/routines': [], '/api/explorations?state=running,ready,compiling': [], '/api/explorations?state=imported': [] }

describe('dictionaries', () => {
  it('have exactly the same keys in Portuguese and English', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(pt).sort())
  })

  it('use the same {slots} in both languages', () => {
    for (const k of Object.keys(pt) as (keyof typeof pt)[]) expect([k, slots(en[k])]).toEqual([k, slots(pt[k])])
  })

  it('interpolates and picks the plural form', () => {
    expect(translate('en', 'rules.testSome', { count: 1 })).toBe('Over the last week, this rule would have applied to 1 action.')
    expect(translate('en', 'rules.testSome', { count: 3 })).toBe('Over the last week, this rule would have applied to 3 actions.')
    expect(translate('pt', 'inbox.didntRun', { name: 'Resumo' })).toBe('Resumo não rodou')
  })
})

describe('English', () => {
  it('shows the Routines empty state in English', async () => {
    mockFetch(empty)
    wrapIn(<Routines onNew={() => {}} />, 'en')
    expect(await screen.findByText('No routines yet')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Routines' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Ask for your first task' })).toBeInTheDocument()
    expect(screen.queryByText('Nenhuma rotina ainda')).not.toBeInTheDocument()
  })

  it('shows Settings in English, with the language select', async () => {
    mockFetch({ '/api/settings': settings, '/api/state': { budget: { spent: 0, limit: 2 } }, '/api/pairing': { base: '', devices: [] }, '/api/protection': null })
    wrapIn(<Settings />, 'en')
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
    expect(screen.getByText('Daily spending limit')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Privacy' })).toBeInTheDocument()
    expect(screen.getByLabelText('Daily limit in dollars')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Language for the app and messages' })).toHaveValue('en-US')
    await userEvent.clear(screen.getByLabelText('Daily limit in dollars'))
    await userEvent.type(screen.getByLabelText('Daily limit in dollars'), '5')
    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  })

  it('prefers the saved locale over the browser and sets <html lang>', async () => {
    // jsdom reports navigator.language as en-US; the saved setting wins once loaded.
    mockFetch({ ...empty, '/api/settings': { ...settings, locale: 'pt-BR' } })
    wrapIn(<Routines onNew={() => {}} />)
    expect(await screen.findByText('Nenhuma rotina ainda')).toBeInTheDocument()
    await waitFor(() => expect(document.documentElement.lang).toBe('pt-BR'))
  })
})
