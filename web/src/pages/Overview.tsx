// Overview.tsx：总览页（F3）。表格：名称(+漂移 pill)/本地进程态/连接状态/
// 平台列(列头注明来源)/model/autostart/重启次数 + 行内 start/stop/restart
// ghost 图标按钮（hover/焦点显现）+ remove；stop/remove 走 Dialog 确认；
// 5s 轮询刷新；空状态；加载骨架。
import { useCallback, useEffect, useState } from 'react'
import { Play, Square, RotateCw, Trash2, Plus } from 'lucide-react'
import { t } from '../i18n'
import {
  listExecutors, apiFetch, type ExecutorView,
} from '../api'
import { StatusDot, localStateKind, localStateText, platformKind, platformText } from '../components/StatusDot'
import { Dialog } from '../components/Dialog'
import { useToast } from '../components/Toast'
import { ToastProvider } from '../components/Toast'
import { navigate } from '../router'

const REFRESH_MS = 5000 // 轮询间隔（选定记录：5s，运维工具求近实时又不过载）

function OverviewInner({ onUnauthorized }: { onUnauthorized: () => void }) {
  const toast = useToast()
  const [rows, setRows] = useState<ExecutorView[] | null>(null) // null=加载中
  const [error, setError] = useState('')
  const [confirm, setConfirm] = useState<
    { kind: 'stop' | 'remove'; name: string; running: boolean } | null
  >(null)
  const [busyName, setBusyName] = useState('')

  const refresh = useCallback(() => {
    listExecutors(onUnauthorized)
      .then((d) => {
        setRows(d.executors)
        setError('')
      })
      .catch((e) => setError(e.message))
  }, [onUnauthorized])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, REFRESH_MS)
    return () => clearInterval(id)
  }, [refresh])

  const act = async (name: string, action: 'start' | 'stop' | 'restart') => {
    setBusyName(name)
    try {
      await apiFetch(`/api/executors/${encodeURIComponent(name)}/${action}`, { method: 'POST' }, onUnauthorized)
      toast(`${name} 已${action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'}`)
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setBusyName('')
      refresh()
    }
  }

  const doRemove = async (name: string, running: boolean) => {
    setBusyName(name)
    try {
      const q = running ? '?stop=true&purge=false' : ''
      await apiFetch(`/api/executors/${encodeURIComponent(name)}${q}`, { method: 'DELETE' }, onUnauthorized)
      toast(`已删除 ${name}${running ? '（已停止进程）' : ''}`)
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setBusyName('')
      refresh()
    }
  }

  // 平台联动展示判定：任一行有平台数据即展示归属列（未联动隐藏两列，
  // 名称回退标识名——design §7.4）
  const linked = rows?.some((e) => e.platform) ?? false

  return (
    <div className="awem-page">
      <div className="awem-page-header">
        <h1 className="awem-page-title">{t.nav.overview}</h1>
        <button className="awem-btn awem-btn-primary" onClick={() => navigate('/add')}>
          <Plus size={16} aria-hidden />{t.actions.addExecutor}
        </button>
      </div>

      {rows === null ? (
        <div className="awem-table-wrap" aria-busy="true">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, padding: 4 }}>
            <div className="awem-skeleton" /><div className="awem-skeleton" /><div className="awem-skeleton" />
          </div>
        </div>
      ) : error ? (
        <div className="awem-empty">
          <span className="awem-error-text">{error}</span>
          <button className="awem-btn awem-btn-secondary" onClick={refresh}>{t.common.retry}</button>
        </div>
      ) : rows.length === 0 ? (
        <div className="awem-empty awem-card">
          <span>还没有执行器</span>
          <button className="awem-btn awem-btn-primary" onClick={() => navigate('/add')}>
            {t.actions.addExecutor}
          </button>
        </div>
      ) : (
        <div className="awem-table-wrap">
          <table className="awem-table">
            <thead>
              <tr>
                <th>名称</th>
                {linked ? (
                  <>
                    <th>所属项目</th>
                    <th>所属小队</th>
                  </>
                ) : null}
                <th>{t.status.local}</th>
                <th>连接</th>
                <th>{t.status.platform}（平台轮询）</th>
                <th>模型</th>
                <th>自启</th>
                <th>重启</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {rows.map((e) => (
                <tr key={e.name}>
                  <td>
                    <a
                      href={`/executors/${encodeURIComponent(e.name)}`}
                      className={e.platform?.name ? '' : 'mono'}
                      title={e.platform?.agent_name ? `数字工人：${e.platform.agent_name}` : undefined}
                      onClick={(ev) => { ev.preventDefault(); navigate(`/executors/${encodeURIComponent(e.name)}`) }}
                    >
                      {e.platform?.name || e.name}
                    </a>
                    {e.platform?.name ? (
                      <div className="awem-hint mono" style={{ fontSize: 12 }}>{e.name}</div>
                    ) : null}
                    {e.drift && e.drift.fields.length > 0 ? (
                      <span className="awem-pill warn" style={{ marginLeft: 8 }} title={e.drift.message}>
                        {t.status.drift} {e.drift.fields.length}
                      </span>
                    ) : null}
                  </td>
                  {linked ? (
                    <>
                      <td className="awem-hint">{e.platform?.project || '—'}</td>
                      <td className="awem-hint">{e.platform?.squad || '—'}</td>
                    </>
                  ) : null}
                  <td><StatusDot kind={localStateKind(e.local.state)} text={localStateText[e.local.state] ?? e.local.state} /></td>
                  <td className="awem-hint">{e.local.conn === 'connected' ? t.status.connected : e.local.conn === 'reconnecting' ? t.status.reconnecting : e.local.conn}</td>
                  <td>
                    {e.platform ? (
                      <StatusDot kind={platformKind(e.platform.status)} text={platformText[e.platform.status] ?? e.platform.status} />
                    ) : (
                      <span className="awem-hint">—</span>
                    )}
                  </td>
                  <td className="mono">{e.parsed?.model || '—'}</td>
                  <td>{e.autostart ? '✓' : '—'}</td>
                  <td>{e.local.restarts}</td>
                  <td>
                    <span className="awem-row-actions">
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.start} aria-label={`${t.actions.start} ${e.name}`}
                        disabled={busyName === e.name}
                        onClick={() => act(e.name, 'start')}
                      ><Play size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.stop} aria-label={`${t.actions.stop} ${e.name}`}
                        disabled={busyName === e.name}
                        onClick={() => setConfirm({ kind: 'stop', name: e.name, running: e.local.state === 'running' })}
                      ><Square size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.restart} aria-label={`${t.actions.restart} ${e.name}`}
                        disabled={busyName === e.name}
                        onClick={() => act(e.name, 'restart')}
                      ><RotateCw size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.remove} aria-label={`${t.actions.remove} ${e.name}`}
                        disabled={busyName === e.name}
                        onClick={() => setConfirm({
                          kind: 'remove', name: e.name,
                          running: e.local.state === 'running' || e.local.state === 'backoff' || e.local.state === 'stopping',
                        })}
                      ><Trash2 size={15} aria-hidden /></button>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {confirm ? (
        <Dialog
          title={confirm.kind === 'stop' ? `停止执行器 ${confirm.name}` : `删除执行器 ${confirm.name}`}
          body={
            confirm.kind === 'stop'
              ? `将停止 ${confirm.name} 的子进程（整棵进程树优雅终止）。`
              : `将删除 ${confirm.name} 的配置${confirm.running ? '（正在运行，将先停止进程）' : ''}，本地日志保留。此操作不可恢复。`
          }
          confirmText={confirm.kind === 'stop' ? `停止 ${confirm.name}` : `删除 ${confirm.name}`}
          danger
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const c = confirm
            setConfirm(null)
            if (c.kind === 'stop') act(c.name, 'stop')
            else doRemove(c.name, c.running)
          }}
        />
      ) : null}
    </div>
  )
}

export function OverviewPage(props: { onUnauthorized: () => void }) {
  return (
    <ToastProvider>
      <OverviewInner {...props} />
    </ToastProvider>
  )
}
