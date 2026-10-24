// Login.tsx：首次 token 输入（§5：居中窄卡片 360px，单输入框 + 单按钮，
// 无其他元素；F2）。提交即用该 token 调 /api/status 验证——错 token 显示
// 错误文案（发生了什么 + 怎么办），验证通过才写入 localStorage 并进入。
import { useState } from 'react'
import { t } from '../i18n'

export function LoginPage({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [tok, setTok] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const value = tok.trim()
    if (!value || busy) return
    setBusy(true)
    setError('')
    try {
      const resp = await fetch('/api/status', {
        headers: { Authorization: `Bearer ${value}` },
      })
      if (resp.ok) {
        onSubmit(value) // 验证通过才写入并进入
        return
      }
      setError(t.login.wrong)
    } catch {
      setError(t.common.networkError)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <form
        className="awem-card"
        style={{ width: 360, display: 'flex', flexDirection: 'column', gap: 12 }}
        onSubmit={submit}
      >
        <h2 className="awem-card-title" style={{ marginBottom: 0 }}>
          {t.login.title}
        </h2>
        <input
          className="awem-input mono"
          type="password"
          autoFocus
          aria-label={t.login.placeholder}
          placeholder={t.login.placeholder}
          value={tok}
          onChange={(e) => setTok(e.target.value)}
        />
        {error ? <div className="awem-error-text">{error}</div> : null}
        <button className="awem-btn awem-btn-primary" type="submit" disabled={!tok.trim() || busy}>
          {busy ? t.common.loading : t.login.submit}
        </button>
        <div className="awem-hint">{t.login.hint}</div>
      </form>
    </div>
  )
}
