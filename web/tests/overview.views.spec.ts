// overview.views.spec.ts：@views 一期总览展示改造——Tab 框架、平铺⇄树形、
// 树形分组/重复叶子/未归属、选择去重与会话内展开记忆。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import {
  bulkBar,
  callsTo,
  gotoOverview,
  mockRequests,
  resetMock,
  row,
  rowCheckbox,
  setMock,
} from './helpers'

const TREE_EXECS = [
  {
    name: 'A', state: 'running',
    platform: {
      status: 'ONLINE', name: '执行器A', agent_name: '工人A',
      project: '阿项目', squad: '甲小队、乙小队', squad_names: ['甲小队', '乙小队'],
    },
  },
  {
    name: 'B', state: 'stopped',
    platform: {
      status: 'BUSY', name: '执行器B', agent_name: '工人B',
      project: '阿项目', squad: '乙小队', squad_names: ['乙小队'],
    },
  },
  {
    name: 'C', state: 'stopped',
    platform: {
      status: 'OFFLINE', name: '执行器C', agent_name: '工人C',
      project: '阿项目', squad: '', squad_names: [],
    },
  },
  { name: 'D', state: 'stopped' },
  {
    name: 'E', state: 'stopped',
    platform: {
      status: 'ONLINE', name: '执行器E', agent_name: '工人E',
      project: '波项目', squad: '丙小队', squad_names: ['丙小队'],
    },
  },
]

const coll = new Intl.Collator('zh-CN')

async function setTree(request: Parameters<typeof setMock>[0]) {
  await setMock(request, { executors: TREE_EXECS })
}

async function enterTree(page: Page) {
  await page.getByRole('button', { name: '树形' }).click()
}

