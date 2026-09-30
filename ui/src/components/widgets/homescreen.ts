import { api } from '../../lib/api'

// In the Android app, a widget can go on the phone's home screen. The app
// gives the page a small bridge; the server makes the phone's widget key,
// which reads only the widgets pinned to this phone.

type Bridge = { available(): boolean; setup(key: string): boolean; pin(id: string): boolean }
const bridge = () => (window as unknown as { PimpoWidgets?: Bridge }).PimpoWidgets

export const canPinToHome = () => {
  try { return bridge()?.available() === true } catch { return false }
}

export async function pinToHome(id: string) {
  const b = bridge()
  if (!b) return false
  const { key } = await api.pinToPhone({ pin: id })
  b.setup(key)
  return b.pin(id)
}
