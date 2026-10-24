// fixtures.ts：扩展 Playwright test，页面加载前注入本机 token（跳过登录卡片）。
import { test as base } from '@playwright/test'

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript(() => localStorage.setItem('awem-token', 'test-token'))
    await use(page)
  },
})
