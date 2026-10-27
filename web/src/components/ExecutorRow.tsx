// ExecutorRow.tsx：总览表格单行（名称双行、所属小队双行、双状态、行内操作）。
import { Play, RotateCw, Square, Trash2 } from 'lucide-react'
import { t } from '../i18n'
import type { ExecutorView } from '../api'
import { StatusDot, localStateKind, localStateText, platformKind, platformText } from './StatusDot'
import { navigate } from '../router'
import { squadTextOf, type BatchKind } from '../overview-model'

export function ExecutorRow({
  e, linked, selected, locked, busyName, batchBusy,
  onToggle, onAct, onStop, onRemove,
}: {
  e: ExecutorView
  linked: boolean
  selected: boolean
  locked: boolean
  busyName: string
  batchBusy: BatchKind | null
  onToggle: (name: string) => void
  onAct: (name: string, action: 'start' | 'stop' | 'restart') => void
  onStop: (e: ExecutorView) => void
  onRemove: (e: ExecutorView) => void
}) {
  const squadText = squadTextOf(e)
  const project = e.platform?.project ?? ''
  const version = e.parsed?.runtime_version ?? ''
  return (
    <tr>
      <td className="awem-col-check">
        <input
          type="checkbox"
          className="awem-checkbox"
          aria-label={t.batch.selectRow(e.name)}
          checked={selected}
          disabled={locked}
          onChange={() => onToggle(e.name)}
        />
      </td>
      <td>
        <div className="awem-name-line">
          <a
            href={`/executors/${encodeURIComponent(e.name)}`}
            className={e.platform?.name ? '' : 'mono'}
            title={e.platform?.agent_name ? `数字工人：${e.platform.agent_name}` : undefined}
            onClick={(ev) => { ev.preventDefault(); navigate(`/executors/${encodeURIComponent(e.name)}`) }}
          >
            {e.platform?.name || e.name}
          </a>
          {e.drift && e.drift.fields.length > 0 ? (
            <span className="awem-pill warn" title={e.drift.message}>
              {t.status.drift} {e.drift.fields.length}
            </span>
          ) : null}
        </div>
        {e.platform?.name ? (
          <div className="awem-hint mono awem-cell-truncate" style={{ fontSize: 12 }} title={e.name}>{e.name}</div>
        ) : null}
      </td>
      {linked ? (
        <td>
          {squadText || project ? (
            <>
              <div className="awem-cell-truncate" title={squadText || undefined}>{squadText || '—'}</div>
              {project ? (
                <div className="awem-hint awem-cell-truncate" style={{ fontSize: 12 }} title={project}>{project}</div>
              ) : null}
            </>
          ) : (
            <span className="awem-hint">—</span>
          )}
        </td>
      ) : null}
      <td className="mono" title={version ? `autowonder@${version}` : undefined}>
        {version || '—'}
      </td>
      <td><StatusDot kind={localStateKind(e.local.state)} text={localStateText[e.local.state] ?? e.local.state} /></td>
      <td className="awem-hint">{e.local.conn === 'connected' ? t.status.connected : e.local.conn === 'reconnecting' ? t.status.reconnecting : e.local.conn}</td>
      <td>
        {e.platform ? (
          <StatusDot kind={platformKind(e.platform.status)} text={platformText[e.platform.status] ?? e.platform.status} />
        ) : (
          <span className="awem-hint">—</span>
        )}
      </td>
      <td className="mono">{e.parsed?.model || '—'}</td>
      <td>{e.autostart ? '✓' : '—'}</td>
      <td>{e.local.restarts}</td>
      <td>
        <span className="awem-row-actions">
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.start} aria-label={`${t.actions.start} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onAct(e.name, 'start')}
          ><Play size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.stop} aria-label={`${t.actions.stop} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onStop(e)}
          ><Square size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.restart} aria-label={`${t.actions.restart} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onAct(e.name, 'restart')}
          ><RotateCw size={15} aria-hidden /></button>
          <button
            className="awem-btn awem-btn-ghost" title={t.actions.remove} aria-label={`${t.actions.remove} ${e.name}`}
            disabled={busyName === e.name || batchBusy !== null}
            onClick={() => onRemove(e)}
          ><Trash2 size={15} aria-hidden /></button>
        </span>
      </td>
    </tr>
  )
}
