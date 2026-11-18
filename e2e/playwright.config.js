// Homes in a real browser (owner, 2026-10-24: testing and UX both matter).
// cmd/homesdemo serves the family pages against fakes; one worker, as the
// demo's house is shared. Phone first: a Pixel-sized viewport, dark.
const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
  testDir: '.',
  workers: 1,
  timeout: 60_000,
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    ...devices['Pixel 7'],
    baseURL: 'http://127.0.0.1:8651',
    colorScheme: 'dark',
    locale: 'de-DE', // Accept-Language: Homes speaks the browser's language (owner 2026-11-18); the family's is German
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'go run ../cmd/homesdemo -addr 127.0.0.1:8651',
    url: 'http://127.0.0.1:8651/start',
    timeout: 180_000,
    reuseExistingServer: !process.env.CI,
  },
});
