// detail.layout.spec.ts：@layout 详情页一屏左右分栏（2026-10-27 起）。
// 左列 进程卡片 → 运行时间带 → 日志卡片（填满剩余高、日志框内部滚动）；
// 右列编辑卡片固定 380px 单列；整页按视口定高（下限 480px），
// 正常视口无页面纵向滚动条，过矮视口允许页面滚动兜底。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import { gotoDetail, resetMock } from './helpers'

const LINES = Array.from({ length: 300 }, (_, i) => `log line ${i}`)

async function mockLogStream(page: Page) {
  await page.routeWebSocket(/\/ws\/logs\//, (ws) => {
    ws.send(JSON.stringify({ type: 'backlog', lines: LINES }))
  })
}

async function geometry(page: Page) {
  return page.evaluate(() => {
    const layout = document.querySelector('.awem-detail-layout') as HTMLElement
    const main = document.querySelector('.awem-detail-main') as HTMLElement
    const side = document.querySelector('.awem-detail-side .awem-card') as HTMLElement
    const cards = [...main.querySelectorAll(':scope > .awem-card')] as HTMLElement[]
    const log = document.querySelector('.awem-detail-log') as HTMLElement
    const logBox = log.querySelector('[role="log"]') as HTMLElement
    const r = (el: HTMLElement) => el.getBoundingClientRect()
    return {
      docScrollHeight: document.documentElement.scrollHeight,
      innerHeight: window.innerHeight,
      layoutH: r(layout).height,
      layoutBottom: r(layout).bottom,
      mainRight: r(main).right,
      sideLeft: r(side).left,
      sideWidth: r(side).width,
      sideBottom: r(side).bottom,
      statusTop: r(cards[0]).top,
      timelineTop: r(cards[1]).top,
      logTop: r(log).top,
      logBottom: r(log).bottom,
      logBoxScroll: logBox.scrollHeight - logBox.clientHeight,
    }
  })
}

test.describe('@layout 详情页一屏左右分栏', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('1440×900：左右并排、左列顺序正确、底边对齐、日志框内部滚动、页面无纵向滚动条', async ({ page }) => {
    await mockLogStream(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoDetail(page, 'Alpha')
    await expect(page.getByRole('log', { name: 'Alpha 日志' })).toContainText('log line 299')

    const m = await geometry(page)
    expect(m.docScrollHeight, '一屏展示：页面不应出现纵向滚动条').toBeLessThanOrEqual(m.innerHeight + 1)
    expect(m.sideLeft, '编辑卡片应在左列右侧').toBeGreaterThan(m.mainRight - 1)
    expect(m.sideWidth, '编辑卡片固定宽 380px').toBe(380)
    expect(m.statusTop).toBeLessThan(m.timelineTop)
    expect(m.timelineTop).toBeLessThan(m.logTop)
    expect(Math.abs(m.logBottom - m.layoutBottom), '日志卡片应填满左列到底边').toBeLessThanOrEqual(1)
    expect(Math.abs(m.sideBottom - m.layoutBottom), '编辑卡片应与左列底边对齐').toBeLessThanOrEqual(1)
    expect(m.logBoxScroll, '300 行日志应在日志框内滚动').toBeGreaterThan(0)
  })

  test('1280×600 过矮视口：布局下限 480px，页面滚动兜底', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 600 })
    await gotoDetail(page, 'Alpha')
    await expect(page.getByRole('heading', { name: '编辑' })).toBeVisible()

    const m = await geometry(page)
    expect(m.layoutH, '布局高度不得低于 480px').toBeGreaterThanOrEqual(479)
    expect(m.docScrollHeight, '过矮视口允许页面滚动').toBeGreaterThan(m.innerHeight)
  })
})
