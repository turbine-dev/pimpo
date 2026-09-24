import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './index.css'

const queries = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: true, staleTime: 5_000 } } })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queries}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)

if ('serviceWorker' in navigator && location.protocol !== 'file:') {
  navigator.serviceWorker.register('/sw.js').catch(() => {})
}
