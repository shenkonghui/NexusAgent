import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vite 配置：开发代理将 /api 请求转发到 Go 后端
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
