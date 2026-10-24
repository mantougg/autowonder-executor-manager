// helpers.ts：Mock 控制面与总览页测试辅助（测试源码，与运行产物分离）。
import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test'

export type LocalState = 'running' | 'stopped' | 'backoff' | 'stopping' | 'failed'

export interface ExecutorSpec {
  name: string
  state?: LocalState
  autostart?: boolean
  restarts?: number
  model?: string
}

export interface MockRequest {
  method: string
  path: string
  query: Record<string, string>
  at: number
}

/** 重置 Mock（默认 4 台执行器）并按需覆盖状态。 */
export async function resetMock(request: APIRequestContext, body?: Record<string, unknown>) {
  const res = await request.post('/__test__/reset')
  expect(res.ok(), 'Mock 重置失败').toBeTruthy()
  if (body) await setMock(request, body)
}

/** 覆盖 Mock 状态（executors / failures / actionDelayMs / listError）。 */
export async function setMock(request: APIRequestContext, body: Record<string, unknown>) {
  const res = await request.post('/__test__/state', { data: body })
  expect(res.ok(), 'Mock 状态注入失败').toBeTruthy()
}

/** 读取 Mock 收到的全部请求（保序）。 */
export async function mockRequests(request: APIRequestContext): Promise<MockRequest[]> {
  const res = await request.get('/__test__/requests')
  expect(res.ok(), '读取 Mock 请求日志失败').toBeTruthy()
  return ((await res.json()) as { requests: MockRequest[] }).requests
}

/** 过滤指定动作的请求。 */
export async function callsTo(
  request: APIRequestContext,
  method: string,
  path: string,
): Promise<MockRequest[]> {
  return (await mockRequests(request)).filter((r) => r.method === method && r.path === path)
}

/** 未配置的业务请求必须为空（Mock 不回落真实后端）。 */
export async function expectNoUnhandled(request: APIRequestContext) {
  const res = await request.get('/__test__/unhandled')
  expect(res.ok(), '读取未配置请求失败').toBeTruthy()
  const data = (await res.json()) as { unhandled: unknown[] }
  expect(data.unhandled, '存在未配置的业务请求（未阻断即回落风险）').toEqual([])
}

/** 总览表格行（按名称文本定位）。 */
export function row(page: Page, name: string): Locator {
  return page.locator('.awem-table tbody tr').filter({ hasText: name })
}

/** 行选择复选框。 */
export function rowCheckbox(page: Page, name: string): Locator {
  return page.getByRole('checkbox', { name: `选择执行器 ${name}` })
}

/** 表头全选复选框。 */
export function selectAllCheckbox(page: Page): Locator {
  return page.getByRole('checkbox', { name: '全选执行器' })
}

/** 批量操作条。 */
export function bulkBar(page: Page): Locator {
  return page.locator('.awem-bulk-bar')
}

/** 打开总览页并等待标题。 */
export async function gotoOverview(page: Page) {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '总览' })).toBeVisible()
}

/** 在页面加载前写入本机 token，跳过登录卡片。 */
export async function injectToken(page: Page) {
  await page.addInitScript(() => localStorage.setItem('awem-token', 'test-token'))
}

/** 默认 4 台的名称快照，便于增补。 */
export const DEFAULT_NAMES = ['Alpha', 'Beta', 'Gamma', 'Delta'] as const
