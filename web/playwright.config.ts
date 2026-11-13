import { defineConfig } from '@playwright/test';
export default defineConfig({
 testDir: './e2e', workers: 1, retries: 0,
 use: { baseURL: process.env['E2E_BASE_URL'] || 'http://127.0.0.1:4200',
  launchOptions: { executablePath: process.env['CHROMIUM_PATH'] || undefined },
  screenshot: 'only-on-failure' },
 reporter: 'list'
});
