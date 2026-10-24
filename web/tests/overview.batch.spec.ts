// overview.batch.spec.ts：@batch 本次新增的批量选择/启动/停止/删除目标行为。
// 先在旧实现上运行应失败（TDD 负对照），实现后必须全部通过；不含旧行为断言。
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
  selectAllCheckbox,
  setMock,
} from './helpers'

async function selectRows(page: Page, names: string[]) {
  for (const name of names) await rowCheckbox(page, name).check()
}

test.describe('@batch 总览页多选批量操作', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('首列复选框：全选/清空与半选态', async ({ page }) => {
    await gotoOverview(page)

    const head = selectAllCheckbox(page)
    await expect(head).toBeVisible()
    await expect(head).not.toBeChecked()
    for (const name of ['Alpha', 'Beta', 'Gamma', 'Delta']) {
      await expect(rowCheckbox(page, name)).toBeVisible()
      await expect(rowCheckbox(page, name)).not.toBeChecked()
    }

    await rowCheckbox(page, 'Alpha').check()
    await expect(head).not.toBeChecked()
    await expect(head).toHaveJSProperty('indeterminate', true)
    await expect(bulkBar(page)).toContainText('已选 1 项')

    await head.click()
    for (const name of ['Alpha', 'Beta', 'Gamma', 'Delta']) {
      await expect(rowCheckbox(page, name)).toBeChecked()
    }
    await expect(head).toBeChecked()
    await expect(head).toHaveJSProperty('indeterminate', false)
    await expect(bulkBar(page)).toContainText('已选 4 项')

    await head.click()
    for (const name of ['Alpha', 'Beta', 'Gamma', 'Delta']) {
      await expect(rowCheckbox(page, name)).not.toBeChecked()
    }
    await expect(bulkBar(page)).toHaveCount(0)
  })

  test('批量条显隐与取消选择', async ({ page }) => {
    await gotoOverview(page)
    await expect(bulkBar(page)).toHaveCount(0)

    await selectRows(page, ['Beta', 'Gamma'])
    await expect(bulkBar(page)).toBeVisible()
    await expect(bulkBar(page)).toContainText('已选 2 项')
    for (const label of ['启动', '停止', '删除', '取消选择']) {
      await expect(bulkBar(page).getByRole('button', { name: label })).toBeVisible()
    }

    await bulkBar(page).getByRole('button', { name: '取消选择' }).click()
    await expect(bulkBar(page)).toHaveCount(0)
    await expect(rowCheckbox(page, 'Beta')).not.toBeChecked()
    await expect(rowCheckbox(page, 'Gamma')).not.toBeChecked()
  })

  test('批量启动：无确认、串行调用、已运行计成功并汇总 Toast', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Alpha', 'Beta', 'Gamma'])
    const requestsBefore = (await mockRequests(request)).length

    await bulkBar(page).getByRole('button', { name: '启动' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.locator('.awem-toast', { hasText: '已启动 3 个执行器' })).toBeVisible()

    const calls = (await mockRequests(request))
      .slice(requestsBefore)
      .filter((r) => r.method === 'POST' && r.path.endsWith('/start'))
    expect(calls.map((c) => c.path)).toEqual([
      '/api/executors/Alpha/start',
      '/api/executors/Beta/start',
      '/api/executors/Gamma/start',
    ])
    for (const name of ['Alpha', 'Beta', 'Gamma']) {
      await expect(row(page, name).getByText('运行中')).toBeVisible()
    }
    await expect(bulkBar(page)).toHaveCount(0)
    await expect(rowCheckbox(page, 'Alpha')).not.toBeChecked()
  })

  test('批量停止：确认文案具体，取消不执行，确认后串行停止', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Alpha', 'Beta'])

    await bulkBar(page).getByRole('button', { name: '停止' }).click()
    const dialog = page.getByRole('dialog', { name: '停止 2 个执行器' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('将终止 2 个执行器的子进程（整棵进程树优雅终止）')
    await expect(dialog.getByRole('button', { name: '停止 2 个执行器' })).toBeVisible()
    expect(await callsTo(request, 'POST', '/api/executors/Alpha/stop')).toHaveLength(0)

    await dialog.getByRole('button', { name: '取消' }).click()
    await expect(dialog).toBeHidden()
    expect(await callsTo(request, 'POST', '/api/executors/Alpha/stop')).toHaveLength(0)
    await expect(bulkBar(page)).toContainText('已选 2 项')

    await bulkBar(page).getByRole('button', { name: '停止' }).click()
    await page.getByRole('dialog', { name: '停止 2 个执行器' }).getByRole('button', { name: '停止 2 个执行器' }).click()
    await expect(page.locator('.awem-toast', { hasText: '已停止 2 个执行器' })).toBeVisible()

    const calls = (await mockRequests(request)).filter((r) => r.method === 'POST' && r.path.endsWith('/stop'))
    expect(calls.map((c) => c.path)).toEqual([
      '/api/executors/Alpha/stop',
      '/api/executors/Beta/stop',
    ])
    for (const name of ['Alpha', 'Beta']) {
      await expect(row(page, name).getByText('已停止')).toBeVisible()
    }
    await expect(bulkBar(page)).toHaveCount(0)
  })

  test('批量删除（无运行中）：确认正文含数量/不可恢复/日志保留/名称，删除带 stop=false&purge=false', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Beta', 'Gamma'])

    await bulkBar(page).getByRole('button', { name: '删除' }).click()
    const dialog = page.getByRole('dialog', { name: '删除 2 个执行器' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('将删除选中的 2 个执行器')
    await expect(dialog).toContainText('此操作不可恢复')
    await expect(dialog).toContainText('本地日志保留')
    await expect(dialog).toContainText('Beta、Gamma')
    await expect(dialog.getByRole('button', { name: '删除 2 个执行器' })).toBeVisible()
    expect(await callsTo(request, 'DELETE', '/api/executors/Beta')).toHaveLength(0)

    await dialog.getByRole('button', { name: '删除 2 个执行器' }).click()
    await expect(page.locator('.awem-toast', { hasText: '已删除 2 个执行器' })).toBeVisible()

    for (const name of ['Beta', 'Gamma']) {
      const calls = await callsTo(request, 'DELETE', `/api/executors/${name}`)
      expect(calls).toHaveLength(1)
      expect(calls[0].query).toEqual({ stop: 'false', purge: 'false' })
      await expect(row(page, name)).toHaveCount(0)
    }
    await expect(bulkBar(page)).toHaveCount(0)
  })

  test('批量删除（含运行中）：整批阻止、不弹框、无删除调用，并截断名称', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Alpha', 'Beta'])

    await bulkBar(page).getByRole('button', { name: '删除' }).click()
    await expect(page.locator('.awem-toast.error', { hasText: '有 1 个运行中，请先停止后再删除：Alpha' })).toBeVisible()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(await callsTo(request, 'DELETE', '/api/executors/Alpha')).toHaveLength(0)
    expect(await callsTo(request, 'DELETE', '/api/executors/Beta')).toHaveLength(0)
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(4)
    await expect(bulkBar(page)).toContainText('已选 2 项')
  })

  test('批量删除含多个运行中：提示列出前 3 个并注明总数', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'R1', state: 'running' },
        { name: 'R2', state: 'backoff' },
        { name: 'R3', state: 'stopping' },
        { name: 'R4', state: 'running' },
        { name: 'S1', state: 'stopped' },
      ],
    })
    await gotoOverview(page)
    await selectAllCheckbox(page).check()

    await bulkBar(page).getByRole('button', { name: '删除' }).click()
    await expect(
      page.locator('.awem-toast.error', { hasText: '有 4 个运行中，请先停止后再删除：R1、R2、R3 等 4 个' }),
    ).toBeVisible()
    await expect(page.getByRole('dialog')).toHaveCount(0)
  })

  test('批量删除二次校验：确认前变为运行中仍整批阻止', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Beta', 'Gamma'])

    await bulkBar(page).getByRole('button', { name: '删除' }).click()
    const dialog = page.getByRole('dialog', { name: '删除 2 个执行器' })
    await expect(dialog).toBeVisible()

    await setMock(request, {
      executors: [
        { name: 'Alpha', state: 'running', autostart: true },
        { name: 'Beta', state: 'running' },
        { name: 'Gamma', state: 'failed', restarts: 3 },
        { name: 'Delta', state: 'backoff' },
      ],
    })

    await dialog.getByRole('button', { name: '删除 2 个执行器' }).click()
    await expect(page.locator('.awem-toast.error', { hasText: '有 1 个运行中，请先停止后再删除：Beta' })).toBeVisible()
    await expect(dialog).toBeHidden()
    expect(await callsTo(request, 'DELETE', '/api/executors/Beta')).toHaveLength(0)
    expect(await callsTo(request, 'DELETE', '/api/executors/Gamma')).toHaveLength(0)
    await expect(row(page, 'Beta')).toHaveCount(1)
  })

  test('部分失败：成功项取消勾选、失败项保留并区分 404/409/其他原因', async ({ page, request }) => {
    await setMock(request, {
      failures: {
        'start:Beta': { status: 404, code: 'not_found', message: '执行器 Beta 不存在' },
        'start:Gamma': { status: 409, code: 'running', message: '执行器 Gamma 运行中' },
        'start:Delta': { status: 500, code: 'internal', message: '模拟启动失败' },
      },
    })
    await gotoOverview(page)
    await selectRows(page, ['Alpha', 'Beta', 'Gamma', 'Delta'])

    await bulkBar(page).getByRole('button', { name: '启动' }).click()
    await expect(
      page.locator('.awem-toast.error', {
        hasText: '已启动 1 个执行器，3 个失败：Beta（不存在）、Gamma（运行中冲突）、Delta（模拟启动失败）',
      }),
    ).toBeVisible()

    await expect(rowCheckbox(page, 'Alpha')).not.toBeChecked()
    for (const name of ['Beta', 'Gamma', 'Delta']) {
      await expect(rowCheckbox(page, name)).toBeChecked()
    }
    await expect(bulkBar(page)).toContainText('已选 3 项')
    await expect(page.locator('.awem-table tbody tr')).toHaveCount(4)
  })

  test('批量执行期间按钮与勾选禁用，轮询继续刷新', async ({ page, request }) => {
    await setMock(request, { actionDelayMs: 5_500 })
    await gotoOverview(page)
    await rowCheckbox(page, 'Beta').check()

    const listBefore = (await mockRequests(request)).filter((r) => r.path === '/api/executors').length
    await bulkBar(page).getByRole('button', { name: '启动' }).click()

    await expect(selectAllCheckbox(page)).toBeDisabled()
    await expect(rowCheckbox(page, 'Beta')).toBeDisabled()
    await expect(bulkBar(page).getByRole('button', { name: '启动' })).toBeDisabled()
    await expect(bulkBar(page).getByRole('button', { name: '取消选择' })).toBeDisabled()
    await expect(row(page, 'Alpha').getByRole('button', { name: '启动 Alpha' })).toBeDisabled()

    await expect
      .poll(async () => (await mockRequests(request)).filter((r) => r.path === '/api/executors').length, {
        timeout: 7_000,
      })
      .toBeGreaterThan(listBefore)

    await expect(page.locator('.awem-toast', { hasText: '已启动 1 个执行器' })).toBeVisible({ timeout: 10_000 })
    await expect(selectAllCheckbox(page)).toBeEnabled()
  })

  test('5s 轮询保持有效选择：消失行移出、新行不选中', async ({ page, request }) => {
    await gotoOverview(page)
    await selectRows(page, ['Alpha', 'Beta'])
    await expect(bulkBar(page)).toContainText('已选 2 项')

    await setMock(request, {
      executors: [
        { name: 'Alpha', state: 'running', autostart: true },
        { name: 'Gamma', state: 'failed', restarts: 3 },
        { name: 'Delta', state: 'backoff' },
        { name: 'Epsilon', state: 'stopped' },
      ],
    })
    await expect(rowCheckbox(page, 'Epsilon')).toBeVisible({ timeout: 8_000 })

    await expect(bulkBar(page)).toContainText('已选 1 项')
    await expect(rowCheckbox(page, 'Alpha')).toBeChecked()
    await expect(rowCheckbox(page, 'Epsilon')).not.toBeChecked()
    await expect(rowCheckbox(page, 'Beta')).toHaveCount(0)
  })

  test('批量删除确认正文名称超 10 个时截断', async ({ page, request }) => {
    await setMock(request, {
      executors: Array.from({ length: 12 }, (_, i) => ({
        name: `E${String(i + 1).padStart(2, '0')}`,
        state: 'stopped',
      })),
    })
    await gotoOverview(page)
    await selectAllCheckbox(page).check()

    await bulkBar(page).getByRole('button', { name: '删除' }).click()
    const dialog = page.getByRole('dialog', { name: '删除 12 个执行器' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('将删除选中的 12 个执行器')
    await expect(dialog).toContainText('E10')
    await expect(dialog).toContainText('等 12 个')
    await expect(dialog).not.toContainText('E11')
    await expect(dialog.getByRole('button', { name: '删除 12 个执行器' })).toBeVisible()
    await dialog.getByRole('button', { name: '取消' }).click()
  })
})
