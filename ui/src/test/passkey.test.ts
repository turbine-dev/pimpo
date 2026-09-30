import { afterEach, describe, expect, it, vi } from 'vitest'
import { canUsePasskeys, passkeyMessage, PasskeyError } from '../lib/passkey'

const at = (url: string, extra: Record<string, unknown> = {}) => {
  const u = new URL(url)
  vi.stubGlobal('location', { hostname: u.hostname, protocol: u.protocol })
  vi.stubGlobal('isSecureContext', true)
  vi.stubGlobal('PublicKeyCredential', function () {})
  for (const [k, v] of Object.entries(extra)) vi.stubGlobal(k, v)
}

describe('where passkeys are offered', () => {
  afterEach(() => { vi.unstubAllGlobals(); delete (window as unknown as Record<string, unknown>).__PIMPO_DESKTOP__ })
  it('at localhost and at https addresses with a name', () => {
    at('http://localhost:7788/')
    expect(canUsePasskeys()).toBe(true)
    at('https://pimpo.tail1.ts.net/')
    expect(canUsePasskeys()).toBe(true)
  })
  it('never at a bare IP address', () => {
    at('http://127.0.0.1:7788/')
    expect(canUsePasskeys()).toBe(false)
    at('https://192.168.1.20/')
    expect(canUsePasskeys()).toBe(false)
    at('https://[::1]/')
    expect(canUsePasskeys()).toBe(false)
  })
  it('never in the desktop app window', () => {
    at('http://localhost:7788/')
    ;(window as unknown as Record<string, unknown>).__PIMPO_DESKTOP__ = 'mac'
    expect(canUsePasskeys()).toBe(false)
  })
})

describe('passkey errors in words', () => {
  const t = ((k: string) => `[${k}]`) as never
  it('shows the server code, the browser cancel and the rest', () => {
    expect(passkeyMessage(new PasskeyError('passkey.address', 'raw english'), t)).toBe('[passkey.address]')
    expect(passkeyMessage(new DOMException('x', 'NotAllowedError'), t)).toBe('[passkey.cancelled]')
    expect(passkeyMessage(new DOMException('x', 'InvalidStateError'), t)).toBe('[passkey.exists]')
    expect(passkeyMessage(new Error('something else'), t)).toBe('something else')
  })
})
