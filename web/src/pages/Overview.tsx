// Overview.tsx：总览页（一期展示改造）。
// 标题下 Tab 容器（本期仅「列表」，hash #tab=list）+ 列表 tab：
// 保留原表格/批量/5s 轮询能力，新增「平铺 ⇄ 树形」（项目 → 小队 → 执行器）。
// 树形多小队执行器重复叶子、选择按执行器标识去重；未归属组固定排最后。
import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Play, Square, RotateCw, Trash2, Plus, ChevronDown, ChevronRight } from 'lucide-react'
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
const TAB_HASH = '#tab=list'
const TREE_KEY = 'awem-overview-tree-collapsed'

const RUNNING_LIKE = new Set(['running', 'backoff', 'stopping'])
const isRunningLike = (state: string) => RUNNING_LIKE.has(state)

type BatchKind = 'start' | 'stop' | 'delete'
type ViewMode = 'flat' | 'tree'

// 筛选哨兵值：与真实项目/小队名隔离；选项标签走 i18n。
const FILTER_ALL = '__all__'
const FILTER_UNOWNED = '__unowned__'
const FILTER_UNASSIGNED_SQUAD = '__unassigned_squad__'
const LOCAL_STATE_ORDER = ['running', 'backoff', 'stopping', 'stopped', 'failed']

interface FilterState {
  name: string
  project: string
  squad: string
  local: string
}

const EMPTY_FILTERS: FilterState = {
  name: '',
  project: FILTER_ALL,
  squad: FILTER_ALL,
  local: FILTER_ALL,
}

function filterActive(f: FilterState): boolean {
  return f.name.trim() !== ''
    || f.project !== FILTER_ALL
    || f.squad !== FILTER_ALL
    || f.local !== FILTER_ALL
}

interface BatchTarget {
  name: string
  label: string
}

interface BatchResult {
  name: string
  ok: boolean
  reason?: string
}

interface SquadGroup {
  key: string
  name: string
  leaves: ExecutorView[]
  names: string[]
  online: number
}

interface ProjectGroup {
  key: string
  name: string
  squads: SquadGroup[]
  names: string[]
  online: number
}

interface TreeModel {
  projects: ProjectGroup[]
  unowned: SquadGroup
}

// 名称列表：超过 limit 时列前 limit 个并注明总数（工单：等 N 个）。
function joinNames(names: string[], limit: number): string {
  if (names.length <= limit) return names.join('、')
  return `${names.slice(0, limit).join('、')} 等 ${names.length} 个`
}

function failureReason(err: unknown): string {
  if (err instanceof APIError) {
    if (err.httpStatus === 404) return t.batch.reason.notFound
    if (err.httpStatus === 409) return t.batch.reason.running
    return err.message || t.batch.reason.unknown
  }
  return (err as Error).message || t.batch.reason.unknown
}

