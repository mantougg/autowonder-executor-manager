// Login.tsx：首次 token 输入（§5：居中窄卡片 360px，单输入框 + 单按钮）。
// F2 完整实现（错误文案/回车提交/成功回调）。
import { useState } from 'react'
import { t } from '../i18n'

export function LoginPage({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [tok, setTok] = useState('')
  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <form
        className="awem-card"
        style={{ width: 360, display: 'flex', flexDirection: 'column', gap: 12 }}
        onSubmit={(e) => {
          e.preventDefault()
          if (tok.trim()) onSubmit(tok.trim())
        }}
      >
        <h2 className="awem-card-title" style={{ marginBottom: 0 }}>{t.login.title}</h2>
        <input
          className="awem-input mono"
          type="password"
          autoFocus
          placeholder={t.login.placeholder}
          value={tok}
          onChange={(e) => setTok(e.target.value)}
        />
        <button className="awem-btn awem-btn-primary" type="submit" disabled={!tok.trim()}>
          {t.login.submit}
        </button>
      </form>
    </div>
  )
}