test.describe('@views 总览展示改造', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('Tab：仅列表、hash #tab=list 保持、无图表入口、方向键可聚焦', async ({ page }) => {
    await gotoOverview(page)
    const tab = page.getByRole('tab', { name: '列表' })
    await expect(tab).toBeVisible()
    await expect(tab).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('tab', { name: '图表' })).toHaveCount(0)
    await expect(page.getByText('图表')).toHaveCount(0)
    await expect(page.getByRole('tabpanel')).toBeVisible()
    await expect.poll(() => new URL(page.url()).hash).toBe('#tab=list')

    await tab.focus()
    await page.keyboard.press('ArrowRight')
    await expect(tab).toBeFocused()

    await page.reload()
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect.poll(() => new URL(page.url()).hash).toBe('#tab=list')
    await expect(page.getByRole('tab', { name: '列表' })).toHaveAttribute('aria-selected', 'true')
  })

  test('无归属数据时隐藏平铺/树形切换并强制平铺', async ({ page, request }) => {
    await gotoOverview(page)
    await expect(page.getByRole('group', { name: '展示方式' })).toHaveCount(0)
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(4)

    // 有平台状态但没有任何项目/小队归属 → 仍隐藏切换
    await setMock(request, {
      executors: [
        { name: 'U1', state: 'stopped', platform: { status: 'ONLINE', name: '执行器U1' } },
        { name: 'U2', state: 'running', platform: { status: 'OFFLINE', name: '执行器U2' } },
      ],
    })
    await page.reload()
    await expect(page.getByRole('link', { name: '执行器U1' })).toBeVisible()
    await expect(page.getByRole('group', { name: '展示方式' })).toHaveCount(0)
  })

  test('联动 11 列在 1440 视口无横向溢出，操作列完整可见', async ({ page, request }) => {
    // 名称/项目/小队取接近生产数据的长度：表格自然宽约 1180px，1200px 内容区会裁掉操作列
    await setMock(request, {
      executors: [
        'awem 前端机-01', 'awem 前端机-02', 'awem 后端机-01', 'awem 观测机-01',
      ].map((name, i) => ({
        name: `exec-1000${i}`,
        state: i % 2 ? 'running' : 'stopped',
        model: 'dfmodel',
        platform: {
          status: i % 2 ? 'ONLINE' : 'OFFLINE',
          name,
          agent_name: `数字工人-${i}`,
          project: i % 2 ? 'git-workspace' : 'autowonder-executor-manager',
          squad: '后端与观测小队',
        },
      })),
    })
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoOverview(page)

    const m = await page.evaluate(() => {
      const wrap = document.querySelector('.awem-table-wrap') as HTMLElement
      const actions = wrap.querySelector('.awem-row-actions') as HTMLElement | null
      const lastBtn = actions?.querySelector('button:last-child') as HTMLElement | null
      return {
        overflowPx: wrap.scrollWidth - wrap.clientWidth,
        lastActionRight: lastBtn ? lastBtn.getBoundingClientRect().right : 0,
        wrapRight: wrap.getBoundingClientRect().right,
      }
    })
    expect(m.overflowPx, '联动 11 列应整体放入内容区，无需横向滚动').toBeLessThanOrEqual(0)
    expect(m.lastActionRight, '操作列最后一个按钮应完整可见').toBeLessThanOrEqual(m.wrapRight)
  })

  test('树形分组：项目/小队/重复叶子/未分配小队/未归属最后/在线徽标/默认展开', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await enterTree(page)

    // 项目组按中文升序，未归属固定最后
    const projects = await page.locator('tr.awem-tree-group[data-level="1"] .awem-tree-name').allTextContents()
    expect(projects).toEqual([...['阿项目', '波项目'].sort((a, b) => coll.compare(a, b)), '未归属'])

    // 阿项目下小队组按中文升序（含未分配小队）
    const squads = await page
      .locator('tr.awem-tree-group[data-level="2"][data-project="阿项目"] .awem-tree-name')
      .allTextContents()
    expect(squads).toEqual(['甲小队', '未分配小队', '乙小队'].sort((a, b) => coll.compare(a, b)))

    // 多小队执行器重复叶子：A 同时出现在甲、乙小队
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' })).toHaveCount(2)
    await expect(
      page.locator('tr.awem-tree-leaf[data-project="阿项目"][data-squad="甲小队"]'),
    ).toHaveCount(1)
    await expect(
      page.locator('tr.awem-tree-leaf[data-project="阿项目"][data-squad="乙小队"]'),
    ).toHaveCount(2) // A + B

    // 组行徽标：在线=平台 ONLINE 或 BUSY；未归属显示成员数与 —
    const badge = (group: string) =>
      page.locator('tr.awem-tree-group', { hasText: group }).locator('.awem-tree-badge')
    await expect(badge('甲小队')).toHaveText('1/1')
    await expect(badge('乙小队')).toHaveText('2/2')
    await expect(badge('未分配小队')).toHaveText('0/1')
    await expect(badge('未归属')).toHaveText('—/1')

    // 未归属排最后
    const groups = page.locator('tr.awem-tree-group[data-level="1"]')
    await expect(groups.last()).toContainText('未归属')

    // 叶子仍显示所属项目/小队
    const leafA = page.locator('tr.awem-tree-leaf[data-project="阿项目"][data-squad="甲小队"]').first()
    await expect(leafA).toContainText('阿项目')
    await expect(leafA).toContainText('甲小队')

    // 默认全展开；收起项目后切换平铺/树形仍记忆，刷新后仍记忆
    await page.getByRole('button', { name: '收起 阿项目' }).click()
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' })).toHaveCount(0)
    await page.getByRole('button', { name: '平铺' }).click()
    await enterTree(page)
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()
    await page.reload()
    await enterTree(page)
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()
    await page.getByRole('button', { name: '展开 阿项目' }).click()
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' })).toHaveCount(2)
  })

  test('重复叶子选择同步、组行全选/半选、已选去重与批量请求去重', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await enterTree(page)

    // 勾选任一重复叶子 → 两处同步，已选按执行器去重
    await page.getByRole('checkbox', { name: '选择执行器 A' }).first().check()
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' }).nth(1)).toBeChecked()
    await expect(bulkBar(page)).toContainText('已选 1 项')

    // 组行全选：乙小队（A+B），随后项目（A+B+C），未归属（D）
    await page.getByRole('checkbox', { name: '全选小队 乙小队' }).check()
    await expect(bulkBar(page)).toContainText('已选 2 项')
    await page.getByRole('checkbox', { name: '全选项目 阿项目' }).check()
    await expect(bulkBar(page)).toContainText('已选 3 项')
    await page.getByRole('checkbox', { name: '全选未归属' }).check()
    await expect(bulkBar(page)).toContainText('已选 4 项')

    // 取消一个重复叶子 → 另一处同步取消，组行呈半选
    await page.getByRole('checkbox', { name: '选择执行器 A' }).first().uncheck()
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' }).nth(1)).not.toBeChecked()
    await expect(bulkBar(page)).toContainText('已选 3 项')
    await expect(page.getByRole('checkbox', { name: '全选项目 阿项目' })).toHaveJSProperty('indeterminate', true)

    // 批量启动按执行器标识去重（A 虽有两个叶子，只调用一次）
    await page.getByRole('checkbox', { name: '选择执行器 A' }).first().check()
    await page.getByRole('checkbox', { name: '全选项目 阿项目' }).check()
    await page.getByRole('checkbox', { name: '全选未归属' }).check()
    await bulkBar(page).getByRole('button', { name: '启动' }).click()
    await expect(page.locator('.awem-toast', { hasText: '已启动 4 个执行器' })).toBeVisible()
    for (const name of ['A', 'B', 'C', 'D']) {
      expect(await callsTo(request, 'POST', `/api/executors/${name}/start`)).toHaveLength(1)
    }
  })

  test('加载骨架屏：列表返回前显示局部骨架', async ({ page, request }) => {
    await setMock(request, { executors: TREE_EXECS, listDelayMs: 800 })
    await page.goto('/')
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect(page.locator('.awem-skeleton').first()).toBeVisible()
    await expect(page.locator('.awem-table')).toBeVisible({ timeout: 5_000 })
  })

  test('选择在平铺⇄树形与 5s 轮询间保持', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await enterTree(page)
    await page.getByRole('checkbox', { name: '选择执行器 A' }).first().check()
    await page.getByRole('checkbox', { name: '选择执行器 B' }).check()
    await expect(bulkBar(page)).toContainText('已选 2 项')

    await page.getByRole('button', { name: '平铺' }).click()
    await expect(bulkBar(page)).toContainText('已选 2 项')
    await expect(rowCheckbox(page, 'A')).toBeChecked()
    await expect(rowCheckbox(page, 'B')).toBeChecked()

    await enterTree(page)
    await expect(bulkBar(page)).toContainText('已选 2 项')
    await expect(page.getByRole('checkbox', { name: '选择执行器 A' }).first()).toBeChecked()

    // 轮询刷新后选择保持（Mock 返回同集合）
    await setMock(request, { executors: TREE_EXECS })
    await expect.poll(
      async () => (await mockRequests(request)).filter((r) => r.path === '/api/executors').length,
      { timeout: 7_000 },
    ).toBeGreaterThan(1)
    await expect(bulkBar(page)).toContainText('已选 2 项')
  })
})
