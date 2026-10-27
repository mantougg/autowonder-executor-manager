// overview.filters.spec.ts：@filters 总览四项查询条件（名称/项目/小队/本地进程）。
// 覆盖：展示与选项、名称匹配、AND 组合、计数、空态与清除、选择清理/全选范围、
// 树形裁剪与折叠恢复、无归属隐藏、轮询/切换保持、选项失效回退。
import { expect, type Page } from '@playwright/test'
import { test } from './fixtures'
import {
  mockRequests,
  gotoOverview,
  resetMock,
  rowCheckbox,
  selectAllCheckbox,
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

function rowCheckboxes(page: Page) {
  return page.getByRole('checkbox', { name: /^选择执行器 / })
}

function countHint(page: Page) {
  return page.locator('.awem-table-toolbar .awem-hint')
}

async function applyFilter(page: Page, label: string, value: string) {
  await page.getByLabel(label).selectOption({ label: value })
}

async function resetFilters(page: Page) {
  await page.getByRole('button', { name: '重置' }).click()
}

test.describe('@filters 总览四项查询条件', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
    await setMock(request, { executors: FILTER_EXECS })
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('筛选行展示四个条件、选项与默认计数', async ({ page }) => {
    await gotoOverview(page)

    const bar = page.getByRole('group', { name: '查询条件' })
    await expect(bar).toBeVisible()
    await expect(page.getByLabel('名称')).toBeVisible()
    await expect(page.getByLabel('所属项目')).toBeVisible()
    await expect(page.getByLabel('所属小队')).toBeVisible()
    await expect(page.getByLabel('本地进程')).toBeVisible()
    await expect(page.getByRole('button', { name: '重置' })).toBeVisible()
    await expect(countHint(page)).toHaveText('共 6 个执行器')

    const projects = await page.getByLabel('所属项目').locator('option').allTextContents()
    expect(projects[0]).toBe('全部')
    expect(projects).toContain('git-workspace')
    expect(projects).toContain('autowonder-executor-manager')
    expect(projects[projects.length - 1]).toBe('未归属')

    const squads = await page.getByLabel('所属小队').locator('option').allTextContents()
    expect(squads[0]).toBe('全部')
    expect(squads).toContain('快速交付')
    expect(squads).toContain('标准自动化交付')
    expect(squads).toContain('未分配小队')

    const locals = await page.getByLabel('本地进程').locator('option').allTextContents()
    expect(locals).toEqual(['全部', '运行中', '重连中', '停止中', '已停止', '失败'])

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

  test('项目/小队/本地进程条件及 AND 组合', async ({ page }) => {
    await gotoOverview(page)

    await applyFilter(page, '所属项目', 'git-workspace')
    await expect(rowCheckboxes(page)).toHaveCount(2)

    await applyFilter(page, '所属小队', '快速交付')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'exec-10001')).toBeVisible()

    await applyFilter(page, '本地进程', '运行中')
    await expect(rowCheckboxes(page)).toHaveCount(1)

    await applyFilter(page, '本地进程', '已停止')
    await expect(rowCheckboxes(page)).toHaveCount(0)
    await expect(page.getByText('没有匹配的执行器')).toBeVisible()

    await resetFilters(page)
    await applyFilter(page, '所属项目', '未归属')
    await expect(rowCheckboxes(page)).toHaveCount(2)

    await applyFilter(page, '所属小队', '未分配小队')
    await expect(rowCheckboxes(page)).toHaveCount(2)

    await applyFilter(page, '本地进程', '停止中')
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

  test('树形按命中裁剪、自动展开、空组隐藏、清空恢复折叠', async ({ page, request }) => {
    await setMock(request, { executors: TREE_EXECS })
    await gotoOverview(page)
    await page.getByRole('button', { name: '树形' }).click()

    // 先收起阿项目，制造筛选前的折叠记忆
    await page.getByRole('button', { name: '收起 阿项目' }).click()
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()

    await page.getByLabel('名称').fill('执行器A')
    // 命中路径自动展开；只保留阿项目→甲/乙小队→A（重复叶子）
    await expect(page.getByRole('button', { name: '收起 阿项目' })).toBeVisible()
    await expect(page.locator('tr.awem-tree-group[data-level="1"] .awem-tree-name')).toHaveText(['阿项目'])
    await expect(rowCheckboxes(page)).toHaveCount(2)
    await expect(page.locator('tr.awem-tree-group', { hasText: '未分配小队' })).toHaveCount(0)
    await expect(page.locator('tr.awem-tree-group', { hasText: '波项目' })).toHaveCount(0)
    await expect(page.locator('tr.awem-tree-group', { hasText: '未归属' })).toHaveCount(0)

    const badge = (group: string) =>
      page.locator('tr.awem-tree-group', { hasText: group }).locator('.awem-tree-badge')
    await expect(badge('阿项目')).toHaveText('1/1')
    await expect(badge('甲小队')).toHaveText('1/1')
    await expect(badge('乙小队')).toHaveText('1/1')

    // 筛选期间折叠操作不改变统一展开，也不写入折叠记忆
    await page.getByRole('button', { name: '收起 阿项目' }).click()
    await expect(page.getByRole('button', { name: '收起 阿项目' })).toBeVisible()
    await expect(rowCheckboxes(page)).toHaveCount(2)

    await resetFilters(page)
    await expect(page.getByRole('button', { name: '展开 阿项目' })).toBeVisible()
    await page.getByRole('button', { name: '展开 阿项目' }).click()
    await expect(badge('甲小队')).toHaveText('1/1')
    await expect(badge('乙小队')).toHaveText('2/2')
    await expect(badge('未归属')).toHaveText('—/1')
  })

  test('无归属数据时隐藏项目/小队条件，名称与本地进程保留', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'U1', state: 'stopped', platform: { status: 'ONLINE', name: '执行器U1' } },
        { name: 'U2', state: 'running', platform: { status: 'OFFLINE', name: '执行器U2' } },
      ],
    })
    await gotoOverview(page)
    await expect(page.getByRole('group', { name: '查询条件' })).toBeVisible()
    await expect(page.getByLabel('名称')).toBeVisible()
    await expect(page.getByLabel('本地进程')).toBeVisible()
    await expect(page.getByLabel('所属项目')).toHaveCount(0)
    await expect(page.getByLabel('所属小队')).toHaveCount(0)

    await page.getByLabel('名称').fill('U2')
    await expect(rowCheckboxes(page)).toHaveCount(1)
  })

  test('筛选跨平铺/树形与轮询保持，刷新重置', async ({ page, request }) => {
    await setMock(request, { executors: TREE_EXECS })
    await gotoOverview(page)
    await page.getByRole('button', { name: '树形' }).click()
    await applyFilter(page, '所属项目', '阿项目')
    await expect(rowCheckboxes(page)).toHaveCount(4) // A 重复两次 + B + C
    await expect(countHint(page)).toHaveText('匹配 3 / 共 5 个执行器')

    const before = (await mockRequests(request)).filter((r) => r.path === '/api/executors').length
    await expect.poll(
      async () => (await mockRequests(request)).filter((r) => r.path === '/api/executors').length,
      { timeout: 7_000 },
    ).toBeGreaterThan(before)
    await expect(page.getByLabel('所属项目').locator('option:checked')).toHaveText('阿项目')

    await page.getByRole('button', { name: '平铺' }).click()
    await expect(page.getByLabel('所属项目').locator('option:checked')).toHaveText('阿项目')
    await expect(rowCheckboxes(page)).toHaveCount(3)
    await expect(countHint(page)).toHaveText('匹配 3 / 共 5 个执行器')

    await page.reload()
    await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
    await expect(page.getByLabel('所属项目').locator('option:checked')).toHaveText('全部')
    await expect(page.getByLabel('名称')).toHaveValue('')
    await expect(rowCheckboxes(page)).toHaveCount(5)
  })

  test('选项随数据刷新更新，失效选中值回退全部', async ({ page, request }) => {
    await gotoOverview(page)
    await applyFilter(page, '所属项目', 'git-workspace')
    await expect(rowCheckboxes(page)).toHaveCount(2)

    await setMock(request, {
      executors: FILTER_EXECS.filter((e) => e.platform?.project !== 'git-workspace'),
    })
    await expect(page.getByLabel('所属项目').locator('option:checked'),
      '选中项目从新数据中消失后应回退全部').toHaveText('全部', { timeout: 8_000 })
    await expect(rowCheckboxes(page)).toHaveCount(4)
    await expect(countHint(page)).toHaveText('共 4 个执行器')
  })

  test('未知本地状态原样进入选项并可按其筛选', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'X', state: 'paused', platform: { status: 'ONLINE', name: '执行器X' } },
        ...FILTER_EXECS,
      ],
    })
    await gotoOverview(page)
    const locals = await page.getByLabel('本地进程').locator('option').allTextContents()
    expect(locals).toContain('paused')

    await applyFilter(page, '本地进程', 'paused')
    await expect(rowCheckboxes(page)).toHaveCount(1)
    await expect(rowCheckbox(page, 'X')).toBeVisible()
  })

  test('无执行器时不显示筛选行', async ({ page, request }) => {
    await setMock(request, { executors: [] })
    await gotoOverview(page)
    await expect(page.getByRole('group', { name: '查询条件' })).toHaveCount(0)
    await expect(page.getByText('还没有执行器')).toBeVisible()
  })
})