function loadCollapsed(): Set<string> {
  try {
    const raw = sessionStorage.getItem(TREE_KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as unknown
    return new Set(Array.isArray(arr) ? arr.filter((x): x is string => typeof x === 'string') : [])
  } catch {
    return new Set()
  }
}

function saveCollapsed(s: Set<string>) {
  try {
    sessionStorage.setItem(TREE_KEY, JSON.stringify([...s]))
  } catch {
    /* 存储不可用时仅会话内失效 */
  }
}

function isOnline(e: ExecutorView): boolean {
  return e.platform?.status === 'ONLINE' || e.platform?.status === 'BUSY'
}

function uniqueNames(leaves: ExecutorView[]): string[] {
  return [...new Set(leaves.map((e) => e.name))]
}

function onlineOf(leaves: ExecutorView[]): number {
  const seen = new Map<string, boolean>()
  for (const e of leaves) {
    if (!seen.has(e.name)) seen.set(e.name, isOnline(e))
  }
  let n = 0
  for (const v of seen.values()) if (v) n++
  return n
}

function squadNamesOf(e: ExecutorView): string[] {
  const arr = e.platform?.squad_names
  if (Array.isArray(arr) && arr.length > 0) return arr
  const joined = e.platform?.squad
  return joined ? joined.split('、').filter(Boolean) : []
}

// 四项条件 AND；名称匹配平台显示名或执行器标识（去空格、不区分大小写子串）。
function matchesFilters(e: ExecutorView, f: FilterState): boolean {
  const q = f.name.trim().toLocaleLowerCase()
  if (q) {
    const names = [e.platform?.name ?? '', e.name]
    if (!names.some((n) => n.toLocaleLowerCase().includes(q))) return false
  }
  if (f.project !== FILTER_ALL) {
    const project = e.platform?.project ?? ''
    if (f.project === FILTER_UNOWNED ? project !== '' : project !== f.project) return false
  }
  if (f.squad !== FILTER_ALL) {
    const squads = squadNamesOf(e)
    if (f.squad === FILTER_UNASSIGNED_SQUAD ? squads.length > 0 : !squads.includes(f.squad)) return false
  }
  if (f.local !== FILTER_ALL && e.local.state !== f.local) return false
  return true
}

const collator = new Intl.Collator('zh-CN')

function buildTree(rows: ExecutorView[]): TreeModel {
  const projects = new Map<string, Map<string, ExecutorView[]>>()
  const unownedLeaves: ExecutorView[] = []
  for (const e of rows) {
    const project = e.platform?.project
    if (!project) {
      unownedLeaves.push(e)
      continue
    }
    let squads = [...new Set(squadNamesOf(e))]
    if (squads.length === 0) squads = [t.tree.unassignedSquad]
    let bySquad = projects.get(project)
    if (!bySquad) {
      bySquad = new Map()
      projects.set(project, bySquad)
    }
    for (const squad of squads) {
      const list = bySquad.get(squad) ?? []
      list.push(e)
      bySquad.set(squad, list)
    }
  }
  const projectGroups: ProjectGroup[] = [...projects.entries()]
    .map(([name, bySquad]) => {
      const squads: SquadGroup[] = [...bySquad.entries()]
        .map(([squadName, leaves]) => {
          const names = uniqueNames(leaves)
          return {
            key: `squad:${name}/${squadName}`,
            name: squadName,
            leaves,
            names,
            online: onlineOf(leaves),
          }
        })
        .sort((a, b) => collator.compare(a.name, b.name))
      const allLeaves = squads.flatMap((s) => s.leaves)
      const names = uniqueNames(allLeaves)
      return { key: `project:${name}`, name, squads, names, online: onlineOf(allLeaves) }
    })
    .sort((a, b) => collator.compare(a.name, b.name))
  const unames = uniqueNames(unownedLeaves)
  return {
    projects: projectGroups,
    unowned: {
      key: 'unowned',
      name: t.tree.unowned,
      leaves: unownedLeaves,
      names: unames,
      online: 0,
    },
  }
}

// TriCheckbox：支持 indeterminate 的复选框（组行全选/半选用）。
function TriCheckbox({
  label, checked, indeterminate, disabled, onChange,
}: {
  label: string
  checked: boolean
  indeterminate: boolean
  disabled: boolean
  onChange: () => void
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate
  }, [indeterminate])
  return (
    <input
      ref={ref}
      type="checkbox"
      className="awem-checkbox"
      aria-label={label}
      checked={checked}
      disabled={disabled}
      onChange={onChange}
    />
  )
}

