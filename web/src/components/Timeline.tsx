// Timeline.tsx：执行器详情页运行时间带（一期）。默认 24h，基于本机
// events.log 重放结果渲染状态区间与崩溃点；崩溃点可穿透定位日志。
import { useEffect, useMemo, useState } from 'react'
import { t } from '../i18n'
import { getTimeline, type TimelineCrash, type TimelineView } from '../api'
import { fmtLocal, fmtLocalMinute } from '../time'

interface Tooltip {
  pct: number
  text: string
}

export function Timeline({
  name,
  onUnauthorized,
  onLocate,
}: {
  name: string
  onUnauthorized: () => void
  onLocate: (crash: TimelineCrash) => void
}) {
  const [data, setData] = useState<TimelineView | null>(null)
  const [error, setError] = useState('')
  const [tip, setTip] = useState<Tooltip | null>(null)

  useEffect(() => {
    let alive = true
    setData(null)
    setError('')
    const to = new Date()
    const from = new Date(to.getTime() - 24 * 3600 * 1000)
    getTimeline(name, from.toISOString(), to.toISOString(), onUnauthorized)
      .then((d) => alive && setData(d))
      .catch((e) => alive && setError((e as Error).message))
    return () => {
      alive = false
    }
  }, [name, onUnauthorized])

  const span = useMemo(() => {
    if (!data) return 0
    return new Date(data.to).getTime() - new Date(data.from).getTime()
  }, [data])

  const pct = (iso: string): number => {
    if (!data || span <= 0) return 0
    const v = ((new Date(iso).getTime() - new Date(data.from).getTime()) / span) * 100
    return Math.min(100, Math.max(0, v))
  }

  return (
    <div className="awem-card awem-timeline-card">
      <h2 className="awem-card-title">{t.timeline.title}</h2>
      {error ? (
        <span className="awem-error-text">{error || t.timeline.failed}</span>
      ) : data === null ? (
        <div className="awem-skeleton" style={{ height: 20 }} aria-busy="true" />
      ) : !data.data_start ? (
        <span className="awem-hint">{t.timeline.empty}</span>
      ) : (
        <>
          <div className="awem-timeline-bar" role="group" aria-label={t.timeline.title}>
            {data.segments.map((s, i) => {
              const left = pct(s.from)
              const right = pct(s.to)
              if (right <= left) return null
              const stateText = t.timeline.stateText[s.state] ?? s.state
              const text = t.timeline.range(stateText, fmtLocal(s.from), fmtLocal(s.to))
              return (
                <div
                  key={`seg-${i}`}
                  tabIndex={0}
                  className={`awem-timeline-seg ${s.state}`}
                  style={{ left: `${left}%`, width: `${right - left}%` }}
                  aria-label={text}
                  onMouseEnter={() => setTip({ pct: (left + right) / 2, text })}
                  onMouseLeave={() => setTip(null)}
                  onFocus={() => setTip({ pct: (left + right) / 2, text })}
                  onBlur={() => setTip(null)}
                />
              )
            })}
            {data.crashes.map((c, i) => {
              const left = pct(c.at)
              return (
                <button
                  key={`crash-${i}`}
                  type="button"
                  className="awem-timeline-crash"
                  style={{ left: `${left}%` }}
                  aria-label={t.timeline.crashAria(fmtLocal(c.at))}
                  onClick={() => onLocate(c)}
                  onMouseEnter={() =>
                    setTip({ pct: left, text: t.timeline.crash(fmtLocal(c.at)) })
                  }
                  onMouseLeave={() => setTip(null)}
                  onFocus={() => setTip({ pct: left, text: t.timeline.crash(fmtLocal(c.at)) })}
                  onBlur={() => setTip(null)}
                >
                  ✕
                </button>
              )
            })}
            {tip ? (
              <div
                role="tooltip"
                className="awem-timeline-tip"
                style={{ left: `${tip.pct}%` }}
              >
                {tip.text}
              </div>
            ) : null}
          </div>
          <div className="awem-timeline-hint">
            {t.timeline.dataStart(fmtLocalMinute(data.data_start))}
          </div>
        </>
      )}
    </div>
  )
}
