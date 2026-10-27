// Select.tsx：自绘下拉选择（§4「下拉选择」）：触发按钮同输入框外观 + 卡片弹层
// （原生 <select> 的弹框样式不可控，全站统一用本组件替代）。
// 键盘：Enter/Space/↑/↓ 打开；打开后 ↑/↓ 移动高亮、Enter 选中、Esc/Tab 关闭；
// 点击组件外部关闭；焦点始终留在触发按钮上。
import { useEffect, useRef, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'

export interface SelectOption {
  value: string
  label: string
}

export function Select({
  value,
  options,
  onChange,
  ariaLabel,
  className,
}: {
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
  ariaLabel: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1) // 键盘/悬浮高亮项
  const rootRef = useRef<HTMLDivElement>(null)
  const current = options.find((o) => o.value === value)

  useEffect(() => {
    if (!open) return
    const onDocDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDocDown)
    return () => document.removeEventListener('mousedown', onDocDown)
  }, [open])

  // 高亮项移出可视区时滚动跟随
  useEffect(() => {
    if (!open || active < 0) return
    rootRef.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [open, active])

  const openList = () => {
    setActive(Math.max(0, options.findIndex((o) => o.value === value)))
    setOpen(true)
  }

  const choose = (v: string) => {
    onChange(v)
    setOpen(false)
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        openList()
      }
      return
    }
    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        setOpen(false)
        break
      case 'ArrowDown':
        e.preventDefault()
        setActive((a) => Math.min(options.length - 1, a + 1))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActive((a) => Math.max(0, a - 1))
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        if (options[active]) choose(options[active].value)
        break
      case 'Tab':
        setOpen(false)
        break
    }
  }

  return (
    <div className={`awem-select${className ? ` ${className}` : ''}`} ref={rootRef} onKeyDown={onKeyDown}>
      <button
        type="button"
        className="awem-select-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        onClick={() => (open ? setOpen(false) : openList())}
      >
        <span className="awem-select-value">{current?.label ?? value}</span>
        <ChevronDown size={14} aria-hidden className="awem-select-caret" />
      </button>
      {open ? (
        <ul className="awem-select-pop" role="listbox" aria-label={ariaLabel}>
          {options.map((o, i) => (
            <li
              key={o.value}
              role="option"
              aria-selected={o.value === value}
              data-index={i}
              data-active={i === active || undefined}
              className="awem-select-option"
              onMouseEnter={() => setActive(i)}
              onClick={() => choose(o.value)}
            >
              <span className="awem-select-check" aria-hidden>
                {o.value === value ? <Check size={14} /> : null}
              </span>
              <span className="awem-select-option-label">{o.label}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
