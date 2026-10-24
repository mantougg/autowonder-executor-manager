// Detail.tsx：执行器详情页（F4）。状态摘要条（双状态/pid/uptime/重启）+
// 日志查看器（LogViewer 组件）+ 结构化编辑表单（字段同 CLI edit；token
// 掩码+眼睛；保存 PUT + Toast；运行中提示需 restart）。
import { useEffect, useRef, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { t } from '../i18n'
import {
  getExecutor, getLogPage, apiFetch, getToken, type ExecutorView, type TimelineCrash,
} from '../api'
import { StatusDot, localStateKind, localStateText, platformKind, platformText } from '../components/StatusDot'
import { LogViewer, type LogJump } from '../components/LogViewer'
import { Timeline } from '../components/Timeline'
import { fmtLocal } from '../time'
import { ToastProvider, useToast } from '../components/Toast'

function fmtUptime(ms: number): string {
  if (ms <= 0) return '—'
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60)
  if (h > 0) return `${h}时${m}分`
  if (m > 0) return `${m}分${s % 60}秒`
  return `${s}秒`
}

interface EditForm {
  model: string
  reasoning_effort: string
  context_window: string
  memory_mode: string
  max_tasks: string
  token: string
  autostart: boolean
  debug: boolean
}

function DetailInner({ name, onUnauthorized }: { name: string; onUnauthorized: () => void }) {
  const toast = useToast()
  const [view, setView] = useState<ExecutorView | null>(null)
  const [error, setError] = useState('')
  const [form, setForm] = useState<EditForm | null>(null)
  const [showToken, setShowToken] = useState(false)
  const [saving, setSaving] = useState(false)
  const [jump, setJump] = useState<LogJump | null>(null)
  const jumpSeq = useRef(0)

  const refresh = () => {
    getExecutor(name, onUnauthorized)
      .then((d) => {
        setView(d.executor)
        setForm((old) =>
          old ?? {
            model: d.executor.parsed?.model ?? '',
            reasoning_effort: d.executor.parsed?.reasoning_effort ?? '',
            context_window: d.executor.parsed?.context_window ?? '',
            memory_mode: d.executor.parsed?.memory_mode ?? '',
            max_tasks: d.executor.parsed?.max_tasks ?? '',
            token: d.executor.parsed?.token ?? '',
            autostart: d.executor.autostart,
            debug: d.executor.parsed?.debug ?? false,
          },
        )
      })
      .catch((e) => setError(e.message))
  }
  useEffect(refresh, [name]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const id = setInterval(refresh, 5000)
    return () => clearInterval(id)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    if (!form) return
    setSaving(true)
    try {
      const fields: Record<string, string | boolean> = {
        model: form.model, reasoning_effort: form.reasoning_effort,
        context_window: form.context_window, memory_mode: form.memory_mode,
        max_tasks: form.max_tasks, debug: form.debug,
      }
      // token 提交掩码原值 = 未修改（API 契约）
      if (form.token && form.token !== view?.parsed?.token) fields.token = form.token
      await apiFetch(`/api/executors/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: JSON.stringify({ fields, autostart: form.autostart }),
      }, onUnauthorized)
      toast('已保存' + (view && ['running', 'backoff', 'stopping'].includes(view.local.state) ? '（运行中，重启后生效）' : ''))
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  // 时间带崩溃点穿透：可定位 → 该时刻前最后一页历史日志；否则加载最近页并降级提示。
  const locateCrash = async (crash: TimelineCrash) => {
    try {
      if (crash.locatable && crash.log_file) {
        const page = await getLogPage(
          name,
          { file: crash.log_file, before: crash.log_offset ?? 0, limit: 500 },
          onUnauthorized,
        )
        jumpSeq.current += 1
        setJump({
          seq: jumpSeq.current,
          lines: page.lines,
          notice: t.timeline.located(fmtLocal(crash.at)),
        })
      } else {
        const page = await getLogPage(name, { tail: 500 }, onUnauthorized)
        jumpSeq.current += 1
        setJump({ seq: jumpSeq.current, lines: page.lines, notice: t.timeline.degrade })
      }
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  if (error) return <div className="awem-page"><div className="awem-error-text">{error}</div></div>
  if (!view || !form) {
    return (
      <div className="awem-page" aria-busy="true">
        <div className="awem-skeleton" style={{ height: 64 }} />
        <div className="awem-skeleton" style={{ height: 420 }} />
      </div>
    )
  }

  const p = view.parsed
  return (
    <div className="awem-page">
      <div className="awem-page-header">
        <h1 className="awem-page-title mono">{view.name}</h1>
        {view.platform?.name ? (
          <span className="awem-hint" style={{ fontSize: 15 }}>
            {view.platform.name}
            {view.platform.agent_name && view.platform.agent_name !== view.platform.name ? `（${view.platform.agent_name}）` : ''}
          </span>
        ) : null}
      </div>

      {/* 状态摘要条：双状态并列 + pid/uptime/重启 */}
      <div className="awem-card" style={{ display: 'flex', gap: 24, flexWrap: 'wrap', alignItems: 'center' }}>
        <StatusDot kind={localStateKind(view.local.state)} text={`${t.status.local}：${localStateText[view.local.state] ?? view.local.state}`} />
        {view.platform ? (
          <StatusDot kind={platformKind(view.platform.status)} text={`${t.status.platform}：${platformText[view.platform.status] ?? view.platform.status}`} />
        ) : (
          <span className="awem-hint">{t.status.platform}：—</span>
        )}
        {view.platform?.project ? <span className="awem-hint">所属项目 {view.platform.project}</span> : null}
        {view.platform?.squad ? <span className="awem-hint">所属小队 {view.platform.squad}</span> : null}
        <span className="awem-hint">pid {view.local.pid || '—'}</span>
        <span className="awem-hint">uptime {fmtUptime(view.local.uptime_ms)}</span>
        <span className="awem-hint">重启 {view.local.restarts} 次</span>
        {view.drift && view.drift.fields.length > 0 ? (
          <span className="awem-pill warn" title={view.drift.message}>{t.status.drift} {view.drift.fields.length}</span>
        ) : null}
      </div>

      {/* 运行时间带（24h，events.log 重放；崩溃点穿透日志） */}
      <Timeline name={view.name} onUnauthorized={onUnauthorized} onLocate={locateCrash} />

      {view.drift && view.drift.fields.length > 0 ? (
        <div className="awem-card" style={{ paddingTop: 12, paddingBottom: 12 }}>
          {view.drift.fields.map((f) => (
            <div key={f.field} className="mono" style={{ fontSize: 13 }}>
              --{f.field}：本地 {f.local} vs 平台 {f.remote}
            </div>
          ))}
          <div className="awem-hint">{view.drift.message}</div>
        </div>
      ) : null}

      {/* 日志查看器（暗底恒定） */}
      <div className="awem-card">
        <h2 className="awem-card-title">日志</h2>
        <LogViewer name={view.name} token={getToken()} jump={jump} />
      </div>

      {/* 结构化编辑表单 */}
      <div className="awem-card">
        <h2 className="awem-card-title">编辑</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', columnGap: 24 }}>
          <div className="awem-field">
            <label>模型（model）</label>
            <input className="awem-input mono" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} />
          </div>
          <div className="awem-field">
            <label>推理力度（reasoning-effort）</label>
            <input className="awem-input mono" value={form.reasoning_effort} onChange={(e) => setForm({ ...form, reasoning_effort: e.target.value })} />
          </div>
          <div className="awef-field awem-field">
            <label>上下文窗口（context-window）</label>
            <input className="awem-input mono" value={form.context_window} onChange={(e) => setForm({ ...form, context_window: e.target.value })} />
          </div>
          <div className="awem-field">
            <label>memory-mode</label>
            <input className="awem-input mono" value={form.memory_mode} onChange={(e) => setForm({ ...form, memory_mode: e.target.value })} />
          </div>
          <div className="awem-field">
            <label>max-tasks</label>
            <input className="awem-input mono" value={form.max_tasks} onChange={(e) => setForm({ ...form, max_tasks: e.target.value })} />
          </div>
          <div className="awem-field">
            <label>执行器 token（掩码显示；保持掩码原样=不修改）</label>
            <div className="awem-secret">
              <input
                className="awem-input mono"
                type={showToken ? 'text' : 'password'}
                value={form.token}
                onChange={(e) => setForm({ ...form, token: e.target.value })}
              />
              <button className="awem-eye" aria-label={showToken ? '隐藏 token' : '显示 token'} onClick={() => setShowToken((v) => !v)}>
                {showToken ? <EyeOff size={16} aria-hidden /> : <Eye size={16} aria-hidden />}
              </button>
            </div>
          </div>
          <div className="awem-field" style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <button className="awem-switch" role="switch" aria-checked={form.autostart} aria-label="autostart" onClick={() => setForm({ ...form, autostart: !form.autostart })} />
            <label>autostart（随 daemon 拉起）</label>
          </div>
          <div className="awem-field" style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <button className="awem-switch" role="switch" aria-checked={form.debug} aria-label="debug" onClick={() => setForm({ ...form, debug: !form.debug })} />
            <label>--debug</label>
          </div>
        </div>
        {p ? (
          <div className="awem-hint" style={{ marginBottom: 12 }}>
            只读：executor-id {p.executor_id} · provider {p.provider} · autowonder@{p.runtime_version}
            {p.passthrough && p.passthrough.length > 0 ? ` · 未知 flag 将保留（${p.passthrough.length} 个）` : ''}
          </div>
        ) : null}
        <button className="awem-btn awem-btn-primary" onClick={save} disabled={saving}>
          {saving ? '保存中…' : '保存修改'}
        </button>
      </div>
    </div>
  )
}

export function DetailPage(props: { name: string; onUnauthorized: () => void }) {
  return (
    <ToastProvider>
      <DetailInner {...props} />
    </ToastProvider>
  )
}
