import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    // Wails v3 的 ExternalAssetHandler 代理走 IPv4 (127.0.0.1)，
    // Vite 默认可能只绑定 IPv6 回环 [::1]，导致页面加载失败
    host: '127.0.0.1',
  },
})
