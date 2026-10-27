// NavTree.tsx：总览左侧归属导航树（ui-guidelines §4「导航树」；仅导航过滤，不含复选框）。
import { ChevronDown, ChevronRight } from 'lucide-react'
import { t } from '../i18n'
import type { TreeModel, TreeSel } from '../overview-model'

export function NavTree({
  model, sel, collapsed, onSelect, onToggle,
}: {
  model: TreeModel
  sel: TreeSel
  collapsed: Set<string>
  onSelect: (s: TreeSel) => void
  onToggle: (key: string) => void
}) {
  return (
    <nav className="awem-navtree" aria-label={t.navTree.label}>
      <ul>
        <li>
          <button
            type="button"
            className="awem-navtree-node"
            aria-current={sel.kind === 'all' ? 'true' : undefined}
            onClick={() => onSelect({ kind: 'all' })}
          >
            <span className="awem-navtree-spacer" aria-hidden />
            <span className="awem-navtree-name" title={t.navTree.all}>{t.navTree.all}</span>
            <span className="awem-tree-badge">{t.tree.badge(model.allOnline, model.allCount)}</span>
          </button>
        </li>
        {model.projects.map((p) => {
          const isCollapsed = collapsed.has(p.key)
          return (
            <li key={p.key}>
              <div className="awem-navtree-row">
                <button
                  type="button"
                  className="awem-navtree-chevron"
                  aria-expanded={!isCollapsed}
                  aria-label={isCollapsed ? t.tree.expand(p.name) : t.tree.collapse(p.name)}
                  onClick={() => onToggle(p.key)}
                >
                  {isCollapsed ? <ChevronRight size={14} aria-hidden /> : <ChevronDown size={14} aria-hidden />}
                </button>
                <button
                  type="button"
                  className="awem-navtree-node"
                  data-project={p.name}
                  aria-current={sel.kind === 'project' && sel.project === p.name ? 'true' : undefined}
                  onClick={() => onSelect({ kind: 'project', project: p.name })}
                >
                  <span className="awem-navtree-name" title={p.name}>{p.name}</span>
                  <span className="awem-tree-badge">{t.tree.badge(p.online, p.names.length)}</span>
                </button>
              </div>
              {!isCollapsed ? (
                <ul>
                  {p.squads.map((s) => (
                    <li key={s.key}>
                      <button
                        type="button"
                        className="awem-navtree-node"
                        data-project={p.name}
                        data-squad={s.name}
                        aria-current={
                          sel.kind === 'squad' && sel.project === p.name && sel.squad === s.name
                            ? 'true'
                            : undefined
                        }
                        onClick={() => onSelect({ kind: 'squad', project: p.name, squad: s.name })}
                      >
                        <span className="awem-navtree-name" title={s.name}>{s.name}</span>
                        <span className="awem-tree-badge">{t.tree.badge(s.online, s.names.length)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          )
        })}
        {model.unowned.leaves.length > 0 ? (
          <li>
            <button
              type="button"
              className="awem-navtree-node"
              data-unowned="true"
              aria-current={sel.kind === 'unowned' ? 'true' : undefined}
              onClick={() => onSelect({ kind: 'unowned' })}
            >
              <span className="awem-navtree-spacer" aria-hidden />
              <span className="awem-navtree-name" title={t.tree.unowned}>{t.tree.unowned}</span>
              <span className="awem-tree-badge">{t.tree.unownedBadge(model.unowned.names.length)}</span>
            </button>
          </li>
        ) : null}
      </ul>
    </nav>
  )
}
