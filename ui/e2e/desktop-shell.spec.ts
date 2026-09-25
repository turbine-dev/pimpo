import { expect, test, type Page } from '@playwright/test'

// The desktop app's own page, with the native calls stood in for, against
// the e2e Zodim playing the part of one running on another machine.
const shell = new URL('../../desktop/shell/index.html', import.meta.url).href
const link = 'http://127.0.0.1:7790/auth?token=e2e-token'

async function open(page: Page, remote: string[], hash = '') {
  const calls: string[] = []
  await page.exposeFunction('__record', (c: string) => { calls.push(c) })
  await page.addInitScript((remote) => {
    const w = window as unknown as Record<string, unknown>
    w.__ZODIM_DESKTOP__ = 'mac'
    w.__TAURI__ = { core: { invoke: async (cmd: string, args?: object) => { (w.__record as (c: string) => void)(JSON.stringify([cmd, args ?? {}])); return cmd === 'remote' ? remote : null } } }
  }, remote)
  await page.goto(shell + hash)
  return calls
}

test('the desktop connects to a Zodim elsewhere and stops its own', async ({ page }) => {
  const calls = await open(page, [], '#pair')
  await expect(page.getByRole('button', { name: 'Usar o Zodim deste computador' })).toBeVisible()
  await page.getByLabel('Link do seu Zodim').fill('http://example.com/auth?token=x')
  await page.getByRole('button', { name: 'Conectar' }).click()
  await expect(page.getByRole('alert')).toContainText('https')
  await page.getByLabel('Link do seu Zodim').fill(link)
  await page.getByRole('button', { name: 'Conectar' }).click()
  await page.waitForURL(/127\.0\.0\.1:7790/)
  expect(calls.some((c) => c.startsWith('["use_remote"') && c.includes('e2e-token'))).toBe(true)
})

test('a saved remote opens by itself, and one that is off says so', async ({ page, context }) => {
  await open(page, [link, ''])
  await page.waitForURL(/127\.0\.0\.1:7790/)
  const off = await context.newPage()
  const calls = await open(off, ['http://127.0.0.1:7899/auth?token=zz', ''])
  await expect(off.locator('#status')).toContainText('Não encontrei o Zodim em 127.0.0.1:7899')
  await off.getByRole('button', { name: 'Usar o Zodim deste computador' }).click()
  expect(calls.some((c) => c.startsWith('["use_local"'))).toBe(true)
})
