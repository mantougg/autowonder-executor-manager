// detail.timeline.spec.ts：@timeline 一期详情页运行时间带与崩溃点日志穿透。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import { gotoDetail, mockRequests, resetMock, setMock } from './helpers'

const H = 3600_000

function iso(ms: number): string {
  return new Date(ms).toISOString()
}

async function seedTimeline(page: Page, request: Parameters<typeof setMock>[0], opts?: {
  locatable?: boolean
}) {
  const now = Date.now()
  const locatable = opts?.locatable ?? true
  await setMock(request, {
    timelines: {
      Alpha: {
        data_start: iso(now - 20 * H),
        data_end: iso(now - 1 * H),
        segments: [
          { from: iso(now - 24 * H), to: iso(now - 20 * H), state: 'nodata' },
          { from: iso(now - 20 * H), to: iso(now - 10 * H), state: 'running' },
          { from: iso(now - 10 * H), to: iso(now - 9 * H), state: 'backoff' },
          { from: iso(now - 9 * H), to: iso(now), state: 'stopped' },
        ],
        crashes: [
          {
            at: iso(now - 10 * H),
            auto_restart: true,
            locatable,
            ...(locatable ? { log_file: 'Alpha.log', log_offset: 120 } : {}),
          },
        ],
      },
    },
    logs: { Alpha: { jump_lines: ['crash-a', 'crash-b'], lines: ['recent-1', 'recent-2'] } },
  })
  await page.reload()
}

test.describe('@timeline 详情页运行时间带', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('区间渲染、数据起点、hover/键盘 tooltip', async ({ page, request }) => {
    await gotoDetail(page, 'Alpha')
    await seedTimeline(page, request)

    const bar = page.locator('.awem-timeline-bar')
    await expect(bar).toBeVisible()
    await expect(bar.locator('.awem-timeline-seg.nodata')).toHaveCount(1)
    await expect(bar.locator('.awem-timeline-seg.running')).toHaveCount(1)
    await expect(bar.locator('.awem-timeline-seg.backoff')).toHaveCount(1)
    await expect(bar.locator('.awem-timeline-seg.stopped')).toHaveCount(1)
    await expect(page.getByText(/数据自 .* 起，events\.log 滚动范围/)).toBeVisible()

    await bar.locator('.awem-timeline-seg.running').hover()
    await expect(page.getByRole('tooltip')).toContainText('运行中')

    await bar.locator('.awem-timeline-seg.backoff').focus()
    await expect(page.getByRole('tooltip')).toContainText('重连中')

    await expect(page.getByRole('button', { name: /崩溃点/ })).toBeVisible()
  })

  test('完全无事件时显示暂无事件数据', async ({ page, request }) => {
    await setMock(request, {
      timelines: { Alpha: { data_start: null, data_end: null, segments: [], crashes: [] } },
    })
    await gotoDetail(page, 'Alpha')
    await expect(page.getByText('暂无事件数据')).toBeVisible()
    await expect(page.locator('.awem-timeline-bar')).toHaveCount(0)
  })

  test('崩溃点穿透定位日志并暂停跟随', async ({ page, request }) => {
    await gotoDetail(page, 'Alpha')
    await seedTimeline(page, request)

    await page.getByRole('button', { name: /崩溃点/ }).click()
    const logBox = page.getByRole('log', { name: 'Alpha 日志' })
    await expect(logBox).toContainText('crash-a')
    await expect(logBox).toContainText('crash-b')
    await expect(page.getByRole('switch', { name: '跟随滚动' })).toHaveAttribute('aria-checked', 'false')
    await expect(page.getByRole('status').filter({ hasText: '已定位到' })).toBeVisible()

    const calls = (await mockRequests(request)).filter(
      (r) => r.method === 'GET' && r.path === '/api/executors/Alpha/log' && r.query.file,
    )
    expect(calls).toHaveLength(1)
    expect(calls[0].query).toEqual({ file: 'Alpha.log', before: '120', limit: '500' })
  })

  test('无定位信息时降级：加载最近历史页并提示无法精确定位', async ({ page, request }) => {
    await gotoDetail(page, 'Alpha')
    await seedTimeline(page, request, { locatable: false })

    await page.getByRole('button', { name: /崩溃点/ }).click()
    const logBox = page.getByRole('log', { name: 'Alpha 日志' })
    await expect(logBox).toContainText('recent-1')
    await expect(page.getByRole('switch', { name: '跟随滚动' })).toHaveAttribute('aria-checked', 'false')
    await expect(page.getByRole('status').filter({ hasText: '无法精确定位' })).toBeVisible()

    const calls = (await mockRequests(request)).filter(
      (r) => r.method === 'GET' && r.path === '/api/executors/Alpha/log',
    )
    expect(calls.some((c) => c.query.tail === '500')).toBeTruthy()
  })
})
