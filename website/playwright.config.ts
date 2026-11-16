import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  webServer: {
    command: 'node scripts/serve.mjs',
    url: 'http://127.0.0.1:4325/impactr/',
    reuseExistingServer: true,
  },
  use: { baseURL: 'http://127.0.0.1:4325/impactr', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['iPhone 13'], browserName: 'chromium' } },
  ],
});
