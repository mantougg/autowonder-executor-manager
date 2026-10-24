// Toast.tsx：右下角通知（§4：3s 自动消失；成功中性 / 失败 danger）。
import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

export interface ToastItem {
  id: number
  message: string
  kind: 'info' | 'error'
}

const ToastCtx = createContext<(message: string, kind?: 'info' | 'error') => void>(() => {})

export function useToast() {
  return useContext(ToastCtx)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const seq = useRef(0)
  const push = useCallback((message: string, kind: 'info' | 'error' = 'info') => {
    const id = ++seq.current
    setItems((cur) => [...cur, { id, message, kind }])
    setTimeout(() => {
      setItems((cur) => cur.filter((x) => x.id !== id))
    }, 3000)
  }, [])
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="awem-toast-wrap" role="status" aria-live="polite">
        {items.map((x) => (
          <div key={x.id} className={`awem-toast ${x.kind === 'error' ? 'error' : ''}`}>
            {x.message}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}
