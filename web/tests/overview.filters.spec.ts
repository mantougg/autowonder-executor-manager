// overview.filters.spec.ts：@filters 总览查询条件（名称/本地进程，2026-10-27 起
// 项目/小队条件由左树导航替代）。覆盖：展示与选项、名称匹配、AND 组合、计数、
// 空态与清除、选择清理/全选范围、与左树选择组合、无归属隐藏、轮询保持/刷新重置、
// 本地进程选项失效回退。
import { expect } from '@playwright/test'
import { test } from './fixtures'
import {
  gotoOverview,
  localFilter,
  mockRequests,
  navNode,
  navTree,
  resetMock,
  rowCheckbox,
  selectAllCheckbox,
  setLocalFilter,
  setMock,
} from './helpers'

const FILTER_EXECS = [
  {
    name: 'exec-10001', state: 'running',
    platform: {
      status: 'ONLINE', name: 'Alpha 执行器', agent_name: '工人A',
      project: 'git-workspace', squad: '快速交付', squad_names: ['快速交付'],
    },
  },
  {
    name: 'exec-10002', state: 'stopped',
    platform: {
      status: 'OFFLINE', name: 'Beta 执行器', agent_name: '工人B',
      project: 'git-workspace', squad: '标准自动化交付', squad_names: ['标准自动化交付'],
    },
  },
  {
    name: 'exec-10003', state: 'backoff',
    platform: {
      status: 'BUSY', name: 'Gamma 执行器', agent_name: '工人C',
      project: 'autowonder-executor-manager', squad: '快速交付、标准自动化交付',
      squad_names: ['快速交付', '标准自动化交付'],
    },
  },
  {
    name: 'exec-10004', state: 'failed',
    platform: {
      status: 'OFFLINE', name: 'Delta 执行器', agent_name: '工人D',
      project: 'autowonder-executor-manager', squad: '', squad_names: [],
    },
  },
  {
    name: 'exec-10005', state: 'stopping',
    platform: {
      status: 'ONLINE', name: 'Epsilon 执行器', agent_name: '工人E',
      project: '', squad: '', squad_names: [],
    },
  },
  { name: 'exec-10006', state: 'stopped' },
]

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

function rowCheckboxes(page: Parameters<typeof rowCheckbox>[0]) {
  return page.getByRole('checkbox', { name: /^选择执行器 / })
}

function countHint(page: Parameters<typeof rowCheckbox>[0]) {
  return page.locator('.awem-table-toolbar .awem-hint')
}

async function resetFilters(page: Parameters<typeof rowCheckbox>[0]) {
  await page.getByRole('button', { name: '重置' }).click()
}

