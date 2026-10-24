// router.ts：极简前端路由（history API；页面仅一层，不引路由库以控体积）。
// 约定路由：/ 总览、/executors/:name 详情、/add 添加导入、/settings 设置。

export type Route =
  | { page: 'overview' }
  | { page: 'detail'; name: string }
  | { page: 'add' }
  | { page: 'settings' }

export function parseRoute(path: string): Route {
  const p = path.replace(/\/+$/, '') || '/'
  if (p === '/') return { page: 'overview' }
  if (p === '/add') return { page: 'add' }
  if (p === '/settings') return { page: 'settings' }
  const m = p.match(/^\/executors\/([^/]+)$/)
  if (m) return { page: 'detail', name: decodeURIComponent(m[1]) }
  return { page: 'overview' }
}

export function navigate(path: string) {
  history.pushState(null, '', path)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

export function useRoute(subscribe: (r: Route) => void): () => Route {
  const current = () => parseRoute(location.pathname)
  const handler = () => subscribe(current())
  window.addEventListener('popstate', handler)
  return current
}
