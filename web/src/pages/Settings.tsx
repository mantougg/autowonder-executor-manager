// Settings.tsx：设置页（F6）。分组表单：平台联动（base_url/MCP 令牌掩码+
// 眼睛/轮询间隔）、日志（max_size_mb/retention_days）、外观（主题三态）。
// 保存 PUT /api/settings；平台从无到有提示将开始联动。
import { useEffect, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { getSettings, putSettings } from '../api'
import { ToastProvider, useToast } from '../components/Toast'
import { getStoredTheme, storeTheme, applyTheme, type ThemeChoice } from '../theme'

function SettingsInner() {
  const toast = useToast()
  const [platformOn, setPlatformOn] = useState(false)
  const [baseUrl, setBaseUrl] = useState('')
  const [mcpToken, setMcpToken] = useState('')
  const [pollInterval, setPollInterval] = useState('60s')
  const [maxSize, setMaxSize] = useState(50)
  const [retention, setRetention] = useState(7)
  const [showToken, setShowToken] = useState(false)
  const [theme, setTheme] = useState<ThemeChoice>(() => getStoredTheme())
  const [saving, setSaving] = useState(false)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    getSettings()
      .then((s) => {
        if (s.platform) {
          setPlatformOn(true)
          setBaseUrl(s.platform.base_url)
          setMcpToken(s.platform.mcp_token) // 掩码；原样保存=不变
          setPollInterval(s.platform.poll_interval)
        }
        setMaxSize(s.log.max_size_mb)
        setRetention(s.log.retention_days)
        setLoaded(true)
      })
      .catch((e) => toast((e as Error).message, 'error'))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    setSaving(true)
    try {
      await putSettings({
        platform: platformOn
          ? { base_url: baseUrl, mcp_token: mcpToken, poll_interval: pollInterval }
          : null,
        log: { max_size_mb: maxSize, retention_days: retention },
      })
      toast('设置已保存' + (platformOn && baseUrl ? '（平台联动将开始/更新轮询）' : ''))
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  const switchTheme = (c: ThemeChoice) => {
    setTheme(c)
    storeTheme(c)
    applyTheme(c)
  }

  if (!loaded) {
    return (
      <div className="awem-page" aria-busy="true">
        <div className="awem-skeleton" /><div className="awem-skeleton" />
      </div>
    )
  }

  return (
    <div className="awem-page">
      <div className="awem-page-header"><h1 className="awem-page-title">设置</h1></div>

      <div className="awem-card">
        <h2 className="awem-card-title">平台联动（只读状态展示）</h2>
        <div className="awem-field" style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <button className="awem-switch" role="switch" aria-checked={platformOn} aria-label="启用平台联动" onClick={() => setPlatformOn((v) => !v)} />
          <label>启用（配置 [platform] 段）</label>
        </div>
        {platformOn ? (
          <>
            <div className="awem-field">
              <label>平台地址（base_url）</label>
              <input className="awem-input mono" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="http://10.x.x.x:7001" />
            </div>
            <div className="awem-field">
              <label>个人 MCP 令牌（掩码显示；保持掩码原样=不修改）</label>
              <div className="awem-secret">
                <input className="awem-input mono" type={showToken ? 'text' : 'password'} value={mcpToken} onChange={(e) => setMcpToken(e.target.value)} />
                <button className="awem-eye" aria-label={showToken ? '隐藏令牌' : '显示令牌'} onClick={() => setShowToken((v) => !v)}>
                  {showToken ? <EyeOff size={16} aria-hidden /> : <Eye size={16} aria-hidden />}
                </button>
              </div>
            </div>
            <div className="awem-field" style={{ maxWidth: 220 }}>
              <label>轮询间隔</label>
              <input className="awem-input mono" value={pollInterval} onChange={(e) => setPollInterval(e.target.value)} placeholder="60s" />
            </div>
          </>
        ) : (
          <div className="awem-hint">关闭后仅本地管理（平台状态列显示 —）</div>
        )}
      </div>

      <div className="awem-card">
        <h2 className="awem-card-title">日志</h2>
        <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
          <div className="awem-field" style={{ maxWidth: 220 }}>
            <label>单文件滚动阈值（MB）</label>
            <input className="awem-input mono" type="number" min={1} value={maxSize} onChange={(e) => setMaxSize(Number(e.target.value) || 1)} />
          </div>
          <div className="awem-field" style={{ maxWidth: 220 }}>
            <label>保留天数</label>
            <input className="awem-input mono" type="number" min={1} value={retention} onChange={(e) => setRetention(Number(e.target.value) || 1)} />
          </div>
        </div>
      </div>

      <div className="awem-card">
        <h2 className="awem-card-title">外观</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          {([['system', '跟随系统'], ['light', '浅色'], ['dark', '深色']] as [ThemeChoice, string][]).map(([v, label]) => (
            <button
              key={v}
              className={`awem-btn awem-btn-compact ${theme === v ? 'awem-btn-primary' : 'awem-btn-secondary'}`}
              aria-pressed={theme === v}
              onClick={() => switchTheme(v)}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <div>
        <button className="awem-btn awem-btn-primary" onClick={save} disabled={saving}>
          {saving ? '保存中…' : '保存设置'}
        </button>
      </div>
    </div>
  )
}

export function SettingsPage() {
  return (
    <ToastProvider>
      <SettingsInner />
    </ToastProvider>
  )
}