function GroupRow({
  level, groupKey, project, name, names, selected, locked, collapsed,
  badge, checkboxLabel, colSpan, onToggleCollapsed, onToggleAll,
}: {
  level: number
  groupKey: string
  project?: string
  name: string
  names: string[]
  selected: Set<string>
  locked: boolean
  collapsed: boolean
  badge: string
  checkboxLabel: string
  colSpan: number
  onToggleCollapsed: () => void
  onToggleAll: (names: string[]) => void
}) {
  const all = names.length > 0 && names.every((n) => selected.has(n))
  const some = names.some((n) => selected.has(n)) && !all
  return (
    <tr className="awem-tree-group" data-level={level} data-project={project ?? ''} data-key={groupKey}>
      <td colSpan={colSpan} style={{ padding: 0 }}>
        <div className="awem-tree-group-cell" style={{ paddingLeft: level * 16 }}>
          <TriCheckbox
            label={checkboxLabel}
            checked={all}
            indeterminate={some}
            disabled={locked}
            onChange={() => onToggleAll(names)}
          />
          <button
            type="button"
            className="awem-tree-toggle"
            aria-expanded={!collapsed}
            aria-label={collapsed ? t.tree.expand(name) : t.tree.collapse(name)}
            onClick={onToggleCollapsed}
          >
            {collapsed ? <ChevronRight size={15} aria-hidden /> : <ChevronDown size={15} aria-hidden />}
            <span className="awem-tree-name">{name}</span>
          </button>
          <span className="awem-tree-badge">{badge}</span>
        </div>
      </td>
    </tr>
  )
}

