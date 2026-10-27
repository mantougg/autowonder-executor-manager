// detail.logscroll.spec.ts：@logscroll 日志滚动链隔离。
// 日志区到顶/底后继续滚轮不得带动详情页；页面可滚动时位置也不因日志滚动改变；
// 页面内容可容纳时不得出现纵向滚动条。日志流通过 Playwright routeWebSocket 注入。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import { gotoDetail, resetMock } from './helpers'

const LINES = Array.from({ length: 300 }, (_, i) => `log line ${i} ${'x'.repeat(40)}`)

async function settle(page: Page) {
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
}

async function mockLogStream(page: Page) {
  await page.routeWebSocket(/\/ws\/logs\//, (ws) => {
    ws.send(JSON.stringify({ type: 'backlog', lines: LINES }))
  })
}

test.describe('@logscroll 日志滚动链隔离', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('日志到顶/底继续滚轮不改变页面滚动位置', async ({ page }) => {
    await mockLogStream(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoDetail(page, 'Alpha')

    const log = page.getByRole('log', { name: 'Alpha 日志' })
    await expect(log).toContainText('log line 299')
    await log.scrollIntoViewIfNeeded()
    await log.hover()
    const beforeY = await page.evaluate(() => window.scrollY)

    await log.evaluate((el) => { el.scrollTop = el.scrollHeight })
    await settle(page)
    await page.mouse.wheel(0, 800)
    await settle(page)
    expect(await page.evaluate(() => window.scrollY), '日志到底后滚轮不得带动页面').toBe(beforeY)

    await log.evaluate((el) => { el.scrollTop = 0 })
    await settle(page)
    await page.mouse.wheel(0, -800)
    await settle(page)
    expect(await page.evaluate(() => window.scrollY), '日志到顶后滚轮不得带动页面').toBe(beforeY)
  })

  test('页面内容可容纳时无页面纵向滚动条，日志滚轮不使其出现', async ({ page }) => {
    await mockLogStream(page)
    await page.setViewportSize({ width: 1600, height: 2000 })
    await gotoDetail(page, 'Alpha')

    const log = page.getByRole('log', { name: 'Alpha 日志' })
    await expect(log).toContainText('log line 299')

    const before = await page.evaluate(() => ({
      scrollY: window.scrollY,
      scrollH: document.documentElement.scrollHeight,
      inner: window.innerHeight,
    }))
    expect(before.scrollY).toBe(0)
    expect(before.scrollH, '页面内容可容纳时不应有纵向滚动条').toBeLessThanOrEqual(before.inner + 1)

    await log.hover()
    await log.evaluate((el) => { el.scrollTop = el.scrollHeight })
    await settle(page)
    await page.mouse.wheel(0, 900)
    await settle(page)
    const afterDown = await page.evaluate(() => ({
      scrollY: window.scrollY,
      scrollH: document.documentElement.scrollHeight,
      inner: window.innerHeight,
    }))
    expect(afterDown.scrollY).toBe(0)
    expect(afterDown.scrollH).toBeLessThanOrEqual(afterDown.inner + 1)

    await log.evaluate((el) => { el.scrollTop = 0 })
    await settle(page)
    await page.mouse.wheel(0, -900)
    await settle(page)
    const afterUp = await page.evaluate(() => ({
      scrollY: window.scrollY,
      scrollH: document.documentElement.scrollHeight,
      inner: window.innerHeight,
    }))
    expect(afterUp.scrollY).toBe(0)
    expect(afterUp.scrollH).toBeLessThanOrEqual(afterUp.inner + 1)

    const overscroll = await log.evaluate((el) => getComputedStyle(el).overscrollBehaviorY)
    expect(overscroll, '日志容器应隔离滚动链').toBe('contain')
  })
})
