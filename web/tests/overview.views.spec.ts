// overview.views.spec.ts：@views 总览左树右表改造（2026-10-27）——
// 无 Tab 单状态、导航树分组/中文排序/徽标/未归属最后、点击项目/小队过滤表格、
// 多小队执行器去重、树选择清空已选、折叠记忆（sessionStorage）、节点失效回退。
import { expect } from '@playwright/test'
import { test } from './fixtures'
import {
  bulkBar,
  callsTo,
  gotoOverview,
  mockRequests,
  navNode,
  navTree,
  resetMock,
  row,
  rowCheckbox,
  setMock,
  toast,
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

/** 顶层节点名序列：全部执行器 → 各项目 → 未归属（小队不重复计入）。 */
function topLevelNames(page: Parameters<typeof navTree>[0]) {
  return page
    .locator('.awem-navtree > ul > li > .awem-navtree-node, .awem-navtree > ul > li > .awem-navtree-row > .awem-navtree-node')
    .locator('.awem-navtree-name')
    .allTextContents()
}

function rowCheckboxes(page: Parameters<typeof navTree>[0]) {
  return page.getByRole('checkbox', { name: /^选择执行器 / })
}

test.describe('@views 总览左树右表', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('单状态：无 Tab 框架、URL 无 #tab hash、默认选中全部执行器', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await expect(page.getByRole('tab')).toHaveCount(0)
    await expect(page.getByRole('tablist')).toHaveCount(0)
    await expect(page.getByRole('group', { name: '展示方式' })).toHaveCount(0)
    expect(new URL(page.url()).hash).toBe('')

    await expect(navTree(page)).toBeVisible()
    await expect(navNode(page, '全部执行器')).toHaveAttribute('aria-current', 'true')
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(5)

    await page.reload()
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    expect(new URL(page.url()).hash).toBe('')
    await expect(navNode(page, '全部执行器')).toHaveAttribute('aria-current', 'true')
  })

  test('无归属数据时隐藏左树、表格占满', async ({ page, request }) => {
    await gotoOverview(page)
    await expect(navTree(page)).toHaveCount(0)
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(4)

    // 有平台状态但没有任何项目/小队归属 → 仍隐藏左树
    await setMock(request, {
      executors: [
        { name: 'U1', state: 'stopped', platform: { status: 'ONLINE', name: '执行器U1' } },
        { name: 'U2', state: 'running', platform: { status: 'OFFLINE', name: '执行器U2' } },
      ],
    })
    await page.reload()
    await expect(page.getByRole('link', { name: '执行器U1' })).toBeVisible()
    await expect(navTree(page)).toHaveCount(0)
  })

  test('导航树结构：项目/小队中文排序、徽标、未归属最后、默认展开', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)

    // 顶层：全部执行器 → 项目中文升序 → 未归属固定最后
    const tops = await topLevelNames(page)
    expect(tops).toEqual(['全部执行器', ...['阿项目', '波项目'].sort((a, b) => coll.compare(a, b)), '未归属'])

    // 阿项目下小队按中文升序（含虚拟未分配小队）
    const squads = await page
      .locator('.awem-navtree-node[data-project="阿项目"][data-squad] .awem-navtree-name')
      .allTextContents()
    expect(squads).toEqual(['甲小队', '未分配小队', '乙小队'].sort((a, b) => coll.compare(a, b)))

    // 徽标：在线=平台 ONLINE 或 BUSY；未归属显示 —/成员数
    await expect(navNode(page, '全部执行器').locator('.awem-tree-badge')).toHaveText('3/5')
    await expect(navNode(page, '阿项目').locator('.awem-tree-badge')).toHaveText('2/3')
    await expect(navNode(page, '波项目').locator('.awem-tree-badge')).toHaveText('1/1')
    await expect(navNode(page, '甲小队').locator('.awem-tree-badge')).toHaveText('1/1')
    await expect(navNode(page, '乙小队').locator('.awem-tree-badge')).toHaveText('2/2')
    await expect(navNode(page, '未分配小队').locator('.awem-tree-badge')).toHaveText('0/1')
    await expect(navNode(page, '未归属').locator('.awem-tree-badge')).toHaveText('—/1')

    // 默认全展开：小队节点直接可见
    await expect(navNode(page, '甲小队')).toBeVisible()
  })

  test('点击项目/小队过滤表格：多小队执行器去重且在各小队视图出现', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)

    // 默认全部：A 虽属甲乙两个小队，表格仅一行
    await expect(row(page, 'A')).toHaveCount(1)
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(5)

    await navNode(page, '阿项目').click()
    await expect(navNode(page, '阿项目')).toHaveAttribute('aria-current', 'true')
    await expect(rowCheckboxes(page)).toHaveCount(3) // A + B + C（去重）

    await navNode(page, '甲小队').click()
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'A')).toBeVisible()

    await navNode(page, '乙小队').click()
    await expect(rowCheckboxes(page)).toHaveCount(2) // A + B
    await expect(rowCheckbox(page, 'A')).toBeVisible()
    await expect(rowCheckbox(page, 'B')).toBeVisible()

    await navNode(page, '未分配小队').click()
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'C')).toBeVisible()

    await navNode(page, '未归属').click()
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'D')).toBeVisible()

    await navNode(page, '全部执行器').click()
    await expect(rowCheckboxes(page)).toHaveCount(5)

    // 树选择参与计数提示
    await navNode(page, '阿项目').click()
    await expect(page.locator('.awem-table-toolbar .awem-hint')).toHaveText('匹配 3 / 共 5 个执行器')
  })

  test('树选择清空已选；批量操作在小队视图按标识去重调用', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)

    await rowCheckbox(page, 'A').check()
    await rowCheckbox(page, 'B').check()
    await expect(bulkBar(page)).toContainText('已选 2 项')

    // 切换树节点 = 切换过滤范围，清空已选
    await navNode(page, '乙小队').click()
    await expect(bulkBar(page)).toHaveCount(0)

    // 小队视图全选（A+B）后批量启动：按标识各调用一次
    await page.getByRole('checkbox', { name: '全选执行器' }).check()
    await expect(bulkBar(page)).toContainText('已选 2 项')
    await bulkBar(page).getByRole('button', { name: '启动' }).click()
    await expect(toast(page, '已启动 2 个执行器')).toBeVisible()
    expect(await callsTo(request, 'POST', '/api/executors/A/start')).toHaveLength(1)
    expect(await callsTo(request, 'POST', '/api/executors/B/start')).toHaveLength(1)
  })

  test('折叠项目记忆于会话内，刷新后保持', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)

    await page.getByRole('button', { name: '收起 阿项目' }).click()
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()
    await expect(navNode(page, '甲小队')).toHaveCount(0)

    await page.reload()
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()
    await expect(navNode(page, '甲小队')).toHaveCount(0)

    await page.getByRole('button', { name: '展开 阿项目' }).click()
    await expect(navNode(page, '甲小队')).toBeVisible()
  })

  test('树选中节点随数据刷新失效时回退全部执行器', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await navNode(page, '波项目').click()
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'E')).toBeVisible()

    await setMock(request, { executors: TREE_EXECS.filter((e) => e.platform?.project !== '波项目') })
    await expect(navNode(page, '全部执行器'), '波项目消失后应回退全部')
      .toHaveAttribute('aria-current', 'true', { timeout: 8_000 })
    await expect(rowCheckboxes(page)).toHaveCount(4)
  })

  test('选择在 5s 轮询间保持', async ({ page, request }) => {
    await setTree(request)
    await gotoOverview(page)
    await rowCheckbox(page, 'A').check()
    await rowCheckbox(page, 'B').check()
    await expect(bulkBar(page)).toContainText('已选 2 项')

    await setMock(request, { executors: TREE_EXECS })
    await expect.poll(
      async () => (await mockRequests(request)).filter((r) => r.path === '/api/executors').length,
      { timeout: 7_000 },
    ).toBeGreaterThan(1)
    await expect(bulkBar(page)).toContainText('已选 2 项')
    await expect(rowCheckbox(page, 'A')).toBeChecked()
    await expect(rowCheckbox(page, 'B')).toBeChecked()
  })

  test('联动 11 列（含版本列）在宽视口无横向溢出，操作列完整可见', async ({ page, request }) => {
    // 名称/小队取接近生产数据的长度；左树占据 280px，主区内容宽相应收窄
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
          squad_names: ['后端与观测小队'],
        },
      })),
    })
    await page.setViewportSize({ width: 1600, height: 900 })
    await gotoOverview(page)
    await expect(navTree(page)).toBeVisible()

    const m = await page.evaluate(() => {
      const wrap = document.querySelector('.awem-overview-table-wrap') as HTMLElement
      const actions = wrap.querySelector('.awem-row-actions') as HTMLElement | null
      const lastBtn = actions?.querySelector('button:last-child') as HTMLElement | null
      return {
        overflowPx: wrap.scrollWidth - wrap.clientWidth,
        lastActionRight: lastBtn ? lastBtn.getBoundingClientRect().right : 0,
        wrapRight: wrap.getBoundingClientRect().right,
      }
    })
    expect(m.overflowPx, '联动 11 列应整体放入主区，无需横向滚动').toBeLessThanOrEqual(0)
    expect(m.lastActionRight, '操作列最后一个按钮应完整可见').toBeLessThanOrEqual(m.wrapRight)
  })

  test('加载骨架屏：列表返回前显示局部骨架', async ({ page, request }) => {
    await setMock(request, { executors: TREE_EXECS, listDelayMs: 800 })
    await page.goto('/')
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect(page.locator('.awem-skeleton').first()).toBeVisible()
    await expect(page.locator('.awem-table')).toBeVisible({ timeout: 5_000 })
  })
})
