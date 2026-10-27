# web/ AGENTS.md — 前端工作约定

awem 的 Web 页是 React SPA（Vite + TypeScript + React 18），构建产物
`web/dist/` 经 `go:embed` 内嵌进 awem 二进制。仓库级约定见根
[AGENTS.md](../AGENTS.md)；代码地图见 [docs/dev-guide.md](../docs/dev-guide.md)。

## 结构速查

| 位置 | 职责 |
|---|---|
| `src/api.ts` | 统一 API 客户端：token 头注入、401 跳登录、`APIError`；与后端 `internal/api/schema.go` 同构 |
| `src/App.tsx` / `router.ts` | 应用外壳与 hash 路由 |
| `src/pages/` | Overview（总览左树右表：导航树+执行器表）/ Detail（详情+实时日志）/ AddImport / Settings / Login |
| `src/components/` | LogViewer（ANSI 渲染+跟随滚动）/ StatusDot / Dialog / Toast |
| `src/i18n.ts` | 文案集中管理（v1 全中文；新文案加在这里，不写死在组件里） |
| `src/styles/tokens.css` | `--awem-*` 设计令牌（颜色/间距/字号/圆角） |
| `public/favicon.svg` | 项目图标（浏览器页签 + 顶栏 logo + README） |

## 规则

1. **视觉/交互以 [docs/design/ui-guidelines.md](../docs/design/ui-guidelines.md) 为准**：
   新增状态语义、新组件、新设计令牌，先更新规范再写代码。
2. **颜色只用 `--awem-*` CSS 变量**（tokens.css），组件与样式里不写死色值；
   状态色语义（成功/警告/危险/离线）按规范取用。
3. **API 变更前后端同改**：后端视图在 `internal/api/schema.go`，前端在
   `src/api.ts`；schema 字段只增不改（向后兼容）。
4. **`web/dist/` 是构建产物、仓库忽略**：不要手改、不要提交；fresh clone 由
   `make build` 从 `web/placeholder/` 复制占位页。
5. **提交前验证**：`npm run build`（含 `tsc --noEmit` 类型门，必须通过）；
   UI 规范自检 `node scripts/verify-ui.mjs`；端到端 `npx playwright test`。

## 常用命令

```bash
npm install          # 首次装依赖
npm run dev          # 本地开发（Vite dev server）
npm run build        # 类型检查 + 产物到 web/dist/
```
