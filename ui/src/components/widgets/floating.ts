import { useEffect, useState } from 'react'
import { inDesktopApp } from '../Mascot'

// In the desktop app any widget can float on the desktop in a small window
// of its own. The app keeps the list and tells the page (see
// desktop/src-tauri), since the page's storage changes with the port.

const read = (): string[] => {
  try { return JSON.parse(localStorage.getItem('pimpo.floating') ?? '[]') } catch { return [] }
}

export const canFloat = () => inDesktopApp()

export function setFloating(id: string, on: boolean) {
  window.location.assign(`/desktop/float?widget=${encodeURIComponent(id)}&on=${on ? 1 : 0}`)
}

export function useFloating(): string[] {
  const [ids, setIds] = useState(read)
  useEffect(() => {
    const update = () => setIds(read())
    window.addEventListener('pimpo:floating', update)
    return () => window.removeEventListener('pimpo:floating', update)
  }, [])
  return ids
}
