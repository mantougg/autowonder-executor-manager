// overview-model.ts：总览页纯逻辑（2026-10-27 由 pages/Overview.tsx 拆出）。
// 含归属导航树模型构建、树/筛选匹配、批量结果辅助与折叠态会话持久化；不依赖 React。
import { APIError, type ExecutorView } from './api'
import { t } from './i18n'

export const TREE_KEY = 'awem-overview-tree-collapsed'

export const RUNNING_LIKE = new Set(['running', 'backoff', 'stopping'])
export const isRunningLike = (state: string) => RUNNING_LIKE.has(state)

export type BatchKind = 'start' | 'stop' | 'delete'

// 筛选哨兵值：与真实状态名隔离；选项标签走 i18n。
export const FILTER_ALL = '__all__'
export const LOCAL_STATE_ORDER = ['running', 'backoff', 'stopping', 'stopped', 'failed']

// 左树选中节点：全部 / 未归属 / 项目 / 小队。
export type TreeSel =
  | { kind: 'all' }
  | { kind: 'unowned' }
  | { kind: 'project'; project: string }
  | { kind: 'squad'; project: string; squad: string }

export interface FilterState {
  name: string
  local: string
}

export const EMPTY_FILTERS: FilterState = { name: '', local: FILTER_ALL }

export function filterActive(f: FilterState): boolean {
  return f.name.trim() !== '' || f.local !== FILTER_ALL
}

export interface BatchTarget {
  name: string
  label: string
}

export interface BatchResult {
  name: string
  ok: boolean
  reason?: string
}

export interface SquadGroup {
  key: string
  name: string
  leaves: ExecutorView[]
  names: string[]
  online: number
}

export interface ProjectGroup {
  key: string
  name: string
  squads: SquadGroup[]
  names: string[]
  online: number
}

export interface TreeModel {
  projects: ProjectGroup[]
  unowned: SquadGroup
  allCount: number
  allOnline: number
}

// 名称列表：超过 limit 时列前 limit 个并注明总数（工单：等 N 个）。
export function joinNames(names: string[], limit: number): string {
  if (names.length <= limit) return names.join('、')
  return `${names.slice(0, limit).join('、')} 等 ${names.length} 个`
}

export function failureReason(err: unknown): string {
  if (err instanceof APIError) {
    if (err.httpStatus === 404) return t.batch.reason.notFound
    if (err.httpStatus === 409) return t.batch.reason.running
    return err.message || t.batch.reason.unknown
  }
  return (err as Error).message || t.batch.reason.unknown
}

export function loadCollapsed(): Set<string> {
  try {
    const raw = sessionStorage.getItem(TREE_KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as unknown
    return new Set(Array.isArray(arr) ? arr.filter((x): x is string => typeof x === 'string') : [])
  } catch {
    return new Set()
  }
}

export function saveCollapsed(s: Set<string>) {
  try {
    sessionStorage.setItem(TREE_KEY, JSON.stringify([...s]))
  } catch {
    /* 存储不可用时仅会话内失效 */
  }
}

export function isOnline(e: ExecutorView): boolean {
  return e.platform?.status === 'ONLINE' || e.platform?.status === 'BUSY'
}

export function uniqueNames(leaves: ExecutorView[]): string[] {
  return [...new Set(leaves.map((e) => e.name))]
}

export function onlineOf(leaves: ExecutorView[]): number {
  const seen = new Map<string, boolean>()
  for (const e of leaves) {
    if (!seen.has(e.name)) seen.set(e.name, isOnline(e))
  }
  let n = 0
  for (const v of seen.values()) if (v) n++
  return n
}

export function squadNamesOf(e: ExecutorView): string[] {
  const arr = e.platform?.squad_names
  if (Array.isArray(arr) && arr.length > 0) return arr
  const joined = e.platform?.squad
  return joined ? joined.split('、').filter(Boolean) : []
}

// 表格「所属小队」单元格主行文本：顿号拼接串（squad_names 兜底拼接）。
export function squadTextOf(e: ExecutorView): string {
  return e.platform?.squad || squadNamesOf(e).join('、')
}

// 树节点匹配用：无小队执行器归入虚拟「未分配小队」节点。
export function squadsForNode(e: ExecutorView): string[] {
  const s = squadNamesOf(e)
  return s.length > 0 ? s : [t.tree.unassignedSquad]
}

export function inTreeSel(e: ExecutorView, sel: TreeSel): boolean {
  if (sel.kind === 'all') return true
  const project = e.platform?.project ?? ''
  if (sel.kind === 'unowned') return project === ''
  if (sel.kind === 'project') return project === sel.project
  return project === sel.project && squadsForNode(e).includes(sel.squad)
}

// 名称/本地进程 AND；名称匹配平台显示名或执行器标识（去空格、不区分大小写子串）。
export function matchesFilters(e: ExecutorView, f: FilterState): boolean {
  const q = f.name.trim().toLocaleLowerCase()
  if (q) {
    const names = [e.platform?.name ?? '', e.name]
    if (!names.some((n) => n.toLocaleLowerCase().includes(q))) return false
  }
  if (f.local !== FILTER_ALL && e.local.state !== f.local) return false
  return true
}

const collator = new Intl.Collator('zh-CN')

export function buildTree(rows: ExecutorView[]): TreeModel {
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
  return {
    projects: projectGroups,
    unowned: {
      key: 'unowned',
      name: t.tree.unowned,
      leaves: unownedLeaves,
      names: uniqueNames(unownedLeaves),
      online: 0,
    },
    allCount: uniqueNames(rows).length,
    allOnline: onlineOf(rows),
  }
}
