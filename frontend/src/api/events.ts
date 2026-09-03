// Wails 运行时事件封装
//
// Phase 8: Wails v2 → v3 迁移。
// v2 时代依赖页面注入的 window.runtime 对象；v3 改为 npm 包
// `@wailsio/runtime`（生成 bindings 也从它 import Call 等）。
// 这里保持对外契约不变：EventsOn(name, callback) 的 callback 仍然
// 直接收到 payload（v3 的 WailsEvent.data），所有组件无需感知迁移。
//
// v3 语义映射：
//   - Go `Emit(name, data)` 单参数 → ev.data 即 payload 本身
//     （多参数时 ev.data 是数组，Aurora 全部使用单 payload，见 main.go / bindings.go）
//   - v2 EventsOnMultiple(name, cb, -1) ≡ v3 Events.On(name, cb)

import { Events } from '@wailsio/runtime'

/** 监听后端事件（持续）。callback 直接收 payload（v3 WailsEvent.data）。 */
export function EventsOn(eventName: string, callback: (...data: any[]) => void): void {
  Events.On(eventName, (ev: any) => {
    callback(ev.data)
  })
}

/** 取消监听 */
export function EventsOff(...eventNames: string[]): void {
  for (const name of eventNames) {
    Events.Off(name)
  }
}
