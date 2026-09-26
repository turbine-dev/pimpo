import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { api } from './lib/api'
import { LocaleProvider } from './lib/i18n'
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

const queries = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: true, staleTime: 5_000 } } })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queries}>
      <LocaleProvider>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </LocaleProvider>
    </QueryClientProvider>
  </StrictMode>,
)

if ('serviceWorker' in navigator && location.protocol !== 'file:') {
  navigator.serviceWorker.register('/sw.js').catch(() => {})
}
