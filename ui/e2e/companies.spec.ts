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

  await page.request.put(`${base}/contexts/voz`, { data: { scope: 'company', title: 'Voz', body: 'Gentil e breve.' }, headers: auth })
  await page.request.put(`${base}/rules/sem-telegram`, { data: { scope: 'company', text: 'Nada de Telegram', when: { capabilities: ['telegram.send'] }, then: 'block' }, headers: auth })
  await page.request.put(`${base}/rules/atendentes`, { data: { scope: 'role', of: 'atendente', text: 'Atendentes podem', when: { capabilities: ['telegram.send'] }, then: 'allow', exception: true }, headers: auth })
  await page.request.put(`${base}/agent-routines/manha`, { data: { member: 'clara', name: 'Manhã', instructions: 'Leia os pedidos da noite', schedule: '0 8 * * 1-5' }, headers: auth })
  await page.request.post(`${base}/members/clara/work`, { data: { request: 'Responda as mensagens de hoje' }, headers: auth })
  await page.request.post(`${base}/tasks`, { data: { assignee: 'bia', title: 'Lançar a coleção', objective: 'Vender a coleção nova', acceptance: 'Está no ar' }, headers: auth })
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
    await page.getByRole('tab', { name: 'Contexto e regras' }).click()
    await expect(page.getByText('Exceção a: Nada de Telegram')).toBeVisible()
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(results.violations.map((v) => `${theme} layers: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 160)).join(' | ')}`)).toEqual([])
    await page.getByRole('tab', { name: 'Tarefas' }).click()
    await expect(page.getByText('Lançar a coleção')).toBeVisible()
    const board = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(board.violations.map((v) => `${theme} tasks: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 160)).join(' | ')}`)).toEqual([])
    await page.getByRole('tab', { name: 'Trabalho' }).click()
    await expect(page.getByText('Responda as mensagens de hoje').first()).toBeVisible()
    const work = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(work.violations.map((v) => `${theme} work: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 160)).join(' | ')}`)).toEqual([])
    await page.getByRole('tab', { name: 'Organograma' }).click()
    await page.locator('.org').getByRole('button', { name: 'Abrir Clara' }).click()
    await page.getByText('Trabalho e rotinas').click()
    const dialog = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(dialog.violations.map((v) => `${theme} member: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 160)).join(' | ')}`)).toEqual([])
    await page.keyboard.press('Escape')
  }
})
