import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'

export function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

export type Call = { url: string; method: string; body?: unknown }

// mockFetch answers "METHOD url" or "url" keys; functions receive the body.
export function mockFetch(routes: Record<string, unknown | ((body: unknown) => unknown)>) {
  const calls: Call[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ url, method, body })
    let out: unknown = routes[`${method} ${url}`] ?? routes[url] ?? null
    if (typeof out === 'function') out = (out as (b: unknown) => unknown)(body)
    return new Response(JSON.stringify(out), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  return calls
}
