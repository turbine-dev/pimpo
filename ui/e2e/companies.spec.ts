import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

const auth = { Authorization: 'Bearer e2e-token' }

test('a company: its chart, a boss changed by dragging, in both themes', async ({ page }) => {
  await page.request.put('/api/account', { data: { name: 'Dener' }, headers: auth })
  const settings = await (await page.request.get('/api/settings', { headers: auth })).json()
  await page.request.put('/api/settings', { data: { ...settings, labs_on: [...(settings.labs_on ?? []), 'companies'] }, headers: auth })
  const org = await (await page.request.post('/api/companies', { data: { name: 'Lume Moda', industry: 'Loja online' }, headers: auth })).json()
  const base = `/api/companies/${org.id}`
  await page.request.put(`${base}/departments/vendas`, { data: { name: 'Vendas', color: 'chart-2' }, headers: auth })
  await page.request.put(`${base}/roles/gerente`, { data: { title: 'Gerente' }, headers: auth })
  await page.request.put(`${base}/roles/atendente`, { data: { title: 'Atendente', function: 'Responde clientes' }, headers: auth })
  await page.request.put(`${base}/members/bia`, { data: { name: 'Bia', role: 'gerente', reports_to: 'ceo' }, headers: auth })
  await page.request.put(`${base}/members/clara`, { data: { name: 'Clara', role: 'atendente', department: 'vendas', reports_to: 'bia' }, headers: auth })

  await page.goto('/auth?token=e2e-token')
  await page.goto('/companies')
  await page.getByRole('link', { name: /Lume Moda/ }).click()
  await expect(page.getByRole('heading', { name: 'Lume Moda' })).toBeVisible()
  const chart = page.locator('.org')
  await chart.getByRole('button', { name: 'Abrir Clara' }).dragTo(chart.getByRole('button', { name: 'Abrir Dener' }))
  await expect.poll(async () => (await (await page.request.get(base, { headers: auth })).json()).members.find((m: { id: string }) => m.id === 'clara').reports_to).toBe('ceo')

  for (const theme of ['dark', 'light']) {
    await page.evaluate((t) => localStorage.setItem('pimpo.theme', t), theme)
    for (const path of ['/companies', `/companies/${org.id}`]) {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
      const problems = results.violations.map((v) => `${theme} ${path}: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 160)).join(' | ')}`)
      expect(problems, problems.join('\n')).toEqual([])
    }
  }
})
