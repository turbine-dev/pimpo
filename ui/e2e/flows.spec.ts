import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

async function login(page: Page) {
  await page.goto('/auth?token=e2e-token')
}

test.describe.serial('the story, end to end', () => {
  test('first run guide leads to the first task', async ({ page }) => {
    await login(page)
    await expect(page).toHaveURL(/\/welcome/)
    await expect(page.getByRole('heading', { name: 'Boas-vindas ao Zodim' })).toBeVisible()
    for (let i = 0; i < 3; i++) await page.getByRole('button', { name: 'Continuar' }).click()
    await page.getByRole('button', { name: /Conservador/ }).click()
    await page.getByRole('button', { name: /Equilibrado/ }).click()
    await page.getByRole('button', { name: 'Continuar' }).click()
    await page.getByLabel('Limite diário em dólares').fill('2')
    await page.getByRole('button', { name: /Pedir a primeira tarefa/ }).click()
    await expect(page.getByRole('heading', { name: 'O que o Zodim deve fazer?' })).toBeVisible()
  })

  test('a task is explored, then compiled into a routine', async ({ page }) => {
    await login(page)
    await page.goto('/routines')
    await page.getByRole('button', { name: 'Nova tarefa' }).click()
    await page.getByLabel('Pedido').fill('Todo dia às 7h me manda a agenda e os e-mails importantes.')
    await page.getByRole('button', { name: /Fazer agora/ }).click()
    await expect(page.getByText('Fazendo agora, com você olhando')).toBeVisible()
    await expect(page.getByText(/Leu a agenda · 3 eventos/)).toBeVisible({ timeout: 20_000 })
    await expect(page.getByText(/Decidiu “Este e-mail é importante para hoje\?” · 2 sim, 2 não/)).toBeVisible()
    await page.getByRole('button', { name: 'Transformar em rotina' }).click()
    await expect(page.getByText('Virou rotina. A partir de agora, ela roda sozinha.')).toBeVisible()
    await page.getByRole('button', { name: /Ver o código/ }).click()
    await expect(page.getByText(/calendar\.events/).first()).toBeVisible()
    await page.getByRole('tab', { name: 'O que ela pode fazer' }).click()
    await expect(page.getByText('Lê a agenda')).toBeVisible()
  })

  test('a routine runs and its receipts can be read', async ({ page }) => {
    await login(page)
    await page.goto('/routines')
    await page.getByRole('button', { name: 'Rotina Resumo matinal' }).click()
    await page.getByRole('button', { name: 'Rodar agora' }).click()
    await page.getByRole('tab', { name: 'Execuções' }).click()
    await expect(page.getByText(/ações/).first()).toBeVisible()
    await page.getByRole('link', { name: 'Atividade' }).click()
    await expect(page.getByText(/Te mandou: “☀️ Bom dia!/).first()).toBeVisible()
  })

  test('archiving can be undone from the receipts', async ({ page }) => {
    await login(page)
    await page.goto('/routines')
    await page.getByRole('button', { name: 'Nova tarefa' }).click()
    await page.getByLabel('Pedido').fill('Às 18h arquiva as newsletters e promoções não lidas.')
    await page.getByRole('button', { name: /Fazer agora/ }).click()
    await expect(page.getByRole('button', { name: 'Transformar em rotina' })).toBeVisible({ timeout: 20_000 })
    await page.getByRole('button', { name: 'Transformar em rotina' }).click()
    await expect(page.getByText('Virou rotina. A partir de agora, ela roda sozinha.')).toBeVisible()
    await page.goto('/routines')
    await page.getByRole('button', { name: 'Rotina Triagem de newsletters' }).click()
    await page.getByRole('button', { name: 'Rodar agora' }).click()
    await page.getByRole('link', { name: 'Atividade' }).click()
    const undo = page.getByRole('button', { name: 'Desfazer' }).first()
    await expect(undo).toBeVisible()
    await undo.click()
    await expect(page.getByText('desfeito').first()).toBeVisible()
  })

  test('a rule is written in words, checked and saved', async ({ page }) => {
    await login(page)
    await page.goto('/rules')
    await page.getByLabel('Nova regra').fill('Nunca apague e-mail sem me perguntar')
    await page.getByRole('button', { name: /Criar/ }).click()
    await expect(page.getByText('Entendido assim — confira antes de salvar:')).toBeVisible()
    await page.getByRole('button', { name: /Salvar regra/ }).click()
    await expect(page.getByText('Nunca apague e-mail sem me perguntar')).toBeVisible()
  })

  test('the palette jumps anywhere', async ({ page }) => {
    await login(page)
    await page.keyboard.press('ControlOrMeta+k')
    await page.getByRole('textbox', { name: 'Buscar' }).fill('custo')
    await page.keyboard.press('Enter')
    await expect(page.getByRole('heading', { name: 'Custo' })).toBeVisible()
  })

  test('the phone layout keeps the essentials', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await login(page)
    await expect(page.getByRole('navigation', { name: 'Principal (celular)' })).toBeVisible()
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    const width = await page.evaluate(() => document.documentElement.scrollWidth)
    expect(width).toBeLessThanOrEqual(390)
  })

  test('a gallery routine is checked and installed', async ({ page }) => {
    await login(page)
    await page.goto('/gallery')
    await page.getByRole('tab', { name: 'Só leem e avisam você' }).click()
    await page.getByText('Agenda do dia', { exact: true }).click()
    await expect(page.getByText(/Assinatura confere/)).toBeVisible()
    await page.getByRole('button', { name: 'Instalar esta rotina' }).click()
    await expect(page).toHaveURL(/\/routines\/agenda-do-dia/)
  })


  test('someone is invited to the house', async ({ page }) => {
    await login(page)
    await page.goto('/people')
    await page.getByRole('button', { name: 'Convidar alguém' }).click()
    await page.getByLabel('Nome').fill('Léo')
    await page.getByRole('radio', { name: /Convidado/ }).click()
    await page.getByRole('button', { name: 'Criar convite' }).click()
    await expect(page.getByRole('button', { name: 'Copiar convite' })).toContainText('/start')
  })

  test('everything exports to one file', async ({ page }) => {
    await login(page)
    await page.goto('/settings')
    await page.getByRole('button', { name: 'Backup' }).click()
    await page.getByLabel('Senha para exportar').fill('uma senha longa')
    const download = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Exportar' }).click()
    expect((await download).suggestedFilename()).toMatch(/\.zodim$/)
  })

  test('a local copy can be put back on the next start', async ({ page }) => {
    await login(page)
    await page.goto('/settings#backup')
    await page.getByRole('button', { name: 'Guardar cópia agora' }).click()
    const row = page.getByRole('listitem').filter({ hasText: 'Guardada por você' }).first()
    await expect(row).toBeVisible()
    page.once('dialog', (d) => d.accept())
    await row.getByRole('button', { name: /Voltar a esta/ }).click()
    await expect(page.getByText(/Ao fechar e abrir o Zodim/)).toBeVisible()
    await page.getByRole('button', { name: 'Cancelar' }).click()
    await expect(page.getByText(/Ao fechar e abrir o Zodim/)).toHaveCount(0)
  })

  test('the phone menu reaches every screen', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await login(page)
    const nav = page.getByRole('navigation', { name: 'Principal (celular)' })
    await expect(nav.getByRole('link', { name: 'Conversar' })).toBeVisible()
    await nav.getByRole('button', { name: 'Mais' }).click()
    await page.getByRole('link', { name: 'Galeria' }).click()
    await expect(page.getByRole('heading', { name: 'Galeria' })).toBeVisible()
  })
})


