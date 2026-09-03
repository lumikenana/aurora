import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vitejs.dev/config/
//
// Phase 8E：多页面方案。Main（index.html）和 QuickInput（quickinput.html）
// 是两个明确的前端入口，物理上独立 webview。
//   - index.html → Main Window（Sidebar + Chat/Settings/Logs/Overlay）
//   - quickinput.html → native QuickInput 窗口（Frameless + AlwaysOnTop）
// 两个入口的 bundle 互不依赖，QuickInput 仅在独立 webview 渲染。
export default defineConfig({
  plugins: [vue()],
  server: {
    // Wails v3 的 ExternalAssetHandler 代理走 IPv4 (127.0.0.1)，
    // Vite 默认可能只绑定 IPv6 回环 [::1]，导致页面加载失败
    host: '127.0.0.1',
  },
  build: {
    rollupOptions: {
      input: {
        // Main entry
        index: resolve(__dirname, 'index.html'),
        // QuickInput native window entry (Phase 8E)
        quickinput: resolve(__dirname, 'quickinput.html'),
      },
    },
  },
})
