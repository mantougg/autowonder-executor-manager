// Overview.tsx：总览页容器（左树右表，2026-10-27 改造；同日拆出纯逻辑与树/行组件）。
// 左侧归属导航树（全部 → 项目 → 小队 → 未归属），点击节点过滤右侧表格；
// 右侧保留批量选择/批量操作/5s 轮询/漂移黄标。列：选择 | 名称 | 所属小队（主）+ 所属项目（副） |
// 版本（parsed.runtime_version） | 本地进程 | 连接 | 平台 | 模型 | 自启 | 重启 | 操作。
// 无平台归属数据时隐藏左树、表格占满。
// 纯逻辑（树构建/筛选/批量辅助）见 ../overview-model；树/行组件见 components/NavTree、ExecutorRow。
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Plus } from 'lucide-react'
import { t } from '../i18n'
import { listExecutors, apiFetch, type ExecutorView } from '../api'
import { localStateText } from '../components/StatusDot'
import { Dialog } from '../components/Dialog'
import { Select } from '../components/Select'
import { useToast, ToastProvider } from '../components/Toast'
import { NavTree } from '../components/NavTree'
import { ExecutorRow } from '../components/ExecutorRow'
import { navigate } from '../router'
import {
  FILTER_ALL, LOCAL_STATE_ORDER, EMPTY_FILTERS, filterActive,
  buildTree, inTreeSel, matchesFilters, isRunningLike,
  joinNames, failureReason, loadCollapsed, saveCollapsed,
  type BatchKind, type BatchResult, type BatchTarget, type FilterState, type TreeSel,
} from '../overview-model'

