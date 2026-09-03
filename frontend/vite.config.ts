import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vitejs.dev/config/
//
// Phase 8E/8F：多页面方案。Main / QuickInput / Overlay 是三个明确的前端入口，
// 物理上独立 webview。
//   - index.html → Main Window（Sidebar + Chat/Settings/Logs/OverlayView）
//   - quickinput.html → native QuickInput 窗口（Frameless + AlwaysOnTop）
//   - overlay.html → native Overlay 窗口（Frameless + AlwaysOnTop + Transparent）
// 各入口的 bundle 互不依赖，仅在其目标 webview 渲染。
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
        // Overlay native window entry (Phase 8F)
        overlay: resolve(__dirname, 'overlay.html'),
      },
    },
  },
})