test('a chat rehearses, then does exactly what it showed', async ({ page }) => {
  await login(page)
  await page.goto('/chat')
  await expect(page.getByRole('heading', { name: 'Como o Zodim pode ajudar?' })).toBeVisible()
  await page.getByLabel('Escreva uma mensagem…').fill('Arquive as newsletters da caixa de entrada')
  await page.getByLabel('Escreva uma mensagem…').press('Enter')
  await expect(page.getByText('Trabalhando…')).toBeVisible()
  await expect(page.getByText(/Três eram newsletters/)).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('Para isso, o Zodim faria:')).toBeVisible()
  await expect(page.getByText('Arquivar um e-mail (INBOX/40)')).toBeVisible()
  await page.getByRole('button', { name: 'Confirmar e fazer' }).click()
  // An earlier test may have archived these already; either way it answers.
  await expect(page.getByText(/^Feito\.$|Parte não deu certo/)).toBeVisible({ timeout: 10_000 })
  await expect(page.getByRole('button', { name: 'Confirmar e fazer' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: /Arquive as newsletters/ })).toBeVisible()
})

test('every screen is in the sidebar', async ({ page }) => {
  await page.goto('/auth?token=e2e-token')
  const nav = page.getByRole('navigation', { name: 'Principal', exact: true })
  for (const name of ['Início', 'Rotinas', 'Atividade', 'Assistentes']) {
    await expect(nav.getByRole('link', { name })).toBeVisible()
  }
  if ((await nav.getByRole('link', { name: 'Galeria' }).count()) === 0) await nav.getByRole('button', { name: 'Mais' }).click()
  for (const name of ['Galeria', 'Conexões', 'Pessoas', 'Memória', 'Regras', 'Custo']) {
    await expect(nav.getByRole('link', { name })).toBeVisible()
  }
  await page.getByRole('button', { name: 'Menu do Zodim' }).click()
  await expect(page.getByRole('menuitem', { name: 'Ajustes' })).toBeVisible()
  await page.getByRole('menuitem', { name: /Ocupação do sistema/ }).click()
  await expect(page.getByRole('heading', { name: 'Ocupação do sistema' })).toBeVisible()
})

test('a routine\'s schedule and settings change without code', async ({ page }) => {
  await page.goto('/auth?token=e2e-token')
  await page.goto('/gallery')
  await page.getByText('Clima da manhã', { exact: true }).click()
  await page.getByRole('button', { name: 'Instalar esta rotina' }).click()
  await expect(page).toHaveURL(/\/routines\/clima-da-manha/)
  await page.getByLabel('Quando').selectOption('weekdays')
  await page.getByLabel('às').fill('06:30')
  await page.getByLabel('Avisar guarda-chuva a partir de (% de chuva)').fill('80')
  await page.getByRole('button', { name: 'Salvar ajustes' }).click()
  await expect(page.getByText('Salvo.')).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Quando')).toHaveValue('weekdays')
  await expect(page.getByLabel('Avisar guarda-chuva a partir de (% de chuva)')).toHaveValue('80')
  await expect(page.getByText(/dias úteis às 06:30/i).first()).toBeVisible()
  for (const theme of ['dark', 'light']) {
    await page.evaluate((t) => localStorage.setItem('zodim.theme', t), theme)
    await page.reload()
    await page.waitForLoadState('networkidle')
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(results.violations.map((v) => `${theme}: ${v.id} ${v.nodes.map((n) => n.html.slice(0, 120)).join(' | ')}`)).toEqual([])
  }
})
