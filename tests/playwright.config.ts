import { defineConfig } from '@playwright/test';
import path from 'path';

const staticDir = path.resolve(__dirname, '../internal/web/static');

export default defineConfig({
  testDir: '.',
  timeout: 15000,
  retries: 0,
  use: {
    baseURL: 'http://localhost:8080',
    headless: true,
  },
  webServer: {
    command: `python3 -m http.server 8080 --directory "${staticDir}"`,
    port: 8080,
    reuseExistingServer: !process.env.CI,
  },
});
