import { expect, test, type Page } from '@playwright/test'

async function login(page: Page) {
  await page.goto('/auth?token=e2e-token')
}

test.describe.serial('the story, end to end', () => {
  test('first run guide leads to the first task', async ({ page }) => {
    await login(page)
    await expect(page).toHaveURL(/\/welcome/)
    await expect(page.getByRole('heading', { name: 'Oi, eu sou o Vigia.' })).toBeVisible()
    for (let i = 0; i < 3; i++) await page.getByRole('button', { name: 'Continuar' }).click()
    await page.getByRole('button', { name: /Conservador/ }).click()
    await page.getByRole('button', { name: /Equilibrado/ }).click()
    await page.getByRole('button', { name: 'Continuar' }).click()
    await page.getByLabel('Limite diário em dólares').fill('2')
    await page.getByRole('button', { name: /Pedir a primeira tarefa/ }).click()
    await expect(page.getByRole('heading', { name: 'O que você quer que eu faça?' })).toBeVisible()
  })

  test('a task is explored, then compiled into a routine', async ({ page }) => {
    await login(page)
    await page.getByRole('button', { name: 'Nova tarefa' }).click()
    await page.getByLabel('Pedido').fill('Todo dia às 7h me manda a agenda e os e-mails importantes.')
    await page.getByRole('button', { name: /Fazer agora/ }).click()
    await expect(page.getByText('Fazendo agora, com você olhando')).toBeVisible()
    await expect(page.getByText(/Leu a agenda · 3 eventos/)).toBeVisible({ timeout: 20_000 })
    await expect(page.getByText(/Decidiu “Este e-mail é importante para hoje\?” · 2 sim, 2 não/)).toBeVisible()
    await page.getByRole('button', { name: 'Transformar em rotina' }).click()
    await expect(page.getByText('Virou rotina. A partir de agora eu faço sozinho.')).toBeVisible()
    await page.getByRole('button', { name: /Ver o código/ }).click()
    await expect(page.getByText(/calendar\.events/).first()).toBeVisible()
    await page.getByRole('tab', { name: 'O que ela pode fazer' }).click()
    await expect(page.getByText('Lê a agenda')).toBeVisible()
  })

  test('a routine runs and its receipts can be read', async ({ page }) => {
    await login(page)
    await page.getByRole('button', { name: 'Rotina Resumo matinal' }).click()
    await page.getByRole('button', { name: 'Rodar agora' }).click()
    await page.getByRole('tab', { name: 'Execuções' }).click()
    await expect(page.getByText(/ações/).first()).toBeVisible()
    await page.getByRole('link', { name: 'Recibos' }).click()
    await expect(page.getByText(/Te mandou: “☀️ Bom dia!/).first()).toBeVisible()
  })

  test('archiving can be undone from the receipts', async ({ page }) => {
    await login(page)
    await page.getByRole('button', { name: 'Nova tarefa' }).click()
    await page.getByLabel('Pedido').fill('Às 18h arquiva as newsletters e promoções não lidas.')
    await page.getByRole('button', { name: /Fazer agora/ }).click()
    await expect(page.getByRole('button', { name: 'Transformar em rotina' })).toBeVisible({ timeout: 20_000 })
    await page.getByRole('button', { name: 'Transformar em rotina' }).click()
    await expect(page.getByText('Virou rotina. A partir de agora eu faço sozinho.')).toBeVisible()
    await page.goto('/')
    await page.getByRole('button', { name: 'Rotina Triagem de newsletters' }).click()
    await page.getByRole('button', { name: 'Rodar agora' }).click()
    await page.getByRole('link', { name: 'Recibos' }).click()
    const undo = page.getByRole('button', { name: 'Desfazer' }).first()
    await expect(undo).toBeVisible()
    await undo.click()
    await expect(page.getByText('desfeito').first()).toBeVisible()
  })

  test('a rule is written in words, checked and saved', async ({ page }) => {
    await login(page)
    await page.getByRole('link', { name: 'Regras' }).click()
    await page.getByLabel('Nova regra').fill('Nunca apague e-mail sem me perguntar')
    await page.getByRole('button', { name: /Criar/ }).click()
    await expect(page.getByText('Entendi assim — confira antes de salvar:')).toBeVisible()
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
    await expect(page.getByRole('heading', { name: 'Rotinas' })).toBeVisible()
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
    await page.getByLabel('Senha para exportar').fill('uma senha longa')
    const download = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Exportar' }).click()
    expect((await download).suggestedFilename()).toMatch(/\.vigia$/)
  })

  test('the phone menu reaches every screen', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await login(page)
    const nav = page.getByRole('navigation', { name: 'Principal (celular)' })
    await expect(nav.getByRole('link', { name: 'Aprovar' })).toBeVisible()
    await nav.getByRole('button', { name: 'Mais' }).click()
    await page.getByRole('link', { name: 'Galeria' }).click()
    await expect(page.getByRole('heading', { name: 'Galeria' })).toBeVisible()
  })
})


test('every screen is in the sidebar', async ({ page }) => {
  await page.goto('/auth?token=e2e-token')
  const nav = page.getByRole('navigation', { name: 'Principal', exact: true })
  for (const name of ['Rotinas', 'Precisa de você', 'Galeria', 'Recibos', 'Regras', 'Custo', 'Memória', 'Conexões', 'Pessoas', 'Ajustes']) {
    await expect(nav.getByRole('link', { name })).toBeVisible()
  }
})
