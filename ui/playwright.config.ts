import { defineConfig } from '@playwright/test'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const data = mkdtempSync(join(tmpdir(), 'pimpo-e2e-'))

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: 0,
  workers: 1,
  use: { baseURL: 'http://127.0.0.1:7790', viewport: { width: 1280, height: 820 }, trace: 'retain-on-failure' },
  webServer: {
    command: `../bin/pimpo serve --demo --data ${data} --addr 127.0.0.1:7790`,
    url: 'http://127.0.0.1:7790/api/health',
    env: { PIMPO_TOKEN: 'e2e-token' },
    reuseExistingServer: false,
    timeout: 30_000,
  },
})
