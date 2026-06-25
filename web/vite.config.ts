import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vite 配置：开发代理将 /api 请求转发到 Go 后端
export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        ws: true, // 启用 WebSocket 代理（终端需要）
      },
    },
  },
})
