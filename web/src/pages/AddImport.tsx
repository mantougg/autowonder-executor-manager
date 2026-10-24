// AddImport.tsx：添加 / 导入页（F5）。添加：粘贴整条命令 → POST 解析
// （服务端解析校验，错误逐字段）→ 解析预览（token/executor-id/model 高亮
// accent 底纹）→ 名称/开关 → 保存；导入：粘贴旧脚本 → 预览（改名建议）→
// 批量结果明细。
import { useState } from 'react'
import { t } from '../i18n'
import { apiFetch, type ExecutorView } from '../api'
import { ToastProvider, useToast } from '../components/Toast'
import { navigate } from '../router'

interface FieldIssue {
  flag: string
  problem: string
}

function AddInner({ onUnauthorized }: { onUnauthorized: () => void }) {
  const toast = useToast()
  const [command, setCommand] = useState('')
  const [name, setName] = useState('')
  const [autostart, setAutostart] = useState(false)
  const [start, setStart] = useState(false)
  const [preview, setPreview] = useState<ExecutorView | null>(null)
  const [fieldErr, setFieldErr] = useState<FieldIssue[] | null>(null)
  const [saving, setSaving] = useState(false)

  // 解析预览（POST 即真实验证；服务端返回视图或逐字段错误）
  const parse = async () => {
    setPreview(null)
    setFieldErr(null)
    setName('')
    try {
      const d = await apiFetch<{ executor: ExecutorView }>('/api/executors', {
        method: 'POST',
        body: JSON.stringify({ name: `预览-${Date.now()}`, command, start: false }),
      }, onUnauthorized).catch(async (e) => {
        // 已存在/字段错误等：还原现场——预览用的名称占用后立即删除
        throw e
      })
      // 预览即创建：立即删除（本地校验通过后的展示副本），保存时用最终名再建
      await apiFetch(`/api/executors/${encodeURIComponent(d.executor.name)}`, { method: 'DELETE' }, onUnauthorized)
      setPreview(d.executor)
      if (!name) setName(`exec-${d.executor.parsed?.executor_id ?? ''}`)
    } catch (e) {
      const err = e as { field?: string; message?: string }
      if (err.field) {
        const flags = err.field.split(',').filter(Boolean)
        setFieldErr(flags.map((f) => ({ flag: f, problem: err.message ?? '' })))
      } else {
        setFieldErr([{ flag: '', problem: err.message ?? String(e) }])
      }
    }
  }

  const save = async () => {
    setSaving(true)
    try {
      await apiFetch('/api/executors', {
        method: 'POST',
        body: JSON.stringify({ name, command, autostart, start }),
      }, onUnauthorized)
      toast(`已添加 ${name}`)
      navigate('/')
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  const p = preview?.parsed
  return (
    <div className="awem-page">
      <div className="awem-page-header"><h1 className="awem-page-title">添加执行器</h1></div>
      <div className="awem-card">
        <div className="awem-field">
          <label>粘贴整条启动命令（npx -y autowonder@版本 connect …）</label>
          <textarea
            className="awem-textarea"
            style={{ minHeight: 120 }}
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder="npx -y autowonder@0.3.22 connect --ws-url ws://… --token … --executor-id … --provider qoder --model …"
          />
        </div>
        <button className="awem-btn awem-btn-secondary awem-btn-compact" onClick={parse} disabled={!command.trim()}>
          解析预览
        </button>

        {fieldErr ? (
          <div style={{ marginTop: 12 }}>
            {fieldErr.map((f, i) => (
              <div key={i} className="awem-error-text">{f.flag ? `--${f.flag}：` : ''}{f.problem}</div>
            ))}
          </div>
        ) : null}

        {p && preview ? (
          <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', fontSize: 13 }}>
              <span>executor-id <span className="mono" style={{ background: 'var(--awem-bg-muted)', padding: '2px 6px', borderRadius: 6, color: 'var(--awem-accent)' }}>{p.executor_id}</span></span>
              <span>model <span className="mono" style={{ background: 'var(--awem-bg-muted)', padding: '2px 6px', borderRadius: 6, color: 'var(--awem-accent)' }}>{p.model || '—'}</span></span>
              <span>token <span className="mono" style={{ background: 'var(--awem-bg-muted)', padding: '2px 6px', borderRadius: 6, color: 'var(--awem-accent)' }}>{p.token}</span>（掩码）</span>
              <span className="awem-hint">provider {p.provider} · autowonder@{p.runtime_version}{p.passthrough && p.passthrough.length > 0 ? ` · 未知 flag 保留 ${p.passthrough.length} 个` : ''}</span>
            </div>
            <div className="awem-field" style={{ maxWidth: 320 }}>
              <label>名称（[a-zA-Z0-9._-]）</label>
              <input className="awem-input mono" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div style={{ display: 'flex', gap: 24 }}>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                <button className="awem-switch" role="switch" aria-checked={autostart} aria-label="autostart" onClick={() => setAutostart((v) => !v)} />
                autostart（随 daemon 拉起）
              </label>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                <button className="awem-switch" role="switch" aria-checked={start} aria-label="start" onClick={() => setStart((v) => !v)} />
                添加后立即启动
              </label>
            </div>
            <div>
              <button className="awem-btn awem-btn-primary" onClick={save} disabled={saving || !name.trim()}>
                {saving ? '保存中…' : t.actions.addExecutor}
              </button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}

interface ImportRow {
  name: string
  status: string
  detail?: string
}

function ImportInner({ onUnauthorized }: { onUnauthorized: () => void }) {
  const toast = useToast()
  const [script, setScript] = useState('')
  const [result, setResult] = useState<{ results: ImportRow[] } | null>(null)
  const [busy, setBusy] = useState(false)

  const run = async () => {
    setBusy(true)
    setResult(null)
    try {
      const d = await apiFetch<{ results: ImportRow[] }>('/api/executors/import', {
        method: 'POST',
        body: JSON.stringify({ script, withLogs: false, yes: true }),
      }, onUnauthorized)
      setResult(d)
      toast('导入完成')
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="awem-page">
      <div className="awem-page-header"><h1 className="awem-page-title">导入旧脚本</h1></div>
      <div className="awem-card">
        <div className="awem-field">
          <label>粘贴旧 executor-manager.sh 内容（EXECUTORS 数组）</label>
          <textarea className="awem-textarea" style={{ minHeight: 140 }} value={script} onChange={(e) => setScript(e.target.value)} />
        </div>
        <button className="awem-btn awem-btn-primary" onClick={run} disabled={busy || !script.trim()}>
          {busy ? '导入中…' : '解析并导入'}
        </button>
        {result ? (
          <div style={{ marginTop: 16 }}>
            {result.results.map((r, i) => (
              <div key={i} className="mono" style={{ fontSize: 13 }}>
                {r.status === 'failed' ? '✗' : '✓'} {r.name}
                {r.status === 'renamed' ? `（${r.detail}）` : ''}
                {r.status === 'failed' ? `：${r.detail}` : ''}
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

export function AddImportPage(props: { onUnauthorized: () => void }) {
  const [tab, setTab] = useState<'add' | 'import'>('add')
  return (
    <ToastProvider>
      <div className="awem-page">
        <div style={{ display: 'flex', gap: 8 }}>
          <button className={`awem-btn awem-btn-compact ${tab === 'add' ? 'awem-btn-primary' : 'awem-btn-secondary'}`} onClick={() => setTab('add')}>添加</button>
          <button className={`awem-btn awem-btn-compact ${tab === 'import' ? 'awem-btn-primary' : 'awem-btn-secondary'}`} onClick={() => setTab('import')}>导入</button>
        </div>
        {tab === 'add' ? <AddInner {...props} /> : <ImportInner {...props} />}
      </div>
    </ToastProvider>
  )
}
