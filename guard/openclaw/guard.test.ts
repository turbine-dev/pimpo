// Runs against a real Vigia when VIGIA_URL and VIGIA_TOKEN are set (the Go
// test suite does this), or against a stub otherwise.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { decide } from './guard.ts'

async function stub(answer: object, status = 200) {
  const server = createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      const got = JSON.parse(body)
      res.writeHead(req.headers.authorization === 'Bearer t' && got.agent === 'openclaw' ? status : 401, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify(answer))
    })
  })
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r))
  const port = (server.address() as { port: number }).port
  return { url: `http://127.0.0.1:${port}`, close: () => server.close() }
}

const live = process.env.VIGIA_URL ? { url: process.env.VIGIA_URL, token: process.env.VIGIA_TOKEN ?? '' } : null

test('maps Vigia decisions to OpenClaw hook results', async () => {
  for (const [answer, check] of [
    [{ decision: 'allow', capability: 'guard.read' }, (r: unknown) => assert.equal(r, undefined)],
    [{ decision: 'block', capability: 'guard.web', reason: 'rede de proteção' }, (r: any) => assert.match(r.blockReason, /rede de proteção/)],
    [{ decision: 'ask', capability: 'guard.exec', reason: 'pergunte' }, (r: any) => assert.equal(r.requireApproval.severity, 'critical')],
  ] as const) {
    const s = await stub(answer)
    check(await decide({ url: s.url, token: 't' }, 'exec', { command: 'ls' }))
    s.close()
  }
})

test('fails closed when Vigia is down, unless told otherwise', async () => {
  const down = { url: 'http://127.0.0.1:9', token: 't', timeoutMs: 500 }
  assert.equal(((await decide(down, 'read', {})) as any).block, true)
  assert.equal(await decide({ ...down, failOpen: true }, 'read', {}), undefined)
  const s = await stub({}, 500)
  assert.equal(((await decide({ url: s.url, token: 't' }, 'read', {})) as any).block, true)
  s.close()
})

test('against a running Vigia', { skip: !live }, async () => {
  assert.equal(await decide(live!, 'read', { path: 'a.md' }), undefined)
  assert.equal(((await decide(live!, 'web_fetch', { url: 'https://webhook.site/x' })) as any).block, true)
  assert.ok(((await decide(live!, 'exec', { command: 'ls' })) as any).requireApproval)
})
