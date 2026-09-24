import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

// WCAG 2.1 AA on every main page, in both themes.
for (const theme of ['dark', 'light']) {
  test(`main pages meet WCAG AA (${theme})`, async ({ page }) => {
    await page.goto('/auth?token=e2e-token')
    await page.evaluate((t) => localStorage.setItem('vigia.theme', t), theme)
    for (const path of ['/', '/inbox', '/receipts', '/rules', '/cost', '/connections', '/settings', '/welcome']) {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
      const problems = results.violations.map((v) => `${path}: ${v.id} (${v.nodes.length}) ${v.nodes.map((n) => n.html.slice(0, 200) + ' — ' + (n.any[0]?.message ?? '')).join(' | ')}`)
      expect(problems, problems.join('\n')).toEqual([])
    }
  })
}
