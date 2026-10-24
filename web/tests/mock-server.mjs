// mock-server.mjs：Playwright 专用 Mock UI 后端（无第三方依赖）。
// - 静态服务 web/dist（本次构建产物），SPA 回退 index.html；
// - 实现总览页使用到的单执行器 API（status/executors/start/stop/restart/delete），
//   行为与 internal/api 契约一致（运行中删除须 stop=true，否则 409）；
// - 未配置的业务请求一律 404 并记入 unhandled，测试结束断言为空，绝不回落真实后端；
// - /__test__/* 为测试控制面（重置、注入状态/失败/延迟、读取请求日志）。
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { dirname, extname, join, normalize, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = dirname(fileURLToPath(import.meta.url))
const DIST = resolve(HERE, '..', 'dist')
const PORT = Number(process.env.MOCK_PORT || 43117)
const RUNNING_LIKE = new Set(['running', 'backoff', 'stopping'])

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.ico': 'image/x-icon',
  '.woff2': 'font/woff2',
  '.map': 'application/json; charset=utf-8',
}

function makeExecutor(spec) {
  const state = spec.state || 'stopped'
  return {
    name: spec.name,
    command: `npx -y autowonder@0.3.22 connect --token test-token --executor-id ${spec.name}`,
    autostart: Boolean(spec.autostart),
    restart: { enabled: true, backoff: 'exponential' },
    parsed: {
      runtime_version: '0.3.22',
      ws_url: 'wss://platform.invalid/ws',
      token: 'test-token',
      executor_id: spec.name,
      provider: 'qoder',
      model: spec.model || 'qfmodel',
      reasoning_effort: 'high',
      context_window: '260000',
      memory_mode: 'platform',
      max_tasks: '5',
      token_aware: false,
      debug: false,
      passthrough: null,
    },
    local: {
      state,
      pid: RUNNING_LIKE.has(state) ? 4321 : 0,
      started_at: '2026-10-25T00:00:00Z',
      uptime_ms: RUNNING_LIKE.has(state) ? 60_000 : 0,
      restarts: spec.restarts ?? 0,
      backoff_delay_ms: state === 'backoff' ? 2_000 : 0,
      conn: state === 'running' ? 'connected' : 'disconnected',
      last_exit: state === 'failed' ? 'exit status 1' : undefined,
    },
    platform: spec.platform || null,
    drift: spec.drift || null,
  }
}

function defaultState() {
  return {
    executors: [
      makeExecutor({ name: 'Alpha', state: 'running', autostart: true }),
      makeExecutor({ name: 'Beta', state: 'stopped' }),
      makeExecutor({ name: 'Gamma', state: 'failed', restarts: 3 }),
      makeExecutor({ name: 'Delta', state: 'backoff' }),
    ],
    failures: {},
    actionDelayMs: 0,
    listDelayMs: 0,
    listError: null,
    timelines: {},
    logs: {},
  }
}

let state = defaultState()
let requests = []
let unhandled = []

function record(req, url) {
  requests.push({
    method: req.method,
    path: url.pathname,
    query: Object.fromEntries(url.searchParams.entries()),
    at: Date.now(),
  })
}

function findExecutor(name) {
  return state.executors.find((e) => e.name === name)
}

function failureFor(action, name) {
  return state.failures[`${action}:${name}`] || state.failures[`${action}:*`] || null
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function sendJSON(res, status, body) {
  const data = JSON.stringify(body)
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(data),
    'Cache-Control': 'no-store',
  })
  res.end(data)
}

function sendError(res, status, code, message) {
  sendJSON(res, status, { error: { code, message } })
}

async function readBody(req) {
  const chunks = []
  let size = 0
  for await (const c of req) {
    size += c.length
    if (size > 1024 * 1024) throw new Error('body too large')
    chunks.push(c)
  }
  if (!chunks.length) return null
  return JSON.parse(Buffer.concat(chunks).toString('utf8'))
}

