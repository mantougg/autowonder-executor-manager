// playwright.config.ts：总览页浏览器级验证（Mock API + 本地构建产物，无头隔离）。
// 运行：npm run test:e2e（全量）/ npm run test:e2e:baseline（旧行为特征基线）
import { defineConfig } from '@playwright/test'

const PORT = Number(process.env.MOCK_PORT || 43117)

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: [
    ['list'],
    ['json', { outputFile: 'test-results/results.json' }],
  ],
  outputDir: 'test-results/artifacts',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    headless: true,
    // 复用本机已安装 Chrome（不下载浏览器二进制）；隔离上下文，不复用个人配置/登录态
    channel: 'chrome',
    viewport: { width: 1280, height: 800 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'node tests/mock-server.mjs',
    url: `http://127.0.0.1:${PORT}/__test__/health`,
    reuseExistingServer: false,
    timeout: 30_000,
    env: { MOCK_PORT: String(PORT) },
  },
})
