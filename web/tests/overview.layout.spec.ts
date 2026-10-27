// overview.layout.spec.ts：@layout 总览表格视口自适应限高、表头吸顶、
// 页面不出现纵向滚动条、少行自然高度与过矮视口 240px 兜底。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import { gotoOverview, resetMock, setMock } from './helpers'

const VIEWPORTS = [
  { width: 1920, height: 1080 },
  { width: 1440, height: 900 },
  { width: 1366, height: 768 },
]

function manyExecutors(n: number) {
  return Array.from({ length: n }, (_, i) => ({
    name: `exec-${20001 + i}`,
    state: i % 3 === 0 ? 'running' : i % 3 === 1 ? 'stopped' : 'backoff',
    platform: {
      status: i % 3 === 0 ? 'ONLINE' : i % 3 === 1 ? 'OFFLINE' : 'BUSY',
      name: `执行器 ${i + 1}`,
      agent_name: `工人 ${i + 1}`,
      project: i % 2 ? 'git-workspace' : 'autowonder-executor-manager',
      squad: i % 2 ? '快速交付' : '标准自动化交付',
      squad_names: [i % 2 ? '快速交付' : '标准自动化交付'],
    },
  }))
}

async function metrics(page: Page) {
  return page.evaluate(() => {
    const wrap = document.querySelector('.awem-overview-table-wrap') as HTMLElement | null
    if (!wrap) return null
    const th = wrap.querySelector('thead th') as HTMLElement
    const bar = document.querySelector('.awem-filter-bar') as HTMLElement | null
    const toolbar = document.querySelector('.awem-table-toolbar') as HTMLElement | null
    const wrapRect = wrap.getBoundingClientRect()
    const headRect = th.getBoundingClientRect()
    return {
      docScrollHeight: document.documentElement.scrollHeight,
      innerHeight: window.innerHeight,
      scrollY: window.scrollY,
      wrapClientHeight: wrap.clientHeight,
      wrapScrollHeight: wrap.scrollHeight,
      wrapTop: wrapRect.top,
      headTop: headRect.top,
      headHeight: headRect.height,
      barInWrap: bar ? wrap.contains(bar) : null,
      toolbarInWrap: toolbar ? wrap.contains(toolbar) : null,
      wrapMaxHeight: getComputedStyle(wrap).maxHeight,
    }
  })
}

