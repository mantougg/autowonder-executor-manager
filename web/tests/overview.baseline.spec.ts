// overview.baseline.spec.ts：@baseline 锁定总览页「修改前」的单条操作旧行为。
// 本文件在功能实现前后都必须通过；仅断言既有行为，不涉及本次新增的批量能力。
import { expect } from '@playwright/test'
import { test } from './fixtures'
import {
  callsTo,
  gotoOverview,
  mockRequests,
  resetMock,
  row,
  setMock,
} from './helpers'

test.describe('@baseline 总览页旧行为', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('表格列与行状态按既有契约渲染（含版本列）', async ({ page }) => {
    await gotoOverview(page)
    const table = page.locator('.awem-table')
    await expect(table.locator('tbody tr')).toHaveCount(4)
    for (const h of ['名称', '版本', '本地进程', '连接', '平台（平台轮询）', '模型', '自启', '重启']) {
      await expect(table.getByRole('columnheader', { name: h })).toBeVisible()
    }
    await expect(table.getByRole('link', { name: 'Alpha' })).toBeVisible()
    await expect(row(page, 'Alpha').getByText('运行中')).toBeVisible()
    await expect(row(page, 'Beta').getByText('已停止')).toBeVisible()
    await expect(row(page, 'Gamma').getByText('失败')).toBeVisible()
    await expect(row(page, 'Delta').getByText('重连中').first()).toBeVisible()
    // 版本列来自 parsed.runtime_version
    await expect(row(page, 'Alpha').locator('td.mono', { hasText: '0.3.22' })).toBeVisible()
    await expect(page.getByRole('button', { name: '添加执行器' })).toBeVisible()
  })

  test('版本解析缺失时版本列回退 —', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        { name: 'Alpha', state: 'running', runtime_version: '0.3.22' },
        { name: 'Legacy', state: 'stopped', runtime_version: null },
      ],
    })
    await gotoOverview(page)
    await expect(row(page, 'Alpha').getByText('0.3.22')).toBeVisible()
    await expect(row(page, 'Legacy').locator('td').nth(2)).toHaveText('—')
  })

  test('单条启动：直接调用 start 并提示成功', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Beta')
    await r.hover()
    await r.getByRole('button', { name: '启动 Beta' }).click()
    await expect(page.locator('.awem-toast', { hasText: 'Beta 已启动' })).toBeVisible()
    expect(await callsTo(request, 'POST', '/api/executors/Beta/start')).toHaveLength(1)
    await expect(row(page, 'Beta').getByText('运行中')).toBeVisible()
  })

  test('单条重启：直接调用 restart 并提示成功', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Alpha')
    await r.hover()
    await r.getByRole('button', { name: '重启 Alpha' }).click()
    await expect(page.locator('.awem-toast', { hasText: 'Alpha 已重启' })).toBeVisible()
    expect(await callsTo(request, 'POST', '/api/executors/Alpha/restart')).toHaveLength(1)
  })

  test('单条停止：确认框文案明确，取消不发起调用', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Alpha')
    await r.hover()
    await r.getByRole('button', { name: '停止 Alpha' }).click()

    const dialog = page.getByRole('dialog', { name: '停止执行器 Alpha' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('将停止 Alpha 的子进程（整棵进程树优雅终止）')
    await expect(dialog.getByRole('button', { name: '停止 Alpha' })).toBeVisible()

    await dialog.getByRole('button', { name: '取消' }).click()
    await expect(dialog).toBeHidden()
    expect(await callsTo(request, 'POST', '/api/executors/Alpha/stop')).toHaveLength(0)
  })

  test('单条停止：确认后调用 stop 并提示成功', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Alpha')
    await r.hover()
    await r.getByRole('button', { name: '停止 Alpha' }).click()
    const dialog = page.getByRole('dialog', { name: '停止执行器 Alpha' })
    await dialog.getByRole('button', { name: '停止 Alpha' }).click()

    await expect(page.locator('.awem-toast', { hasText: 'Alpha 已停止' })).toBeVisible()
    expect(await callsTo(request, 'POST', '/api/executors/Alpha/stop')).toHaveLength(1)
    await expect(row(page, 'Alpha').getByText('已停止')).toBeVisible()
  })

  test('单条删除（已停止）：确认后无 stop/purge 参数，提示日志保留', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Beta')
    await r.hover()
    await r.getByRole('button', { name: '删除 Beta' }).click()

    const dialog = page.getByRole('dialog', { name: '删除执行器 Beta' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('本地日志保留')
    await expect(dialog).toContainText('此操作不可恢复')
    await dialog.getByRole('button', { name: '删除 Beta' }).click()

    await expect(page.locator('.awem-toast', { hasText: '已删除 Beta' })).toBeVisible()
    const calls = await callsTo(request, 'DELETE', '/api/executors/Beta')
    expect(calls).toHaveLength(1)
    expect(calls[0].query).toEqual({})
    await expect(row(page, 'Beta')).toHaveCount(0)
  })

  test('单条删除（运行中）：确认框提示先停进程，删除带 stop=true&purge=false', async ({ page, request }) => {
    await gotoOverview(page)
    const r = row(page, 'Alpha')
    await r.hover()
    await r.getByRole('button', { name: '删除 Alpha' }).click()

    const dialog = page.getByRole('dialog', { name: '删除执行器 Alpha' })
    await expect(dialog).toContainText('正在运行，将先停止进程')
    await dialog.getByRole('button', { name: '删除 Alpha' }).click()

    await expect(page.locator('.awem-toast', { hasText: '已删除 Alpha（已停止进程）' })).toBeVisible()
    const calls = await callsTo(request, 'DELETE', '/api/executors/Alpha')
    expect(calls).toHaveLength(1)
    expect(calls[0].query).toEqual({ stop: 'true', purge: 'false' })
    await expect(row(page, 'Alpha')).toHaveCount(0)
  })

  test('单条操作执行期间行内按钮禁用', async ({ page, request }) => {
    await setMock(request, { actionDelayMs: 800 })
    await gotoOverview(page)
    const r = row(page, 'Beta')
    await r.hover()
    await r.getByRole('button', { name: '启动 Beta' }).click()
    await expect(r.getByRole('button', { name: '启动 Beta' })).toBeDisabled()
    await expect(r.getByRole('button', { name: '停止 Beta' })).toBeDisabled()
    await expect(r.getByRole('button', { name: '重启 Beta' })).toBeDisabled()
    await expect(r.getByRole('button', { name: '删除 Beta' })).toBeDisabled()
    await expect(r.getByRole('button', { name: '启动 Beta' })).toBeEnabled({ timeout: 5_000 })
  })

  test('5s 轮询刷新列表', async ({ page, request }) => {
    await gotoOverview(page)
    await expect(row(page, 'Alpha')).toHaveCount(1)
    const all = await mockRequests(request)
    const firstListAt = all.filter((r) => r.path === '/api/executors').length
    expect(firstListAt).toBeGreaterThan(0)

    await setMock(request, {
      executors: [
        { name: 'Alpha', state: 'running', autostart: true },
        { name: 'Beta', state: 'stopped' },
        { name: 'Gamma', state: 'failed', restarts: 3 },
        { name: 'Delta', state: 'backoff' },
        { name: 'Epsilon', state: 'stopped' },
      ],
    })
    await expect(page.getByRole('link', { name: 'Epsilon' })).toBeVisible({ timeout: 8_000 })
  })

  test('空状态：无执行器时给出添加入口', async ({ page, request }) => {
    await setMock(request, { executors: [] })
    await gotoOverview(page)
    await expect(page.getByText('还没有执行器')).toBeVisible()
    await expect(page.locator('.awem-empty').getByRole('button', { name: '添加执行器' })).toBeVisible()
    await expect(page.locator('.awem-table')).toHaveCount(0)
  })

  test('列表刷新失败：加载成功后轮询出错显示错误并可重试', async ({ page, request }) => {
    // 旧行为：首次加载失败停留在骨架屏，错误块只在已有数据后轮询失败时出现。
    await gotoOverview(page)
    await expect(page.getByRole('link', { name: 'Alpha' })).toBeVisible()

    await setMock(request, { listError: { status: 500, code: 'internal', message: '模拟列表故障' } })
    await expect(page.getByText('模拟列表故障')).toBeVisible({ timeout: 8_000 })
    await expect(page.locator('.awem-table')).toHaveCount(0)

    await setMock(request, { listError: null })
    await page.getByRole('button', { name: '重试' }).click()
    await expect(page.getByRole('link', { name: 'Alpha' })).toBeVisible()
  })
})
