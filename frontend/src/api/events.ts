// Wails 运行时事件封装
//
// Wails v2 在页面中注入 window.runtime 对象（dev 和生产模式均可用），
// CLI 生成的 wailsjs/runtime 包装文件在部分版本中不稳定，
// 这里直接基于 window.runtime 做轻量封装。

/* eslint-disable @typescript-eslint/no-explicit-any */

interface WailsRuntime {
  EventsOnMultiple(eventName: string, callback: (...data: any[]) => void, maxCallbacks: number): void
  EventsOff(eventName: string, ...additionalEventNames: string[]): void
  EventsEmit(eventName: string, ...data: any[]): void
}

function rt(): WailsRuntime {
  const runtime = (window as any).runtime
  if (!runtime) {
    throw new Error('Wails runtime not injected yet')
  }
  return runtime as WailsRuntime
}

/** 监听后端事件（持续） */
export function EventsOn(eventName: string, callback: (...data: any[]) => void): void {
  rt().EventsOnMultiple(eventName, callback, -1)
}

/** 取消监听 */
export function EventsOff(...eventNames: string[]): void {
  rt().EventsOff(...eventNames)
}