test.describe('@layout 总览表格限高与吸顶', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  for (const vp of VIEWPORTS) {
    test(`${vp.width}x${vp.height}：表格内部滚动且页面无纵向滚动条，表头吸顶`, async ({ page, request }) => {
      await setMock(request, { executors: manyExecutors(30) })
      await page.setViewportSize(vp)
      await gotoOverview(page)
      await expect(page.locator('.awem-overview-table-wrap')).toBeVisible()

      await expect.poll(async () => {
        const m = await page.evaluate(() => ({
          h: document.documentElement.scrollHeight,
          i: window.innerHeight,
        }))
        return m.h <= m.i + 1
      }, { timeout: 3_000 }).toBe(true)

      const m = await metrics(page)
      expect(m, '存在 .awem-overview-table-wrap').not.toBeNull()
      expect(m!.docScrollHeight, '页面不应出现纵向溢出').toBeLessThanOrEqual(m!.innerHeight + 1)
      expect(m!.wrapScrollHeight, '表格容器内部应可纵向滚动').toBeGreaterThan(m!.wrapClientHeight)
      expect(m!.barInWrap, '筛选行在滚动容器之外').toBe(false)
      expect(m!.toolbarInWrap, '工具行在滚动容器之外').toBe(false)
      expect(m!.scrollY).toBe(0)

      await page.evaluate(() => {
        const w = document.querySelector('.awem-overview-table-wrap') as HTMLElement
        w.scrollTop = 200
      })
      const s = await metrics(page)
      expect(Math.abs(s!.headTop - s!.wrapTop), '表头应吸附在容器顶部').toBeLessThanOrEqual(2)
      expect(s!.scrollY).toBe(0)
    })
  }

  test('行数少时表格保持自然高度，不留空滚动区', async ({ page, request }) => {
    await setMock(request, { executors: manyExecutors(3) })
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoOverview(page)
    await expect(page.locator('.awem-overview-table-wrap')).toBeVisible()

    const m = await metrics(page)
    expect(m, '存在 .awem-overview-table-wrap').not.toBeNull()
    expect(m!.wrapScrollHeight).toBeLessThanOrEqual(m!.wrapClientHeight + 1)
    expect(m!.docScrollHeight).toBeLessThanOrEqual(m!.innerHeight + 1)
  })

  test('视口过矮时表体保持最小 240px，允许页面滚动兜底', async ({ page, request }) => {
    await setMock(request, { executors: manyExecutors(30) })
    await page.setViewportSize({ width: 1280, height: 360 })
    await gotoOverview(page)
    await expect(page.locator('.awem-overview-table-wrap')).toBeVisible()

    const m = await metrics(page)
    expect(m, '存在 .awem-overview-table-wrap').not.toBeNull()
    expect(m!.docScrollHeight, '过矮视口允许页面滚动').toBeGreaterThan(m!.innerHeight)
    expect(m!.wrapClientHeight - m!.headHeight, '表体高度不得低于 240px').toBeGreaterThanOrEqual(239)

    await page.evaluate(() => {
      const w = document.querySelector('.awem-overview-table-wrap') as HTMLElement
      w.scrollTop = 100
    })
    const s = await metrics(page)
    expect(Math.abs(s!.headTop - s!.wrapTop)).toBeLessThanOrEqual(2)
  })

  test('≤768px 窄视口筛选行可换行，页面无横向溢出', async ({ page, request }) => {
    await setMock(request, { executors: manyExecutors(30) })
    await page.setViewportSize({ width: 720, height: 900 })
    await gotoOverview(page)
    await expect(page.locator('.awem-overview-table-wrap')).toBeVisible()

    const m = await page.evaluate(() => {
      const bar = document.querySelector('.awem-filter-bar') as HTMLElement
      const fields = [...bar.querySelectorAll('.awem-filter-field')] as HTMLElement[]
      return {
        pageScrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
        fieldTops: fields.map((f) => Math.round(f.getBoundingClientRect().top)),
      }
    })
    expect(m.pageScrollWidth, '页面不应横向溢出').toBeLessThanOrEqual(m.innerWidth + 1)
    expect(new Set(m.fieldTops).size, '筛选条件应允许换行').toBeGreaterThan(1)
  })

  test('横向滚动时表头与表体同步移动', async ({ page, request }) => {
    await setMock(request, { executors: manyExecutors(30) })
    await page.setViewportSize({ width: 800, height: 900 })
    await gotoOverview(page)
    await expect(page.locator('.awem-overview-table-wrap')).toBeVisible()

    const read = () => page.evaluate(() => {
      const w = document.querySelector('.awem-overview-table-wrap') as HTMLElement
      const th = w.querySelector('thead th:nth-child(2)') as HTMLElement
      const td = w.querySelector('tbody tr td:nth-child(2)') as HTMLElement
      return {
        overflow: w.scrollWidth - w.clientWidth,
        thLeft: th.getBoundingClientRect().left,
        tdLeft: td.getBoundingClientRect().left,
      }
    })
    const before = await read()
    expect(before.overflow, '窄视口应出现表格内部横向滚动').toBeGreaterThan(0)

    await page.evaluate(() => {
      const w = document.querySelector('.awem-overview-table-wrap') as HTMLElement
      w.scrollLeft = 120
    })
    const after = await read()
    expect(Math.abs(after.thLeft - after.tdLeft), '表头与表体应同步').toBeLessThanOrEqual(1)
    expect(Math.abs(before.thLeft - after.thLeft - 120), '横向滚动位移应一致').toBeLessThanOrEqual(2)
  })
})
