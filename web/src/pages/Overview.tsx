// Overview.tsx：总览页（F3）。表格：首列多选 + 名称(+漂移 pill)/本地进程态/连接状态/
// 平台列(列头注明来源)/model/autostart/重启次数 + 行内 start/stop/restart
// ghost 图标按钮（hover/焦点显现）+ remove；stop/remove 走 Dialog 确认；
// 选中后显示批量操作条（启动/停止/删除串行调用单条 API）；5s 轮询刷新；空状态；加载骨架。
import { useCallback, useEffect, useRef, useState } from 'react'
import { Play, Square, RotateCw, Trash2, Plus } from 'lucide-react'
import { t } from '../i18n'
import {
  listExecutors, apiFetch, APIError, type ExecutorView,
} from '../api'
import { StatusDot, localStateKind, localStateText, platformKind, platformText } from '../components/StatusDot'
import { Dialog } from '../components/Dialog'
import { useToast } from '../components/Toast'
import { ToastProvider } from '../components/Toast'
import { navigate } from '../router'

const REFRESH_MS = 5000 // 轮询间隔（选定记录：5s，运维工具求近实时又不过载）

const RUNNING_LIKE = new Set(['running', 'backoff', 'stopping'])
const isRunningLike = (state: string) => RUNNING_LIKE.has(state)

// 名称列表：超过 limit 时列前 limit 个并注明总数（工单：等 N 个）。
function joinNames(names: string[], limit: number): string {
  if (names.length <= limit) return names.join('、')
  return `${names.slice(0, limit).join('、')} 等 ${names.length} 个`
}

type BatchKind = 'start' | 'stop' | 'delete'

interface BatchTarget {
  name: string
  label: string
}

interface BatchResult {
  name: string
  ok: boolean
  reason?: string
}

function failureReason(err: unknown): string {
  if (err instanceof APIError) {
    if (err.httpStatus === 404) return t.batch.reason.notFound
    if (err.httpStatus === 409) return t.batch.reason.running
    return err.message || t.batch.reason.unknown
  }
  return (err as Error).message || t.batch.reason.unknown
}