function ExecutorRow({
  e, level, project, squad, linked, selected, locked, busyName, batchBusy,
  onToggle, onAct, onStop, onRemove,
}: {
  e: ExecutorView
  level: number
  project?: string
  squad?: string
  linked: boolean
  selected: boolean
  locked: boolean
  busyName: string
  batchBusy: BatchKind | null
  onToggle: (name: string) => void
  onAct: (name: string, action: 'start' | 'stop' | 'restart') => void
  onStop: (e: ExecutorView) => void
  onRemove: (e: ExecutorView) => void
}) {
  const treeAttrs = level > 0
    ? { 'data-project': project ?? '', 'data-squad': squad ?? '' }
    : {}
  return (
    <tr className={level > 0 ? 'awem-tree-leaf' : undefined} {...treeAttrs}>
      <td className="awem-col-check">
        <input
          type="checkbox"
          className="awem-checkbox"
          aria-label={t.batch.selectRow(e.name)}
          checked={selected}
          disabled={locked}
          onChange={() => onToggle(e.name)}
        />
      </td>
      <td style={level > 0 ? { paddingLeft: level * 16 } : undefined}>
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
            onClick={() => onAct(e.name, 'start')}
          ><Play size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.stop} aria-label={`${t.actions.stop} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onStop(e)}
          ><Square size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.restart} aria-label={`${t.actions.restart} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onAct(e.name, 'restart')}
          ><RotateCw size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.remove} aria-label={`${t.actions.remove} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onRemove(e)}
          ><Trash2 size={15} aria-hidden /></button>
        </span>
      </td>
    </tr>
  )
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
  const [mode, setMode] = useState<ViewMode>('flat')
  const [collapsed, setCollapsed] = useState<Set<string>>(loadCollapsed)
  const [filters, setFilters] = useState<FilterState>(EMPTY_FILTERS)
  const headRef = useRef<HTMLInputElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)

  // Tab 选择写 URL hash（本期仅 #tab=list），刷新/前进后退保持。
  useEffect(() => {
    const ensure = () => {
      if (location.hash !== TAB_HASH) history.replaceState(null, '', TAB_HASH)
    }
    ensure()
    window.addEventListener('hashchange', ensure)
    return () => window.removeEventListener('hashchange', ensure)
  }, [])

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
  // 平台联动/归属展示基于全量数据；筛选不改变列结构。
  const linked = rows?.some((e) => e.platform) ?? false
  const hasOwnership = (rows ?? []).some((e) => {
    const p = e.platform
    return Boolean(p && (p.project || p.squad || (p.squad_names && p.squad_names.length > 0)))
  })

  const filtersActive = filterActive(filters)
  const filteredRows = useMemo(
    () => (rows ?? []).filter((e) => matchesFilters(e, filters)),
    [rows, filters],
  )
  const filteredCount = filteredRows.length

  const projectOptions = useMemo(() => {
    const set = new Set<string>()
    for (const e of rows ?? []) if (e.platform?.project) set.add(e.platform.project)
    return [...set].sort(collator.compare)
  }, [rows])
  const squadOptions = useMemo(() => {
    const set = new Set<string>()
    for (const e of rows ?? []) for (const s of squadNamesOf(e)) set.add(s)
    return [...set].sort(collator.compare)
  }, [rows])
  const localOptions = useMemo(() => {
    const extra = new Set<string>()
    for (const e of rows ?? []) {
      if (!LOCAL_STATE_ORDER.includes(e.local.state)) extra.add(e.local.state)
    }
    return [...LOCAL_STATE_ORDER, ...[...extra].sort()]
  }, [rows])

  // 选择按执行器名称保持：轮询后消失或不再命中的行移出选择，新行默认不选中。
  useEffect(() => {
    if (!rows) return
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const visible = new Set(filteredRows.map((r) => r.name))
      let changed = false
      const next = new Set<string>()
      for (const name of prev) {
        if (visible.has(name)) next.add(name)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [rows, filteredRows])

  // 选项随数据刷新：选中值失效或归属条件被隐藏时回退「全部」并清空已选。
  useEffect(() => {
    const projectInvalid = filters.project !== FILTER_ALL
      && filters.project !== FILTER_UNOWNED
      && !projectOptions.includes(filters.project)
    const squadInvalid = filters.squad !== FILTER_ALL
      && filters.squad !== FILTER_UNASSIGNED_SQUAD
      && !squadOptions.includes(filters.squad)
    const localInvalid = filters.local !== FILTER_ALL && !localOptions.includes(filters.local)
    const ownershipHidden = !hasOwnership
      && (filters.project !== FILTER_ALL || filters.squad !== FILTER_ALL)
    if (!projectInvalid && !squadInvalid && !localInvalid && !ownershipHidden) return
    setFilters((prev) => {
      const next = { ...prev }
      if (projectInvalid || ownershipHidden) next.project = FILTER_ALL
      if (squadInvalid || ownershipHidden) next.squad = FILTER_ALL
      if (localInvalid) next.local = FILTER_ALL
      return next
    })
    setSelected(new Set())
  }, [filters, projectOptions, squadOptions, localOptions, hasOwnership])

  const allSelected = filteredCount > 0 && filteredRows.every((r) => selected.has(r.name))
  const someSelected = filteredRows.some((r) => selected.has(r.name)) && !allSelected
  useEffect(() => {
    if (headRef.current) headRef.current.indeterminate = someSelected
  }, [someSelected])

  const locked = batchBusy !== null || busyName !== ''
  const selectedRows = filteredRows.filter((r) => selected.has(r.name))
  const targetsOf = (list: ExecutorView[]): BatchTarget[] =>
    list.map((r) => ({ name: r.name, label: r.platform?.name || r.name }))

  const toggleAll = () => {
    if (filteredRows.length === 0) return
    setSelected((prev) =>
      filteredRows.every((r) => prev.has(r.name))
        ? new Set<string>()
        : new Set(filteredRows.map((r) => r.name)),
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

  const toggleGroup = (names: string[]) => {
    setSelected((prev) => {
      const next = new Set(prev)
      const all = names.length > 0 && names.every((n) => next.has(n))
      for (const n of names) {
        if (all) next.delete(n)
        else next.add(n)
      }
      return next
    })
  }

  const toggleCollapsed = (key: string) => {
    if (filtersActive) return // 筛选期间命中路径统一展开，不写入折叠记忆
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      saveCollapsed(next)
      return next
    })
  }

  const updateFilters = (patch: Partial<FilterState>) => {
    setFilters((prev) => ({ ...prev, ...patch }))
    setSelected(new Set())
  }

  const resetFilters = () => {
    setFilters(EMPTY_FILTERS)
    setSelected(new Set())
  }

  const isCollapsed = (key: string) => !filtersActive && collapsed.has(key)

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

  useEffect(() => {
    if (!hasOwnership && mode === 'tree') setMode('flat')
  }, [hasOwnership, mode])

  const tree = useMemo(() => buildTree(filteredRows), [filteredRows])
  const columnCount = linked ? 12 : 10
  // 工具行在列表非空时常驻（无归属数据也保留，仅隐藏平铺/树形切换）
  const showToolbar = rows !== null && !error && rows.length > 0

  // 表体最大高度 = 视口剩余空间（顶栏/页头/Tab/筛选行/工具行/表头之外），
  // 行数少时保持自然高度；过矮时保留 240px 表体兜底（允许页面滚动）。
  useLayoutEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const update = () => {
      const rect = el.getBoundingClientRect()
      const docTop = rect.top + window.scrollY
      const main = el.closest('.awem-main') as HTMLElement | null
      const bottomPad = main ? (parseFloat(getComputedStyle(main).paddingBottom) || 0) : 24
      const thead = el.querySelector('thead') as HTMLElement | null
      const headerH = Math.ceil(thead?.getBoundingClientRect().height ?? 37)
      const available = Math.floor(window.innerHeight - docTop - bottomPad) - 1
      const maxHeight = Math.max(240 + headerH + 2, available) // +2 为容器上下边框
      const next = `${maxHeight}px`
      if (el.style.maxHeight !== next) el.style.maxHeight = next
    }
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [rows, mode, filters, hasOwnership, error])

  return (
    <div className="awem-page">
      <div className="awem-page-header">
        <h1 className="awem-page-title">{t.nav.overview}</h1>
        <button className="awem-btn awem-btn-primary" onClick={() => navigate('/add')}>
          <Plus size={16} aria-hidden />{t.actions.addExecutor}
        </button>
      </div>

      <div className="awem-tabs" role="tablist" aria-label={t.tabs.label}>
        <button
          type="button"
          role="tab"
          id="tab-list"
          aria-selected="true"
          aria-controls="tabpanel-list"
          tabIndex={0}
          className="awem-tab"
          onKeyDown={(e) => {
            if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) e.preventDefault()
          }}
        >
          {t.tabs.list}
        </button>
      </div>

      <div role="tabpanel" id="tabpanel-list" aria-labelledby="tab-list">
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
          <>
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
                {hasOwnership ? (
                  <>
                    <label className="awem-filter-field">
                      <span className="awem-filter-label">{t.filters.project}</span>
                      <select
                        className="awem-select awem-filter-control"
                        value={filters.project}
                        onChange={(e) => updateFilters({ project: e.target.value })}
                      >
                        <option value={FILTER_ALL}>{t.filters.all}</option>
                        {projectOptions.map((p) => <option key={p} value={p}>{p}</option>)}
                        <option value={FILTER_UNOWNED}>{t.filters.unowned}</option>
                      </select>
                    </label>
                    <label className="awem-filter-field">
                      <span className="awem-filter-label">{t.filters.squad}</span>
                      <select
                        className="awem-select awem-filter-control"
                        value={filters.squad}
                        onChange={(e) => updateFilters({ squad: e.target.value })}
                      >
                        <option value={FILTER_ALL}>{t.filters.all}</option>
                        {squadOptions.map((s) => <option key={s} value={s}>{s}</option>)}
                        <option value={FILTER_UNASSIGNED_SQUAD}>{t.filters.unassignedSquad}</option>
                      </select>
                    </label>
                  </>
                ) : null}
                <label className="awem-filter-field">
                  <span className="awem-filter-label">{t.filters.local}</span>
                  <select
                    className="awem-select awem-filter-control"
                    value={filters.local}
                    onChange={(e) => updateFilters({ local: e.target.value })}
                  >
                    <option value={FILTER_ALL}>{t.filters.all}</option>
                    {localOptions.map((s) => (
                      <option key={s} value={s}>{localStateText[s] ?? s}</option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  className="awem-btn awem-btn-secondary awem-btn-compact awem-filter-reset"
                  onClick={resetFilters}
                  disabled={!filtersActive}
                >{t.filters.reset}</button>
              </div>
            ) : null}
            {/* 工具行常驻（行高锁定 32px）：无选中显示计数提示，有选中切换为批量操作条，
                避免批量条出现/消失把表格顶上顶下；平铺/树形切换无归属数据时隐藏 */}
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
                    {filtersActive ? t.filters.count(filteredCount, rowCount) : t.filters.total(rowCount)}
                  </span>
                )}
                {hasOwnership ? (
                  <div className="awem-segmented" role="group" aria-label={t.views.label}>
                    <button type="button" aria-pressed={mode === 'flat'} onClick={() => setMode('flat')}>
                      {t.views.flat}
                    </button>
                    <button type="button" aria-pressed={mode === 'tree'} onClick={() => setMode('tree')}>
                      {t.views.tree}
                    </button>
                  </div>
                ) : null}
              </div>
            ) : null}
            <div className="awem-table-wrap awem-overview-table-wrap" ref={wrapRef}>
              <table className={`awem-table${mode === 'tree' ? ' awem-table-tree' : ''}`}>
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
                  ) : mode === 'flat'
                    ? filteredRows.map((e) => (
                      <ExecutorRow
                        key={e.name}
                        e={e}
                        level={0}
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
                    : (
                      <>
                        {tree.projects.map((p) => (
                          <Fragment key={p.key}>
                            <GroupRow
                              level={1}
                              groupKey={p.key}
                              name={p.name}
                              names={p.names}
                              selected={selected}
                              locked={locked}
                              collapsed={isCollapsed(p.key)}
                              badge={t.tree.badge(p.online, p.names.length)}
                              checkboxLabel={t.tree.projectCheckbox(p.name)}
                              colSpan={columnCount}
                              onToggleCollapsed={() => toggleCollapsed(p.key)}
                              onToggleAll={toggleGroup}
                            />
                            {!isCollapsed(p.key) && p.squads.map((s) => (
                              <Fragment key={s.key}>
                                <GroupRow
                                  level={2}
                                  groupKey={s.key}
                                  project={p.name}
                                  name={s.name}
                                  names={s.names}
                                  selected={selected}
                                  locked={locked}
                                  collapsed={isCollapsed(s.key)}
                                  badge={t.tree.badge(s.online, s.names.length)}
                                  checkboxLabel={t.tree.squadCheckbox(s.name)}
                                  colSpan={columnCount}
                                  onToggleCollapsed={() => toggleCollapsed(s.key)}
                                  onToggleAll={toggleGroup}
                                />
                                {!isCollapsed(s.key) && s.leaves.map((e) => (
                                  <ExecutorRow
                                    key={`${s.key}/${e.name}`}
                                    e={e}
                                    level={3}
                                    project={p.name}
                                    squad={s.name}
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
                                ))}
                              </Fragment>
                            ))}
                          </Fragment>
                        ))}
                        {tree.unowned.leaves.length > 0 ? (
                          <Fragment key="unowned">
                            <GroupRow
                              level={1}
                              groupKey="unowned"
                              name={t.tree.unowned}
                              names={tree.unowned.names}
                              selected={selected}
                              locked={locked}
                              collapsed={isCollapsed('unowned')}
                              badge={t.tree.unownedBadge(tree.unowned.names.length)}
                              checkboxLabel={t.tree.unownedCheckbox}
                              colSpan={columnCount}
                              onToggleCollapsed={() => toggleCollapsed('unowned')}
                              onToggleAll={toggleGroup}
                            />
                            {!isCollapsed('unowned') && tree.unowned.leaves.map((e) => (
                              <ExecutorRow
                                key={`unowned/${e.name}`}
                                e={e}
                                level={2}
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
                            ))}
                          </Fragment>
                        ) : null}
                      </>
                    )}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>

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
