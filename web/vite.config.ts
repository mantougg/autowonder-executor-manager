// Vite 配置：生产构建产物 web/dist/（go:embed 消费）；开发代理到本机
// daemon（127.0.0.1:18765 为 dev 默认端口约定，见 README）。
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    target: 'es2020',
    // 轻量快速原则（ui-guidelines §1.5）：关闭 sourcemap、内联小资源
    sourcemap: false,
    assetsInlineLimit: 8192,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:18765',
      '/ws': { target: 'ws://127.0.0.1:18765', ws: true },
    },
  },
})
