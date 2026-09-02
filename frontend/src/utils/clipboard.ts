// Clipboard helper — 真实反映剪贴板结果（不伪造 "Copied"）。
//
// 任务 §25：UI feedback must be truthful.
// 任务 A3/A4：Copy 失败要 graceful 处理（不要 uncaught throw）。

/**
 * 把文本写入剪贴板。返回是否成功。
 * 失败时不抛异常，调用方根据返回值决定 UI。
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 走 fallback
  }
  // Fallback：textarea + execCommand('copy')
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    ta.style.pointerEvents = 'none'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
