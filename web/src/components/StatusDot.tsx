// StatusDot.tsx：状态徽标（§2.2 圆点+文字双编码）。
import type { ReactNode } from 'react'

export type StatusKind = 'success' | 'busy' | 'warn' | 'danger' | 'off'

export function StatusDot({ kind, text }: { kind: StatusKind; text: ReactNode }) {
  return (
    <span className={`awem-status ${kind}`}>
      <span className="awem-status-dot" aria-hidden="true" />
      <span>{text}</span>
    </span>
  )
}

// 本地进程态 → 徽标色（§2.2：运行中 success；退避/重连 warn；失败 danger；
// 停止 off；停止中 warn）。
export function localStateKind(state: string): StatusKind {
  switch (state) {
    case 'running':
      return 'success'
    case 'backoff':
    case 'stopping':
      return 'warn'
    case 'failed':
      return 'danger'
    default:
      return 'off'
  }
}

export const localStateText: Record<string, string> = {
  running: '运行中',
  stopped: '已停止',
  backoff: '重连中',
  stopping: '停止中',
  failed: '失败',
}

// 平台态 → 徽标色（ONLINE success；BUSY busy；OFFLINE off；不可达/未归属 warn）。
export function platformKind(status: string): StatusKind {
  switch (status) {
    case 'ONLINE':
      return 'success'
    case 'BUSY':
      return 'busy'
    case 'OFFLINE':
      return 'off'
    case 'unreachable':
      return 'danger'
    default:
      return 'warn'
  }
}

export const platformText: Record<string, string> = {
  ONLINE: 'ONLINE',
  BUSY: 'BUSY',
  OFFLINE: 'OFFLINE',
  unreachable: '不可达',
  unowned: '未归属',
}
