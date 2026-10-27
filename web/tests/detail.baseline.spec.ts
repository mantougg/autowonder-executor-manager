// detail.baseline.spec.ts：@baseline 锁定详情页「修改前」的既有结构（状态摘要条 /
// 日志查看器 / 编辑表单）。功能实现前后都必须通过；不涉及本期新增的运行时间带。
import { expect } from '@playwright/test'
import { test } from './fixtures'
import { gotoDetail, resetMock, setMock } from './helpers'

test.describe('@baseline 详情页旧行为', () => {
  test.beforeEach(async ({ request }) => {
    await resetMock(request)
  })
  test.afterEach(async ({ request }) => {
    const res = await request.get('/__test__/unhandled')
    const data = (await res.json()) as { unhandled: unknown[] }
    expect(data.unhandled, '存在未配置的业务请求（Mock 不应回落真实后端）').toEqual([])
  })

  test('状态摘要条/日志查看器/编辑表单按旧结构渲染', async ({ page }) => {
    await gotoDetail(page, 'Alpha')

    await expect(page.getByText('本地进程：运行中')).toBeVisible()
    await expect(page.getByText('平台：—')).toBeVisible()
    await expect(page.getByText('pid 4321')).toBeVisible()
    await expect(page.getByText('uptime 1分0秒')).toBeVisible()
    await expect(page.getByText('重启 0 次')).toBeVisible()

    await expect(page.getByRole('heading', { name: '日志' })).toBeVisible()
    await expect(page.getByRole('switch', { name: '跟随滚动' })).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByText('（暂无日志；启动执行器后产生）')).toBeVisible()

    await expect(page.getByRole('heading', { name: '编辑' })).toBeVisible()
    await expect(
      page.locator('.awem-field', { hasText: '模型（model）' }).locator('input'),
    ).toHaveValue('qfmodel')
    await expect(page.getByRole('button', { name: '保存修改' })).toBeVisible()
    await expect(page.getByText(/executor-id Alpha/)).toBeVisible()
  })

  test('平台归属与漂移按旧结构展示', async ({ page, request }) => {
    await setMock(request, {
      executors: [
        {
          name: 'Alpha',
          state: 'running',
          autostart: true,
          platform: {
            status: 'ONLINE',
            name: '甲执行器',
            agent_name: '数字工人甲',
            project: '项目甲',
            squad: '小队甲',
          },
          drift: {
            fields: [{ field: 'model', local: 'qfmodel', remote: 'qmodel' }],
            message: '配置与平台不一致，需 restart 或平台配置已变（v1 不自动回写）',
          },
        },
      ],
    })
    await gotoDetail(page, 'Alpha')

    // 标题行：中文名为主标题（左），等宽标识名 hint（右）
    await expect(page.getByRole('heading', { name: '甲执行器（数字工人甲）' })).toBeVisible()
    await expect(page.locator('.awem-page-header').getByText('Alpha', { exact: true })).toBeVisible()
    await expect(page.getByText('平台：ONLINE')).toBeVisible()
    await expect(page.getByText('所属项目 项目甲')).toBeVisible()
    await expect(page.getByText('所属小队 小队甲')).toBeVisible()
    await expect(page.getByText('漂移 1')).toBeVisible()
    await expect(page.getByText('--model：本地 qfmodel vs 平台 qmodel')).toBeVisible()
  })
})