async function serveStatic(res, pathname) {
  const rel = pathname === '/' ? '/index.html' : pathname
  const file = normalize(join(DIST, decodeURIComponent(rel)))
  if (file !== DIST && !file.startsWith(DIST + sep)) {
    sendError(res, 400, 'bad_path', '非法路径')
    return
  }
  let data
  try {
    data = await readFile(file)
  } catch {
    try {
      data = await readFile(join(DIST, 'index.html'))
    } catch {
      sendError(res, 500, 'no_build', 'web/dist 不存在，请先 npm run build')
      return
    }
    res.writeHead(200, { 'Content-Type': MIME['.html'], 'Cache-Control': 'no-store' })
    res.end(data)
    return
  }
  res.writeHead(200, {
    'Content-Type': MIME[extname(file)] || 'application/octet-stream',
    'Cache-Control': 'no-store',
  })
  res.end(data)
}

async function handleControl(req, res, url) {
  const p = url.pathname
  if (p === '/__test__/health') return sendJSON(res, 200, { ok: true })
  if (p === '/__test__/reset' && req.method === 'POST') {
    state = defaultState()
    requests = []
    unhandled = []
    return sendJSON(res, 200, { ok: true })
  }
  if (p === '/__test__/state' && req.method === 'POST') {
    const body = (await readBody(req)) || {}
    if (body.executors) {
      state.executors = body.executors.map((spec) => makeExecutor(spec))
    }
    if (body.failures) state.failures = body.failures
    if (typeof body.actionDelayMs === 'number') state.actionDelayMs = body.actionDelayMs
    if (typeof body.listDelayMs === 'number') state.listDelayMs = body.listDelayMs
    if ('listError' in body) state.listError = body.listError
    if (body.timelines) state.timelines = body.timelines
    if (body.logs) state.logs = body.logs
    return sendJSON(res, 200, { ok: true })
  }
  if (p === '/__test__/requests' && req.method === 'GET') {
    return sendJSON(res, 200, { requests })
  }
  if (p === '/__test__/clear-requests' && req.method === 'POST') {
    requests = []
    return sendJSON(res, 200, { ok: true })
  }
  if (p === '/__test__/unhandled' && req.method === 'GET') {
    return sendJSON(res, 200, { unhandled })
  }
  return sendError(res, 404, 'unknown_control', '未知测试控制端点')
}

