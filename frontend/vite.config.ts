import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发期:npm run dev 起在 8084,/api 代理到本机 workflow 服务(8081)。
// 生产:npm run build 产物由 nginx 容器托管(见 Dockerfile / nginx.conf)。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 8084,
    proxy: {
      '/api': 'http://localhost:8081',
    },
  },
})