test.describe('@filters 总览查询条件', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
    await setMock(request, { executors: FILTER_EXECS })
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('筛选行展示名称/本地进程两个条件与默认计数，项目/小队条件下拉已移除', async ({ page }) => {
    await gotoOverview(page)

    const bar = page.getByRole('group', { name: '查询条件' })
    await expect(bar).toBeVisible()
    await expect(page.getByLabel('名称')).toBeVisible()
    await expect(localFilter(page)).toBeVisible()
    await expect(page.getByLabel('所属项目')).toHaveCount(0)
    await expect(page.getByLabel('所属小队')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '重置' })).toBeVisible()
    await expect(countHint(page)).toHaveText('共 6 个执行器')

    // 自定义 Select：打开弹层核对选项（固定五档常驻），Esc 关闭
    await localFilter(page).click()
    await expect(page.getByRole('option')).toHaveText([
      '全部', '运行中', '重连中', '停止中', '已停止', '失败',
    ])
    await page.keyboard.press('Escape')
    await expect(page.getByRole('option')).toHaveCount(0)

    // 筛选行位于工具行上方，且都不在表格滚动容器内
    const pos = await page.evaluate(() => {
      const barEl = document.querySelector('.awem-filter-bar') as HTMLElement
      const toolbar = document.querySelector('.awem-table-toolbar') as HTMLElement
      const wrap = document.querySelector('.awem-overview-table-wrap') as HTMLElement
      return {
        barBottom: barEl.getBoundingClientRect().bottom,
        toolbarTop: toolbar.getBoundingClientRect().top,
        barInWrap: wrap.contains(barEl),
        toolbarInWrap: wrap.contains(toolbar),
      }
    })
    expect(pos.barBottom).toBeLessThanOrEqual(pos.toolbarTop)
    expect(pos.barInWrap).toBe(false)
    expect(pos.toolbarInWrap).toBe(false)
  })

  test('名称按显示名/标识子串匹配，去空格且不区分大小写', async ({ page }) => {
    await gotoOverview(page)

    await page.getByLabel('名称').fill(' alpha ')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10001')).toBeVisible()

    await page.getByLabel('名称').fill('10002')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10002')).toBeVisible()

    await page.getByLabel('名称').fill('EXEC-10003')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10003')).toBeVisible()

    await page.getByLabel('名称').fill('执行器')
    await expect(rowCheckboxes(page)).toHaveCount(5)

    await page.getByLabel('名称').fill('zzz')
    await expect(rowCheckboxes(page)).toHaveCount(0)
  })

  test('本地进程条件及与名称的 AND 组合', async ({ page }) => {
    await gotoOverview(page)

    await setLocalFilter(page, '运行中')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10001')).toBeVisible()

    await page.getByLabel('名称').fill('执行器')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    await setLocalFilter(page, '已停止')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10002')).toBeVisible()

    await setLocalFilter(page, '重连中')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10003')).toBeVisible()

    await setLocalFilter(page, '失败')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10004')).toBeVisible()

    await page.getByLabel('名称').fill('zzz')
    await expect(page.getByText('没有匹配的执行器')).toBeVisible()
    await expect(rowCheckboxes(page)).toHaveCount(0)

    await resetFilters(page)
    await setLocalFilter(page, '停止中')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10005')).toBeVisible()
  })

  test('计数、无命中空态与清除筛选', async ({ page }) => {
    await gotoOverview(page)
    await expect(countHint(page)).toHaveText('共 6 个执行器')

    await page.getByLabel('名称').fill('Beta')
    await expect(countHint(page)).toHaveText('匹配 1 / 共 6 个执行器')

    await page.getByLabel('名称').fill('zzz')
    await expect(countHint(page)).toHaveText('匹配 0 / 共 6 个执行器')
    await expect(page.getByText('没有匹配的执行器')).toBeVisible()
    await expect(page.locator('.awem-table thead')).toBeVisible()
    await expect(page.getByRole('group', { name: '查询条件' })).toBeVisible()

    await page.getByRole('button', { name: '清除筛选' }).click()
    await expect(countHint(page)).toHaveText('共 6 个执行器')
    await expect(rowCheckboxes(page)).toHaveCount(6)
    await expect(page.getByRole('button', { name: '重置' })).toBeDisabled()
  })

  test('筛选变化清空已选，表头全选仅命中行', async ({ page }) => {
    await gotoOverview(page)
    await rowCheckbox(page, 'exec-10001').check()
    await expect(page.locator('.awem-bulk-bar')).toContainText('已选 1 项')

    await page.getByLabel('名称').fill('Beta')
    await expect(page.locator('.awem-bulk-bar')).toHaveCount(0)
    await expect(countHint(page)).toHaveText('匹配 1 / 共 6 个执行器')

    await selectAllCheckbox(page).check()
    await expect(page.locator('.awem-bulk-bar')).toContainText('已选 1 项')
    await expect(rowCheckbox(page, 'exec-10002')).toBeChecked()
    await expect(rowCheckboxes(page)).toHaveCount(1)

    // 0 命中时表头全选不可用
    await page.getByLabel('名称').fill('zzz')
    await expect(selectAllCheckbox(page)).toBeDisabled()
  })

  test('筛选与左树选择 AND 组合：树不被筛选裁剪', async ({ page, request }) => {
    await setMock(request, { executors: TREE_EXECS })
    await gotoOverview(page)

    // 点项目节点过滤：阿项目 → A/B/C 三台（A 多小队也去重为一行）
    await navNode(page, '阿项目').click()
    await expect(rowCheckboxes(page)).toHaveCount(3)
    await expect(countHint(page)).toHaveText('匹配 3 / 共 5 个执行器')

    // 叠加名称筛选：AND 生效；左树仍展示全量归属结构（不被裁剪）
    await page.getByLabel('名称').fill('执行器A')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'A')).toBeVisible()
    await expect(countHint(page)).toHaveText('匹配 1 / 共 5 个执行器')
    await expect(navNode(page, '波项目')).toBeVisible()
    await expect(navNode(page, '未归属')).toBeVisible()

    // 重置筛选仅清条件，树选择保持
    await resetFilters(page)
    await expect(rowCheckboxes(page)).toHaveCount(3)
    await expect(navNode(page, '阿项目')).toHaveAttribute('aria-current', 'true')
  })

  test('无归属数据时隐藏左树，名称与本地进程筛选保留', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'U1', state: 'stopped', platform: { status: 'ONLINE', name: '执行器U1' } },
        { name: 'U2', state: 'running', platform: { status: 'OFFLINE', name: '执行器U2' } },
      ],
    })
    await gotoOverview(page)
    await expect(navTree(page)).toHaveCount(0)
    await expect(page.getByRole('group', { name: '查询条件' })).toBeVisible()
    await expect(page.getByLabel('名称')).toBeVisible()
    await expect(localFilter(page)).toBeVisible()

    await page.getByLabel('名称').fill('U2')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    await page.getByLabel('名称').fill('')
    await setLocalFilter(page, '已停止')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'U1')).toBeVisible()
  })

  test('筛选跨轮询保持，刷新重置', async ({ page, request }) => {
    await gotoOverview(page)
    await setLocalFilter(page, '运行中')
    await page.getByLabel('名称').fill('Alpha')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    const before = (await mockRequests(request)).filter((r) => r.path === '/api/executors').length
    await expect.poll(
      async () => (await mockRequests(request)).filter((r) => r.path === '/api/executors').length,
      { timeout: 7_000 },
    ).toBeGreaterThan(before)
    await expect(localFilter(page)).toHaveText('运行中')
    await expect(page.getByLabel('名称')).toHaveValue('Alpha')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    await page.reload()
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect(localFilter(page)).toHaveText('全部')
    await expect(page.getByLabel('名称')).toHaveValue('')
    await expect(rowCheckboxes(page)).toHaveCount(6)
  })

  test('本地进程选项随数据刷新更新，失效选中值回退全部', async ({ page, request }) => {
    // 固定五档（运行中/重连中/停止中/已停止/失败）常驻，只有数据驱动的额外状态会失效
    await setMock(request, {
      executors: [
        { name: 'X', state: 'paused', platform: { status: 'ONLINE', name: '执行器X' } },
        ...FILTER_EXECS,
      ],
    })
    await gotoOverview(page)
    await setLocalFilter(page, 'paused')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    await setMock(request, { executors: FILTER_EXECS })
    await expect(localFilter(page), '选中状态从新数据中消失后应回退全部')
      .toHaveText('全部', { timeout: 8_000 })
    await expect(rowCheckboxes(page)).toHaveCount(6)
    await expect(countHint(page)).toHaveText('共 6 个执行器')
  })

  test('未知本地状态原样进入选项并可按其筛选', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'X', state: 'paused', platform: { status: 'ONLINE', name: '执行器X' } },
        ...FILTER_EXECS,
      ],
    })
    await gotoOverview(page)
    await localFilter(page).click()
    await expect(page.getByRole('option', { name: 'paused' })).toHaveCount(1)
    await page.getByRole('option', { name: 'paused' }).click()
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'X')).toBeVisible()
  })

  test('本地进程下拉支持键盘操作与点击外部关闭', async ({ page }) => {
    await gotoOverview(page)

    // ↑/↓ 打开并移动高亮，Enter 选中
    await localFilter(page).press('ArrowDown')
    await expect(page.getByRole('listbox', { name: '本地进程' })).toBeVisible()
    await localFilter(page).press('ArrowDown') // 全部 → 运行中
    await localFilter(page).press('Enter')
    await expect(page.getByRole('listbox')).toHaveCount(0)
    await expect(localFilter(page)).toHaveText('运行中')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    // 点击组件外部仅关闭弹层，不改变选中值
    await localFilter(page).click()
    await expect(page.getByRole('option', { name: '已停止' })).toBeVisible()
    await page.getByRole('heading', { name: '总览' }).click()
    await expect(page.getByRole('listbox')).toHaveCount(0)
    await expect(localFilter(page)).toHaveText('运行中')
  })

  test('无执行器时不显示筛选行', async ({ page, request }) => {
    await setMock(request, { executors: [] })
    await gotoOverview(page)
    await expect(page.getByRole('group', { name: '查询条件' })).toHaveCount(0)
    await expect(page.getByText('还没有执行器')).toBeVisible()
  })
})
