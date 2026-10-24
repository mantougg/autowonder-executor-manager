// Dialog.tsx：破坏性确认 / 编辑对话框（§4：Esc 可关；确认按钮写具体动作）。
import { useEffect, useRef, type ReactNode } from 'react'

export function Dialog({
  title,
  body,
  confirmText,
  danger = false,
  onConfirm,
  onCancel,
}: {
  title: string
  body: ReactNode
  confirmText: string // 具体动作文案（如"停止全部执行器"）
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel()
    }
    window.addEventListener('keydown', onKey)
    // 焦点管理：打开即聚焦到对话框内
    ref.current?.focus()
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])
  return (
    <div
      className="awem-dialog-mask"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="awem-dialog" role="dialog" aria-modal="true" aria-label={title} ref={ref} tabIndex={-1}>
        <h3 className="awem-dialog-title">{title}</h3>
        <div className="awem-dialog-body">{body}</div>
        <div className="awem-dialog-actions">
          <button className="awem-btn awem-btn-secondary" onClick={onCancel}>
            取消
          </button>
          <button
            className={`awem-btn ${danger ? 'awem-btn-danger' : 'awem-btn-primary'}`}
            onClick={onConfirm}
          >
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  )
}
