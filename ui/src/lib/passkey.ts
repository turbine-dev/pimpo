// Passkeys in the browser: WebAuthn wants bytes where the server sends
// base64url text, and the other way round for the answer.

const toBytes = (s: string) => {
  const b = atob(s.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(s.length / 4) * 4, '='))
  return Uint8Array.from(b, (c) => c.charCodeAt(0))
}
const toText = (b: ArrayBuffer | null) => (b ? btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '') : '')

type Descriptor = { id: string; type: string; transports?: string[] }

export const canUsePasskeys = () => typeof window !== 'undefined' && !!window.PublicKeyCredential && window.isSecureContext

async function post(path: string, body?: unknown) {
  const res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error ?? res.statusText)
  return data
}

// addPasskey makes a passkey on this device for the person signed in.
export async function addPasskey(name: string) {
  const { key, options } = await post('/api/passkeys/begin', { name })
  const pk = options.publicKey
  const cred = (await navigator.credentials.create({
    publicKey: {
      ...pk,
      challenge: toBytes(pk.challenge),
      user: { ...pk.user, id: toBytes(pk.user.id) },
      excludeCredentials: (pk.excludeCredentials ?? []).map((c: Descriptor) => ({ ...c, id: toBytes(c.id) })),
    },
  })) as PublicKeyCredential | null
  if (!cred) throw new Error('cancelled')
  const r = cred.response as AuthenticatorAttestationResponse
  return post(`/api/passkeys/finish?key=${key}`, {
    id: cred.id, rawId: toText(cred.rawId), type: cred.type,
    response: { clientDataJSON: toText(r.clientDataJSON), attestationObject: toText(r.attestationObject), transports: r.getTransports?.() ?? [] },
  })
}

// signIn opens a session with a passkey this device has for this address.
export async function signIn() {
  const { key, options } = await post('/auth/passkey/begin')
  const pk = options.publicKey
  const cred = (await navigator.credentials.get({
    publicKey: { ...pk, challenge: toBytes(pk.challenge), allowCredentials: (pk.allowCredentials ?? []).map((c: Descriptor) => ({ ...c, id: toBytes(c.id) })) },
  })) as PublicKeyCredential | null
  if (!cred) throw new Error('cancelled')
  const r = cred.response as AuthenticatorAssertionResponse
  return post(`/auth/passkey/finish?key=${key}`, {
    id: cred.id, rawId: toText(cred.rawId), type: cred.type,
    response: { clientDataJSON: toText(r.clientDataJSON), authenticatorData: toText(r.authenticatorData), signature: toText(r.signature), userHandle: toText(r.userHandle) },
  })
}
