import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, MemoryRouter } from 'react-router-dom'
import App from './App'
import { api, ApiError } from './lib/api'
import { LocaleProvider, localeFrom } from './lib/i18n'
import { loadTelegram, MiniApp, telegramLanguage } from './pages/MiniApp'
import './index.css'

// Inside the desktop app the window cannot open new ones: links meant for a
// new tab go to the computer's browser through the local server.
const desktop = (window as { __PIMPO_DESKTOP__?: string }).__PIMPO_DESKTOP__
if (desktop) {
  document.documentElement.dataset.desktop = desktop
  document.addEventListener('click', (e) => {
    const a = (e.target as Element | null)?.closest?.('a[target="_blank"]') as HTMLAnchorElement | null
    if (!a || !/^(https?|mailto):/.test(a.href)) return
    e.preventDefault()
    api.openLink(a.href).catch(() => {})
  }, true)
}

// Not signed in, or not allowed: asking again will not change the answer.
const final = (e: unknown) => e instanceof ApiError && [401, 403, 404].includes(e.status)
const queries = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: true, staleTime: 5_000, retry: (n, e) => !final(e) && n < 3 } } })

// The Telegram Mini App is a page of its own: Telegram's script, its
// initData and a session kept in memory, none of which the app uses.
const miniApp = location.pathname.replace(/\/$/, '') === '/tg/app'

function MiniAppRoot() {
  const [initData, setInitData] = useState<string>()
  useEffect(() => {
    loadTelegram().then((tg) => {
      tg?.ready()
      tg?.expand()
      setInitData(tg?.initData ?? '')
    })
  }, [])
  return <MiniApp initData={initData} />
}

createRoot(document.getElementById('root')!).render(miniApp ? (
  <StrictMode>
    <QueryClientProvider client={queries}>
      <LocaleProvider locale={localeFrom(telegramLanguage() ?? navigator.language)}>
        <MemoryRouter>
          <MiniAppRoot />
        </MemoryRouter>
      </LocaleProvider>
    </QueryClientProvider>
  </StrictMode>
) : (
  <StrictMode>
    <QueryClientProvider client={queries}>
      <LocaleProvider>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </LocaleProvider>
    </QueryClientProvider>
  </StrictMode>
))

if (!miniApp && 'serviceWorker' in navigator && location.protocol !== 'file:') {
  navigator.serviceWorker.register('/sw.js').catch(() => {})
}
