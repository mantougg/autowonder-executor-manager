// LogViewer.tsx：日志查看器（§5：无论主题恒暗底 #0d1117、等宽 13px、
// ANSI 颜色渲染、WS 实时尾部流、跟随开关默认开、上滚暂停、回底恢复、
// 历史向上翻页加载）。
import { useEffect, useRef, useState, type JSX } from 'react'

// ansiToNodes：解析常见 SGR 色码（30-37/90-97 前景、1 加粗、0 重置）为
// <span>。覆盖 connect 日志的 ANSI 用色（如 \x1b[36m 青）。
const ANSI_COLORS: Record<number, string> = {
  30: '#6a737d', 31: '#f85149', 32: '#3fb950', 33: '#d29922',
  34: '#58a6ff', 35: '#bc8cff', 36: '#39c5cf', 37: '#e6edf3',
  90: '#8b949e', 91: '#ffa198', 92: '#56d364', 93: '#e3b341',
  94: '#79c0ff', 95: '#d2a8ff', 96: '#56d4dd', 97: '#f0f6fc',
}

function ansiToNodes(line: string): JSX.Element {
  interface Part { text: string; color?: string; bold?: boolean }
  const parts: (string | Part)[] = []
  let cur: Part = { text: '' }
  let i = 0
  while (i < line.length) {
    const idx = line.indexOf('\x1b[', i)
    if (idx < 0) {
      cur.text += line.slice(i)
      break
    }
    cur.text += line.slice(i, idx)
    const m = line.slice(idx).match(/^\x1b\[([0-9;]*)m/)
    if (!m) {
      cur.text += line[idx]
      i = idx + 1
      continue
    }
    if (cur.text) parts.push(cur)
    const codes = m[1] === '' ? ['0'] : m[1].split(';')
    cur = { text: '' }
    for (const c of codes) {
      const n = Number(c)
      if (n === 0) cur = { text: '' }
      else if (n === 1) cur.bold = true
      else if (ANSI_COLORS[n]) cur.color = ANSI_COLORS[n]
    }
    i = idx + m[0].length
  }
  if (cur.text) parts.push(cur)
  return (
    <>
      {parts.map((p, k) =>
        typeof p === 'string' ? (
          <span key={k}>{p}</span>
        ) : (
          <span key={k} style={{ color: p.color, fontWeight: p.bold ? 600 : undefined }}>
            {p.text}
          </span>
        ),
      )}
    </>
  )
}

export function LogViewer({ name, token, tail = 200 }: { name: string; token: string; tail?: number }) {
  const [lines, setLines] = useState<string[]>([])
  const [follow, setFollow] = useState(true)
  const [connected, setConnected] = useState(false)
  const [pausedByScroll, setPausedByScroll] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)
  const followRef = useRef(true)
  const totalRef = useRef(0)
  const [older, setOlder] = useState<number | null>(null) // 更早日志是否可加载
  const [loadingOlder, setLoadingOlder] = useState(false)

  useEffect(() => {
    followRef.current = follow && !pausedByScroll
  }, [follow, pausedByScroll])

  // WS 实时流
  useEffect(() => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const url = `${proto}://${location.host}/ws/logs/${encodeURIComponent(name)}?tail=${tail}&token=${encodeURIComponent(token)}`
    let ws: WebSocket | null = null
    let retry: number | undefined
    let closed = false
    const connect = () => {
      ws = new WebSocket(url)
      ws.onopen = () => setConnected(true)
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data as string)
          if (msg.type === 'backlog') {
            totalRef.current += msg.lines.length
            setLines((cur) => [...cur, ...msg.lines])
          } else if (msg.type === 'line') {
            totalRef.current += 1
            setLines((cur) => [...cur, msg.text])
          } else if (msg.type === 'error') {
            setLines((cur) => [...cur, `[日志流错误] ${msg.message}`])
          }
        } catch {
          /* 忽略非 JSON 帧 */
        }
      }
      ws.onclose = () => {
        setConnected(false)
        if (!closed) retry = window.setTimeout(connect, 3000) // 自动重连（选定记录：3s 退避重试）
      }
    }
    connect()
    return () => {
      closed = true
      if (retry) clearTimeout(retry)
      ws?.close()
    }
  }, [name, token, tail])

  // 跟随滚动：新行追加且处于跟随态 → 滚到底
  useEffect(() => {
    if (followRef.current && boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight
    }
  }, [lines])

  const onScroll = () => {
    const el = boxRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    setPausedByScroll(!atBottom)
  }

  // 历史日志向上翻页：加载更早日志（走滚动文件链——v1 从滚动链头部补）
  const loadOlder = async () => {
    setLoadingOlder(true)
    try {
      const resp = await fetch(
        `/api/executors/${encodeURIComponent(name)}/log?tail=500`,
        { headers: { Authorization: `Bearer ${token}` } },
      )
      if (resp.ok) {
        const d = await resp.json() as { lines: string[] }
        const want = Math.max(0, totalRef.current - d.lines.length) // 已展示的头部位置
        const pre = d.lines.slice(0, want)
        if (pre.length === 0) {
          setOlder(null) // 没有更早的了
        } else {
          totalRef.current += pre.length
          setLines((cur) => [...pre, ...cur])
          setOlder(pre.length >= 500 ? 1 : null)
        }
      }
    } finally {
      setLoadingOlder(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 13, color: 'var(--awem-fg-secondary)' }}>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <input
            type="checkbox"
            checked={follow && !pausedByScroll}
            disabled={!follow}
            readOnly
            aria-label="跟随滚动"
            style={{ display: 'none' }}
          />
          <button
            className="awem-switch"
            role="switch"
            aria-checked={follow && !pausedByScroll}
            aria-label="跟随滚动"
            onClick={() => {
              setFollow((f) => !f)
              setPausedByScroll(false)
            }}
          />
          跟随滚动
        </label>
        {pausedByScroll && follow ? (
          <button className="awem-btn awem-btn-ghost" onClick={() => setPausedByScroll(false)}>
            回到底部恢复跟随
          </button>
        ) : null}
        <span>{connected ? '● 实时流已连接' : '○ 实时流断开（自动重连中）'}</span>
      </div>
      <div
        ref={boxRef}
        onScroll={onScroll}
        className="mono"
        style={{
          background: '#0d1117',
          color: '#e6edf3',
          fontSize: 13,
          lineHeight: 1.45,
          borderRadius: 8,
          border: '1px solid var(--awem-border)',
          padding: 12,
          height: 420,
          overflowY: 'auto',
        }}
        role="log"
        aria-label={`${name} 日志`}
      >
        {lines.length === 0 ? (
          <div style={{ color: '#8b949e' }}>（暂无日志；启动执行器后产生）</div>
        ) : (
          lines.map((l, i) => (
            <div key={i} style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
              {ansiToNodes(l)}
            </div>
          ))
        )}
      </div>
      {older !== null ? (
        <button className="awem-btn awem-btn-secondary awem-btn-compact" onClick={loadOlder} disabled={loadingOlder}>
          {loadingOlder ? '加载中…' : '加载更早日志'}
        </button>
      ) : null}
    </div>
  )
}