function OverviewInner({ onUnauthorized }: { onUnauthorized: () => void }) {
  const toast = useToast()
  const [rows, setRows] = useState<ExecutorView[] | null>(null) // null=加载中
  const [error, setError] = useState('')
  const [confirm, setConfirm] = useState<
    { kind: 'stop' | 'remove'; name: string; running: boolean } | null
  >(null)
  const [busyName, setBusyName] = useState('')
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const [batchBusy, setBatchBusy] = useState<BatchKind | null>(null)
  const [batchConfirm, setBatchConfirm] = useState<
    { kind: 'stop' | 'delete'; targets: BatchTarget[] } | null
  >(null)
  const headRef = useRef<HTMLInputElement>(null)

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

  // 选择按执行器名称保持：轮询后消失的行移出选择，新行默认不选中。
  useEffect(() => {
    if (!rows) return
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const visible = new Set(rows.map((r) => r.name))
      let changed = false
      const next = new Set<string>()
      for (const name of prev) {
        if (visible.has(name)) next.add(name)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [rows])

  const rowCount = rows?.length ?? 0
  const allSelected = rowCount > 0 && (rows ?? []).every((r) => selected.has(r.name))
  const someSelected = selected.size > 0 && !allSelected
  useEffect(() => {
    if (headRef.current) headRef.current.indeterminate = someSelected
  }, [someSelected])

  const locked = batchBusy !== null || busyName !== ''
  const selectedRows = (rows ?? []).filter((r) => selected.has(r.name))
  const targetsOf = (list: ExecutorView[]): BatchTarget[] =>
    list.map((r) => ({ name: r.name, label: r.platform?.name || r.name }))

  const toggleAll = () => {
    if (!rows || rows.length === 0) return
    setSelected((prev) =>
      rows.every((r) => prev.has(r.name))
        ? new Set<string>()
        : new Set(rows.map((r) => r.name)),
    )
  }

  const toggleOne = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

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

  // 批量执行：按点击时刻名单串行调用现有单条 API；成功项取消勾选，失败项保留。
  const batchRun = async (kind: BatchKind, targets: BatchTarget[]) => {
    setBatchBusy(kind)
    const results: BatchResult[] = []
    try {
      for (const target of targets) {
        try {
          if (kind === 'delete') {
            await apiFetch(
              `/api/executors/${encodeURIComponent(target.name)}?stop=false&purge=false`,
              { method: 'DELETE' },
              onUnauthorized,
            )
          } else {
            await apiFetch(
              `/api/executors/${encodeURIComponent(target.name)}/${kind}`,
              { method: 'POST' },
              onUnauthorized,
            )
          }
          results.push({ name: target.name, ok: true })
        } catch (e) {
          results.push({ name: target.name, ok: false, reason: failureReason(e) })
        }
      }
    } finally {
      const succeeded = new Set(results.filter((r) => r.ok).map((r) => r.name))
      if (succeeded.size > 0) {
        setSelected((prev) => {
          const next = new Set(prev)
          for (const name of succeeded) next.delete(name)
          return next
        })
      }
      setBatchBusy(null)
      refresh()
    }

    const failed = results.filter((r) => !r.ok)
    const okCount = results.length - failed.length
    if (failed.length === 0) {
      toast(t.batch.done[kind](okCount))
      return
    }
    const labels = new Map(targets.map((x) => [x.name, x.label]))
    const detail = joinNames(
      failed.map((f) => `${labels.get(f.name) ?? f.name}（${f.reason}）`),
      3,
    )
    toast(t.batch.partial(t.batch.action[kind], okCount, failed.length, detail), 'error')
  }

  const startSelected = () => {
    const targets = targetsOf(selectedRows)
    if (targets.length > 0) batchRun('start', targets)
  }

  const stopSelected = () => {
    const targets = targetsOf(selectedRows)
    if (targets.length > 0) setBatchConfirm({ kind: 'stop', targets })
  }

  const deleteSelected = () => {
    const running = selectedRows.filter((r) => isRunningLike(r.local.state))
    if (running.length > 0) {
      toast(t.batch.deleteBlocked(running.length, joinNames(running.map((r) => r.platform?.name || r.name), 3)), 'error')
      return
    }
    const targets = targetsOf(selectedRows)
    if (targets.length > 0) setBatchConfirm({ kind: 'delete', targets })
  }

  // 删除确认后二次校验：拉取最新列表，若期间有项变为运行中仍整批阻止。
  const confirmBatchDelete = async (targets: BatchTarget[]) => {
    setBatchConfirm(null)
    setBatchBusy('delete')
    let fresh: ExecutorView[]
    try {
      fresh = (await listExecutors(onUnauthorized)).executors
    } catch {
      setBatchBusy(null)
      toast(t.batch.refreshFailed, 'error')
      return
    }
    setRows(fresh)
    const byName = new Map(fresh.map((r) => [r.name, r]))
    const running = targets.filter((x) => {
      const r = byName.get(x.name)
      return r ? isRunningLike(r.local.state) : false
    })
    if (running.length > 0) {
      setBatchBusy(null)
      toast(t.batch.deleteBlocked(running.length, joinNames(running.map((x) => x.label), 3)), 'error')
      return
    }
    await batchRun('delete', targets)
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

      {selected.size > 0 && rows && rows.length > 0 ? (
        <div className="awem-bulk-bar" role="group" aria-label={t.batch.barLabel}>
          <span className="awem-bulk-count">{t.batch.selected(selected.size)}</span>
          <button
            className="awem-btn awem-btn-secondary awem-btn-compact"
            disabled={locked}
            onClick={startSelected}
          >{t.batch.action.start}</button>
          <button
            className="awem-btn awem-btn-secondary awem-btn-compact"
            disabled={locked}
            onClick={stopSelected}
          >{t.batch.action.stop}</button>
          <button
            className="awem-btn awem-btn-danger awem-btn-compact"
            disabled={locked}
            onClick={deleteSelected}
          >{t.batch.action.delete}</button>
          <button
            className="awem-btn awem-btn-ghost awem-btn-compact"
            disabled={locked}
            onClick={() => setSelected(new Set())}
          >{t.batch.clear}</button>
        </div>
      ) : null}

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
                <th className="awem-col-check">
                  <input
                    ref={headRef}
                    type="checkbox"
                    className="awem-checkbox"
                    aria-label={t.batch.selectAll}
                    checked={allSelected}
                    disabled={locked}
                    onChange={toggleAll}
                  />
                </th>
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
                  <td className="awem-col-check">
                    <input
                      type="checkbox"
                      className="awem-checkbox"
                      aria-label={t.batch.selectRow(e.name)}
                      checked={selected.has(e.name)}
                      disabled={locked}
                      onChange={() => toggleOne(e.name)}
                    />
                  </td>
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
                        disabled={busyName === e.name || batchBusy !== null}
                        onClick={() => act(e.name, 'start')}
                      ><Play size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.stop} aria-label={`${t.actions.stop} ${e.name}`}
                        disabled={busyName === e.name || batchBusy !== null}
                        onClick={() => setConfirm({ kind: 'stop', name: e.name, running: e.local.state === 'running' })}
                      ><Square size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.restart} aria-label={`${t.actions.restart} ${e.name}`}
                        disabled={busyName === e.name || batchBusy !== null}
                        onClick={() => act(e.name, 'restart')}
                      ><RotateCw size={15} aria-hidden /></button>
                      <button
                        className="awem-btn awem-btn-ghost" title={t.actions.remove} aria-label={`${t.actions.remove} ${e.name}`}
                        disabled={busyName === e.name || batchBusy !== null}
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

      {batchConfirm ? (
        <Dialog
          title={
            batchConfirm.kind === 'stop'
              ? t.batch.stopDialogTitle(batchConfirm.targets.length)
              : t.batch.deleteDialogTitle(batchConfirm.targets.length)
          }
          body={
            batchConfirm.kind === 'stop'
              ? t.batch.stopDialogBody(batchConfirm.targets.length)
              : (
                <div>
                  <div>{t.batch.deleteDialogIntro(batchConfirm.targets.length)}</div>
                  <div className="awem-hint" style={{ marginTop: 8 }}>
                    {joinNames(batchConfirm.targets.map((x) => x.label), 10)}
                  </div>
                </div>
              )
          }
          confirmText={
            batchConfirm.kind === 'stop'
              ? t.batch.stopDialogConfirm(batchConfirm.targets.length)
              : t.batch.deleteDialogConfirm(batchConfirm.targets.length)
          }
          danger
          onCancel={() => setBatchConfirm(null)}
          onConfirm={() => {
            const c = batchConfirm
            if (c.kind === 'stop') {
              setBatchConfirm(null)
              batchRun('stop', c.targets)
            } else {
              confirmBatchDelete(c.targets)
            }
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
