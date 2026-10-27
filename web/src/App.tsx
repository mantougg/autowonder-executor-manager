// App.tsx：应用骨架（登录门 + 顶栏 + 路由分发）。页面组件逐任务点实现：
// F2 登录、F3 总览、F4 详情、F5 添加导入、F6 设置。
import { useEffect, useState } from 'react'
import { t } from './i18n'
import { parseRoute, type Route } from './router'
import { getToken, setToken, clearToken, getStatus, type StatusView } from './api'
import { StatusDot } from './components/StatusDot'
import { LoginPage } from './pages/Login'
import { OverviewPage } from './pages/Overview'
import { DetailPage } from './pages/Detail'
import { AddImportPage } from './pages/AddImport'
import { SettingsPage } from './pages/Settings'
import { navigate } from './router'

export function App() {
  const [authed, setAuthed] = useState(getToken() !== '')
  const [route, setRoute] = useState<Route>(() => parseRoute(location.pathname))
  const [status, setStatus] = useState<StatusView | null>(null)

  useEffect(() => {
    const handler = () => setRoute(parseRoute(location.pathname))
    window.addEventListener('popstate', handler)
    return () => window.removeEventListener('popstate', handler)
  }, [])

  // 顶栏 daemon 状态：进入后拉一次 + 30s 轮询（轻量；daemon 顶栏来源 T08）
  useEffect(() => {
    if (!authed) return
    let alive = true
    const tick = () => {
      getStatus(() => setAuthed(false))
        .then((s) => alive && setStatus(s))
        .catch(() => alive && setStatus(null))
    }
    tick()
    const id = setInterval(tick, 30_000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [authed])

  if (!authed) {
    return (
      <LoginPage
        onSubmit={(tok) => {
          setToken(tok)
          setAuthed(true)
        }}
      />
    )
  }

  return (
    <div className="awem-shell">
      <header className="awem-topbar">
        <a
          href="/"
          className="awem-topbar-left"
          style={{ color: 'var(--awem-fg)' }}
          onClick={(e) => {
            e.preventDefault()
            navigate('/')
          }}
        >
          <img src="/favicon.svg" alt="" width={20} height={20} aria-hidden />
          {t.app.name}
        </a>
        <div className="awem-topbar-right">
          <StatusDot
            kind={status ? 'success' : 'danger'}
            text={status ? `${t.app.topbar.daemon} ${status.version}` : t.app.topbar.daemon}
          />
          {status?.platform.configured ? (
            <StatusDot
              kind={status.platform.connected === false ? 'danger' : 'success'}
              text={t.app.topbar.platform}
            />
          ) : null}
          <button className="awem-btn awem-btn-ghost" onClick={() => navigate('/settings')}>
            {t.app.topbar.settings}
          </button>
        </div>
      </header>
      <main className="awem-main">
        {route.page === 'overview' && (
          <OverviewPage onUnauthorized={() => { clearToken(); setAuthed(false) }} />
        )}
        {route.page === 'detail' && (
          <DetailPage name={route.name} onUnauthorized={() => { clearToken(); setAuthed(false) }} />
        )}
        {route.page === 'add' && (
          <AddImportPage onUnauthorized={() => { clearToken(); setAuthed(false) }} />
        )}
        {route.page === 'settings' && <SettingsPage />}
      </main>
    </div>
  )
}