const REFRESH_MS = 5000 // 轮询间隔（选定记录：5s，运维工具求近实时又不过载）

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
  const [sel, setSel] = useState<TreeSel>({ kind: 'all' })
  const [collapsed, setCollapsed] = useState<Set<string>>(loadCollapsed)
  const [filters, setFilters] = useState<FilterState>(EMPTY_FILTERS)
  const headRef = useRef<HTMLInputElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  const treeRef = useRef<HTMLDivElement>(null)

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

  const rowCount = rows?.length ?? 0
  // 平台联动/归属展示基于全量数据；筛选与树选择不改变列结构。
  const linked = rows?.some((e) => e.platform) ?? false
  const hasOwnership = (rows ?? []).some((e) => {
    const p = e.platform
    return Boolean(p && (p.project || p.squad || (p.squad_names && p.squad_names.length > 0)))
  })

  const filtersActive = filterActive(filters)
  const tree = useMemo(() => buildTree(rows ?? []), [rows])
  const viewRows = useMemo(
    () => (rows ?? []).filter((e) => inTreeSel(e, sel) && matchesFilters(e, filters)),
    [rows, sel, filters],
  )
  const filteredCount = viewRows.length

  const localOptions = useMemo(() => {
    const extra = new Set<string>()
    for (const e of rows ?? []) {
      if (!LOCAL_STATE_ORDER.includes(e.local.state)) extra.add(e.local.state)
    }
    return [...LOCAL_STATE_ORDER, ...[...extra].sort()]
  }, [rows])

  // 树选中节点随轮询失效（项目/小队消失或无归属数据）时回退「全部执行器」。
  useEffect(() => {
    if (sel.kind === 'all') return
    if (!hasOwnership) {
      setSel({ kind: 'all' })
      return
    }
    if (sel.kind === 'unowned') {
      if (tree.unowned.leaves.length === 0) setSel({ kind: 'all' })
      return
    }
    const p = tree.projects.find((x) => x.name === sel.project)
    if (!p) {
      setSel({ kind: 'all' })
      return
    }
    if (sel.kind === 'squad' && !p.squads.some((s) => s.name === sel.squad)) {
      setSel({ kind: 'all' })
    }
  }, [tree, hasOwnership, sel])

  // 选择按执行器名称保持：轮询后消失或不再命中的行移出选择，新行默认不选中。
  useEffect(() => {
    if (!rows) return
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const visible = new Set(viewRows.map((r) => r.name))
      let changed = false
      const next = new Set<string>()
      for (const name of prev) {
        if (visible.has(name)) next.add(name)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [rows, viewRows])

  // 本地进程选项随数据刷新：选中值失效时回退「全部」并清空已选。
  useEffect(() => {
    if (filters.local === FILTER_ALL || localOptions.includes(filters.local)) return
    setFilters((prev) => ({ ...prev, local: FILTER_ALL }))
    setSelected(new Set())
  }, [filters, localOptions])

  const allSelected = filteredCount > 0 && viewRows.every((r) => selected.has(r.name))
  const someSelected = viewRows.some((r) => selected.has(r.name)) && !allSelected
  useEffect(() => {
    if (headRef.current) headRef.current.indeterminate = someSelected
  }, [someSelected])

  const locked = batchBusy !== null || busyName !== ''
  const selectedRows = viewRows.filter((r) => selected.has(r.name))
  const targetsOf = (list: ExecutorView[]): BatchTarget[] =>
    list.map((r) => ({ name: r.name, label: r.platform?.name || r.name }))

  const toggleAll = () => {
    if (viewRows.length === 0) return
    setSelected((prev) =>
      viewRows.every((r) => prev.has(r.name))
        ? new Set<string>()
        : new Set(viewRows.map((r) => r.name)),
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

  const toggleCollapsed = (key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      saveCollapsed(next)
      return next
    })
  }

  // 切换树节点 = 切换过滤范围：与筛选变化一致，清空已选。
  const selectNode = (next: TreeSel) => {
    setSel(next)
    setSelected(new Set())
  }

  const updateFilters = (patch: Partial<FilterState>) => {
    setFilters((prev) => ({ ...prev, ...patch }))
    setSelected(new Set())
  }

  const resetFilters = () => {
    setFilters(EMPTY_FILTERS)
    setSelected(new Set())
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

  const columnCount = linked ? 11 : 10
  // 工具行在列表非空时常驻
  const showToolbar = rows !== null && !error && rows.length > 0
  const showTree = hasOwnership

  // 表格/左树最大高度 = 各自顶边到视口底的剩余空间（同一公式分别计算，保证两者底边对齐）；
  // 行数少时保持自然高度（flex stretch 已让树与表格区同高）；过矮时保留 240px 表体兜底
  // （允许页面滚动）。
  useLayoutEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const limitHeight = (target: HTMLElement, reserveHeader: boolean) => {
      const rect = target.getBoundingClientRect()
      const docTop = rect.top + window.scrollY
      const main = target.closest('.awem-main') as HTMLElement | null
      const bottomPad = main ? (parseFloat(getComputedStyle(main).paddingBottom) || 0) : 24
      const thead = target.querySelector('thead') as HTMLElement | null
      const headerH = reserveHeader ? Math.ceil(thead?.getBoundingClientRect().height ?? 37) : 0
      const available = Math.floor(window.innerHeight - docTop - bottomPad) - 1
      const maxHeight = Math.max(240 + headerH + 2, available) // +2 为容器上下边框
      const next = `${maxHeight}px`
      if (target.style.maxHeight !== next) target.style.maxHeight = next
    }
    const update = () => {
      limitHeight(el, true)
      const treeEl = treeRef.current
      if (treeEl) limitHeight(treeEl, false)
    }
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [rows, filters, sel, hasOwnership, error])

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
        <div className="awem-overview-layout">
          {showTree ? (
            <div ref={treeRef} className="awem-navtree-wrap">
              <NavTree
                model={tree}
                sel={sel}
                collapsed={collapsed}
                onSelect={selectNode}
                onToggle={toggleCollapsed}
              />
            </div>
          ) : null}
          <div className="awem-overview-main">
            {/* 查询条件：位于工具行上方、表格滚动容器之外；加载/错误/空列表不渲染 */}
            {showToolbar ? (
              <div className="awem-filter-bar" role="group" aria-label={t.filters.label}>
                <label className="awem-filter-field awem-filter-name">
                  <span className="awem-filter-label">{t.filters.name}</span>
                  <input
                    className="awem-input awem-filter-control"
                    value={filters.name}
                    placeholder={t.filters.name}
                    onChange={(e) => updateFilters({ name: e.target.value })}
                  />
                </label>
                <div className="awem-filter-field">
                  <span className="awem-filter-label">{t.filters.local}</span>
                  <Select
                    ariaLabel={t.filters.local}
                    value={filters.local}
                    options={[
                      { value: FILTER_ALL, label: t.filters.all },
                      ...localOptions.map((s) => ({ value: s, label: localStateText[s] ?? s })),
                    ]}
                    onChange={(v) => updateFilters({ local: v })}
                  />
                </div>
                <button
                  type="button"
                  className="awem-btn awem-btn-secondary awem-btn-compact awem-filter-reset"
                  onClick={resetFilters}
                  disabled={!filtersActive}
                >{t.filters.reset}</button>
              </div>
            ) : null}
            {/* 工具行常驻（行高锁定 32px）：无选中显示计数提示，有选中切换为批量操作条，
                避免批量条出现/消失把表格顶上顶下 */}
            {showToolbar ? (
              <div className="awem-table-toolbar">
                {selected.size > 0 ? (
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
                ) : (
                  <span className="awem-hint">
                    {filtersActive || sel.kind !== 'all'
                      ? t.filters.count(filteredCount, rowCount)
                      : t.filters.total(rowCount)}
                  </span>
                )}
              </div>
            ) : null}
            <div className="awem-table-wrap awem-overview-table-wrap" ref={wrapRef}>
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
                        disabled={locked || filteredCount === 0}
                        onChange={toggleAll}
                      />
                    </th>
                    <th>名称</th>
                    {linked ? <th>{t.columns.squad}</th> : null}
                    <th>{t.columns.version}</th>
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
                  {filteredCount === 0 && filtersActive ? (
                    <tr className="awem-filter-empty">
                      <td colSpan={columnCount}>
                        <div className="awem-filter-empty-cell">
                          <span>{t.filters.empty}</span>
                          <button
                            className="awem-btn awem-btn-secondary awem-btn-compact"
                            onClick={resetFilters}
                          >{t.filters.clear}</button>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    viewRows.map((e) => (
                      <ExecutorRow
                        key={e.name}
                        e={e}
                        linked={linked}
                        selected={selected.has(e.name)}
                        locked={locked}
                        busyName={busyName}
                        batchBusy={batchBusy}
                        onToggle={toggleOne}
                        onAct={act}
                        onStop={(x) => setConfirm({ kind: 'stop', name: x.name, running: x.local.state === 'running' })}
                        onRemove={(x) => setConfirm({
                          kind: 'remove', name: x.name,
                          running: isRunningLike(x.local.state),
                        })}
                      />
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
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