async function handleAPI(req, res, url) {
  const p = url.pathname

  if (req.method === 'GET' && p === '/api/status') {
    return sendJSON(res, 200, {
      version: 'test',
      pid: 1,
      port: PORT,
      started_at: '2026-10-25T00:00:00Z',
      uptime_ms: 60_000,
      platform: { configured: false, connected: null },
    })
  }

  if (req.method === 'GET' && p === '/api/executors') {
    record(req, url)
    if (state.listDelayMs > 0) await sleep(state.listDelayMs)
    if (state.listError) {
      return sendError(res, state.listError.status, state.listError.code || 'error', state.listError.message)
    }
    return sendJSON(res, 200, { executors: state.executors })
  }

  const singleGet = p.match(/^\/api\/executors\/([^/]+)$/)
  if (req.method === 'GET' && singleGet) {
    record(req, url)
    const name = decodeURIComponent(singleGet[1])
    const e = findExecutor(name)
    if (!e) return sendError(res, 404, 'not_found', `执行器 ${name} 不存在`)
    return sendJSON(res, 200, { executor: e })
  }

  const logReq = p.match(/^\/api\/executors\/([^/]+)\/log$/)
  if (req.method === 'GET' && logReq) {
    record(req, url)
    const name = decodeURIComponent(logReq[1])
    if (!findExecutor(name)) return sendError(res, 404, 'not_found', `执行器 ${name} 不存在`)
    const cfg = state.logs[name] || {}
    const file = url.searchParams.get('file')
    const before = url.searchParams.get('before')
    if (file || before) {
      const limit = Number(url.searchParams.get('limit') || 500)
      const lines = (cfg.jump_lines || cfg.lines || []).slice(-limit)
      return sendJSON(res, 200, {
        name, file, lines,
        start_file: file,
        start_offset: Number(before || 0),
        has_more: false,
      })
    }
    const tail = Number(url.searchParams.get('tail') || 50)
    return sendJSON(res, 200, { name, tail, lines: (cfg.lines || []).slice(-tail) })
  }

  const timelineReq = p.match(/^\/api\/stats\/executors\/([^/]+)\/timeline$/)
  if (req.method === 'GET' && timelineReq) {
    record(req, url)
    const name = decodeURIComponent(timelineReq[1])
    if (!findExecutor(name)) return sendError(res, 404, 'not_found', `执行器 ${name} 不存在`)
    const injected = state.timelines[name] || {
      data_start: null, data_end: null, segments: [], crashes: [],
    }
    return sendJSON(res, 200, {
      name,
      from: url.searchParams.get('from'),
      to: url.searchParams.get('to'),
      ...injected,
    })
  }

  const action = p.match(/^\/api\/executors\/([^/]+)\/(start|stop|restart)$/)
  if (req.method === 'POST' && action) {
    const name = decodeURIComponent(action[1])
    const kind = action[2]
    record(req, url)
    const e = findExecutor(name)
    if (!e) return sendError(res, 404, 'not_found', `执行器 ${name} 不存在`)
    const failure = failureFor(kind, name)
    if (state.actionDelayMs > 0) await sleep(state.actionDelayMs)
    if (failure) return sendError(res, failure.status, failure.code || 'error', failure.message)
    if (kind === 'start' || kind === 'restart') {
      e.local.state = 'running'
      e.local.pid = 4321
      e.local.conn = 'connected'
    } else {
      e.local.state = 'stopped'
      e.local.pid = 0
      e.local.conn = 'disconnected'
    }
    return sendJSON(res, 200, { executor: e })
  }

  const single = p.match(/^\/api\/executors\/([^/]+)$/)
  if (req.method === 'DELETE' && single) {
    const name = decodeURIComponent(single[1])
    record(req, url)
    const e = findExecutor(name)
    if (!e) return sendError(res, 404, 'not_found', `执行器 ${name} 不存在`)
    const failure = failureFor('delete', name)
    if (state.actionDelayMs > 0) await sleep(state.actionDelayMs)
    if (failure) return sendError(res, failure.status, failure.code || 'error', failure.message)
    const stop = url.searchParams.get('stop') === 'true'
    const purge = url.searchParams.get('purge') === 'true'
    const running = RUNNING_LIKE.has(e.local.state)
    if (running && !stop) {
      return sendError(res, 409, 'running', `执行器 ${name} 运行中，删除须显式 stop=true（先停止再删除）`)
    }
    state.executors = state.executors.filter((x) => x.name !== name)
    return sendJSON(res, 200, { removed: name, stopped: running, purged: purge })
  }

  unhandled.push({ method: req.method, path: p })
  return sendError(res, 404, 'unhandled', `未配置的业务请求：${req.method} ${p}`)
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url || '/', `http://127.0.0.1:${PORT}`)
  try {
    if (url.pathname.startsWith('/__test__/')) return await handleControl(req, res, url)
    if (url.pathname.startsWith('/api/')) return await handleAPI(req, res, url)
    if (req.method === 'GET' || req.method === 'HEAD') return await serveStatic(res, url.pathname)
    return sendError(res, 405, 'method_not_allowed', '方法不支持')
  } catch (err) {
    sendError(res, 500, 'mock_error', String(err && err.message ? err.message : err))
  }
})

server.listen(PORT, '127.0.0.1', () => {
  console.log(`mock-server listening on http://127.0.0.1:${PORT} (dist=${DIST})`)
})
