// api.ts：统一 API 客户端（token 头注入 + 401 处理；schema 与 T06/F3 契约一致）。
import { t } from './i18n'

const TOKEN_KEY = 'awem-token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}
export function setToken(tok: string) {
  localStorage.setItem(TOKEN_KEY, tok)
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

export class APIError extends Error {
  code: string
  field: string
  httpStatus: number
  constructor(httpStatus: number, code: string, message: string, field = '') {
    super(message)
    this.httpStatus = httpStatus
    this.code = code
    this.field = field
  }
}

/** onUnauthorized：401 时回调（App 清 token 回登录卡片）。 */
export async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
  onUnauthorized?: () => void,
): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${getToken()}`)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  let resp: Response
  try {
    resp = await fetch(path, { ...init, headers })
  } catch {
    throw new APIError(0, 'network', t.common.networkError)
  }
  if (resp.status === 401) {
    onUnauthorized?.()
    throw new APIError(401, 'unauthorized', t.login.wrong)
  }
  const data = await resp.json().catch(() => null)
  if (!resp.ok) {
    const err = (data as { error?: { code?: string; message?: string; field?: string } })?.error
    throw new APIError(resp.status, err?.code ?? 'unknown', err?.message ?? `HTTP ${resp.status}`, err?.field ?? '')
  }
  return data as T
}

// --- 类型（与 API 视图契约一致；T07-F10 汇总表） ---

export interface DriftField {
  field: string
  local: string
  remote: string
}

export interface ExecutorLocal {
  state: string
  pid: number
  started_at: string
  uptime_ms: number
  restarts: number
  backoff_delay_ms: number
  conn: string
  last_exit?: string
}

export interface ExecutorParsed {
  runtime_version: string
  ws_url: string
  token: string
  executor_id: string
  provider: string
  model: string
  reasoning_effort: string
  context_window: string
  memory_mode: string
  max_tasks: string
  token_aware: boolean
  debug: boolean
  passthrough: string[] | null // v0.1.0 服务端曾对无未知 flag 输出 null，消费方须判空
}

export interface PlatformInfo {
  status: string
  last_heartbeat?: string
  name?: string // 平台侧执行器名（接入时的中文名）
  agent_name?: string // 所属数字工人中文名
  project?: string // 所属项目（工作空间）名
  squad?: string // 所属小队名（顿号拼接，列表列兼容）
  squad_names?: string[] // 所属小队数组（树形分组用；旧服务端可能缺失）
}

export interface DriftInfo {
  fields: DriftField[]
  message: string
}

export interface ExecutorView {
  name: string
  command: string
  autostart: boolean
  workspace_root?: string
  restart: { enabled: boolean; backoff: string }
  parsed: ExecutorParsed | null
  local: ExecutorLocal
  platform: PlatformInfo | null
  drift: DriftInfo | null
}

export interface StatusView {
  version: string
  pid: number
  port: number
  started_at: string
  uptime_ms: number
  platform: { configured: boolean; connected: boolean | null }
}

export interface SettingsView {
  platform: { base_url: string; mcp_token: string; poll_interval: string } | null
  log: { max_size_mb: number; retention_days: number }
}

export const listExecutors = (on401?: () => void) =>
  apiFetch<{ executors: ExecutorView[] }>('/api/executors', {}, on401)
export const getExecutor = (name: string, on401?: () => void) =>
  apiFetch<{ executor: ExecutorView }>(`/api/executors/${encodeURIComponent(name)}`, {}, on401)
export const getStatus = (on401?: () => void) => apiFetch<StatusView>('/api/status', {}, on401)
export const getSettings = (on401?: () => void) => apiFetch<SettingsView>('/api/settings', {}, on401)
export const putSettings = (body: unknown, on401?: () => void) =>
  apiFetch<SettingsView>('/api/settings', { method: 'PUT', body: JSON.stringify(body) }, on401)
export const platformHealth = (on401?: () => void) =>
  apiFetch<{ configured: boolean; ok: boolean | null; detail: string }>('/api/platform/health', {}, on401)

// --- 运行时间带（一期） ---

export interface TimelineSegment {
  from: string
  to: string
  state: string // running / backoff / stopped / nodata
}

export interface TimelineCrash {
  at: string
  log_file?: string
  log_offset?: number
  auto_restart: boolean
  locatable: boolean
}

export interface TimelineView {
  name: string
  from: string
  to: string
  data_start: string | null
  data_end: string | null
  segments: TimelineSegment[]
  crashes: TimelineCrash[]
}

export const getTimeline = (name: string, from: string, to: string, on401?: () => void) => {
  const q = new URLSearchParams({ from, to })
  return apiFetch<TimelineView>(
    `/api/stats/executors/${encodeURIComponent(name)}/timeline?${q.toString()}`,
    {},
    on401,
  )
}

export interface LogPage {
  lines: string[]
  tail?: number
  start_file?: string
  start_offset?: number
  has_more?: boolean
}

/** 日志读取：默认尾部 tail；带 file/before/limit 时按定位信息读取历史页。 */
export const getLogPage = (
  name: string,
  params: { tail?: number; file?: string; before?: number; limit?: number },
  on401?: () => void,
) => {
  const q = new URLSearchParams()
  if (params.tail !== undefined) q.set('tail', String(params.tail))
  if (params.file) q.set('file', params.file)
  if (params.before !== undefined) q.set('before', String(params.before))
  if (params.limit !== undefined) q.set('limit', String(params.limit))
  return apiFetch<LogPage>(`/api/executors/${encodeURIComponent(name)}/log?${q.toString()}`, {}, on401)
}
