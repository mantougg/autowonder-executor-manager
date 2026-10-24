// verify-ui.mjs：ui-guidelines 对照自检（T09-F7）。node scripts/verify-ui.mjs
// 检查项（退出码非 0 = 有违例）：
//   1. 动效时长均为 150/200ms ease-out；
//   2. :focus-visible 2px accent 焦点环存在、全站无 outline: none；
//   3. prefers-reduced-motion 关闭动画块存在；
//   4. 组件样式与页面源码无写死色值（白名单：#fff 按钮文字、#0d1117 日志
//      暗底、LogViewer ANSI 色表[暗底语义固定的终端色]）；
//   5. 间距只用 4/8/12/16/24/32 档（tokens.css 抽查）。
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
process.chdir(WEB_ROOT) // 任何目录调用都以 web/ 为根

const fail = []
const ok = (cond, msg) => { if (!cond) fail.push(msg) }

const read = (p) => readFileSync(join('src', p), 'utf8')
const tokens = read('styles/tokens.css')
const base = read('styles/base.css')
const comps = read('styles/components.css')

// 1. 动效
ok(/--awem-motion-fast:\s*150ms ease-out/.test(tokens), '动效 fast 应为 150ms ease-out')
ok(/--awem-motion:\s*200ms ease-out/.test(tokens), '动效应为 200ms ease-out')
for (const [f, src] of [['base.css', base], ['components.css', comps]]) {
  for (const m of src.matchAll(/transition:[^;]+;/g)) {
    const v = m[0]
    if (/var\(--awem-motion/.test(v)) continue
    if (/\d+m?s/.test(v) && !/0s/.test(v)) fail.push(`${f} 存在非令牌动效：${v}`)
  }
}

// 2. 焦点环
ok(/:focus-visible\s*{[^}]*outline:\s*2px solid var\(--awem-accent\)/.test(base), '缺 2px accent 焦点环')
for (const [f, src] of [['base.css', base], ['components.css', comps]]) {
  if (/outline:\s*none/.test(src)) fail.push(`${f} 出现 outline: none（违例）`)
}

// 3. reduced-motion
ok(/prefers-reduced-motion/.test(base), '缺 prefers-reduced-motion 处理')

// 4. 写死色值（tsx + css 白名单外）
const WHITELIST = new Set(['#fff', '#ffffff', '#0d1117'])
const ansiColors = ['f85149', '3fb950', 'd29922', '58a6ff', 'bc8cff', '39c5cf', 'e6edf3', '6a737d', '8b949e', 'ffa198', '56d364', 'e3b341', '79c0ff', 'd2a8ff', '56d4dd', 'f0f6fc']
for (const c of ansiColors) WHITELIST.add('#' + c)
const walk = (dir) => readdirSync(dir).flatMap((n) => {
  const p = join(dir, n)
  return statSync(p).isDirectory() ? walk(p) : [p]
})
for (const f of [...walk('src/styles'), ...walk('src/pages'), ...walk('src/components')]) {
  if (!f.endsWith('.css') && !f.endsWith('.tsx')) continue
  if (f.endsWith('tokens.css')) continue // 令牌定义文件：色值唯一合法落点
  const src = readFileSync(f, 'utf8')
  for (const m of src.matchAll(/#[0-9a-fA-F]{3,8}\b/g)) {
    if (!WHITELIST.has(m[0].toLowerCase())) fail.push(`${f} 写死色值 ${m[0]}（白名单外）`)
  }
}

// 5. 间距档位
for (const [name, want] of [['--awem-space-1','4px'],['--awem-space-2','8px'],['--awem-space-3','12px'],['--awem-space-4','16px'],['--awem-space-6','24px'],['--awem-space-8','32px']]) {
  ok(new RegExp(name + ':\\s*' + want).test(tokens), `间距档位 ${name} 应为 ${want}`)
}

if (fail.length) {
  console.error('❌ UI 规范自检未通过：')
  for (const f of fail) console.error('  - ' + f)
  process.exit(1)
}
console.log('✅ UI 规范自检通过（动效/焦点环/reduced-motion/色值白名单/间距档位）')
