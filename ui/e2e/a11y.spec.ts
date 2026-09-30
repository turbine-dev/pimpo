import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

// WCAG 2.1 AA on every main page, in both themes.
for (const theme of ['dark', 'light']) {
  test(`main pages meet WCAG AA (${theme})`, async ({ page }) => {
    await page.goto('/auth?token=e2e-token')
    await page.evaluate((t) => localStorage.setItem('pimpo.theme', t), theme)
    await page.reload()
    await page.waitForLoadState('networkidle')
    // The first visit asks for the administrator's account; check that
    // screen too, then make it.
    const create = page.getByRole('heading', { name: 'Crie a conta de administrador' })
    if (await create.isVisible()) {
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
      expect(results.violations.map((v) => `account: ${v.id}`)).toEqual([])
      await page.getByLabel('Seu nome').fill('Dener')
      await page.getByRole('button', { name: 'Criar conta' }).click()
      // Where the browser can make passkeys, the next step offers one.
      const later = page.getByRole('button', { name: 'Agora não' })
      await later.waitFor({ timeout: 5000 }).then(() => later.click(), () => {})
      await expect(create).toBeHidden()
    }
    for (const path of ['/', '/routines', '/inbox', '/receipts', '/rules', '/cost', '/connections', '/settings', '/settings#modelos', '/settings#backup', '/settings#notificacoes', '/chat', '/assistants', '/help', '/welcome', '/memory', '/people', '/gallery', '/import']) {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
      const problems = results.violations.map((v) => `${path}: ${v.id} (${v.nodes.length}) ${v.nodes.map((n) => n.html.slice(0, 200) + ' — ' + (n.any[0]?.message ?? '')).join(' | ')}`)
      expect(problems, problems.join('\n')).toEqual([])
    }
  })
}
