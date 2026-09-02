// Markdown 渲染器 — Aurora 内部最小实现
//
// 设计目标（Phase 6 A2）：
//   1. 支持基本 Markdown：段落、标题、列表、强调、行内代码、代码块、链接
//   2. 安全性：所有用户输入都经过 escapeHtml；不解释原始 HTML；输出白名单标签
//   3. 零依赖：避免引入 marked / markdown-it 带来的 ~30-50KB 体积
//   4. 流式友好：partial markdown（未关闭的代码块）能合理降级展示
//
// 不支持（任务范围外，避免误用）：
//   - 表格（LLM 偶尔会输出，但桌面工具场景用得少）
//   - 任务列表
//   - 删除线
//   - HTML 内嵌（任务明确禁止）
//   - 图片（聊天场景不需要）
//
// 输出：HTML 字符串。
// 调用方应该用 v-html + 我方产出的 HTML 字符串（因为已经转义过所有用户内容）。

// ---- 转义 ----
const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => HTML_ESCAPES[c] || c)
}

// ---- 行内解析（顺序很重要：先 code 再 em 再 strong 再 link） ----

/**
 * 处理行内 markdown。
 * 任何 HTML 字符已被转义，所以输出可以安全 v-html。
 */
function renderInline(text: string): string {
  let s = escapeHtml(text)

  // 行内代码：先处理（避免内部被 em/strong 进一步解析）
  s = s.replace(/`([^`\n]+)`/g, (_m, code) => `<code class="md-inline-code">${code}</code>`)

  // 链接：[text](url) — 仅允许 http/https/mailto
  s = s.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+|mailto:[^\s)]+)\)/g, (_m, label, url) => {
    return `<a class="md-link" href="${url}" target="_blank" rel="noopener noreferrer">${label}</a>`
  })

  // Strong：**text** 或 __text__
  s = s.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
  s = s.replace(/__([^_\n]+)__/g, '<strong>$1</strong>')

  // Em：*text* 或 _text_（避免吃掉单词内的 _ 强调：要求两侧有非单词字符或边界）
  s = s.replace(/(^|[^\w*])\*([^*\n]+)\*(?=[^\w*]|$)/g, '$1<em>$2</em>')
  s = s.replace(/(^|[^\w_])_([^_\n]+)_(?=[^\w_]|$)/g, '$1<em>$2</em>')

  return s
}

// ---- 块级解析 ----

interface BlockState {
  inCodeBlock: boolean
  codeLang: string
  codeLines: string[]
  listType: 'ul' | 'ol' | null
  listItems: string[]
  out: string[]
}

function flushList(state: BlockState) {
  if (state.listType && state.listItems.length > 0) {
    const tag = state.listType
    const items = state.listItems.map((it) => `<li>${renderInline(it)}</li>`).join('')
    state.out.push(`<${tag} class="md-list">${items}</${tag}>`)
  } else if (state.listType && state.listItems.length === 0) {
    // 空列表不输出
  }
  state.listType = null
  state.listItems = []
}

function flushCodeBlock(state: BlockState) {
  if (state.inCodeBlock) {
    // 未关闭的代码块（流式中常见）：降级为已收集的行
    const langClass = state.codeLang ? ` class="md-code-lang-${escapeHtml(state.codeLang)}"` : ''
    const body = state.codeLines.map(escapeHtml).join('\n')
    state.out.push(
      `<pre class="md-code-block"${langClass}><code>${body}</code></pre>`,
    )
    state.inCodeBlock = false
    state.codeLang = ''
    state.codeLines = []
  }
}

/**
 * 把 markdown 文本渲染为安全 HTML 字符串。
 * 对 partial 输入（流式未结束）也尽量不破坏。
 */
export function renderMarkdown(input: string): string {
  if (!input) return ''
  const state: BlockState = {
    inCodeBlock: false,
    codeLang: '',
    codeLines: [],
    listType: null,
    listItems: [],
    out: [],
  }

  const lines = input.split('\n')
  let paragraph: string[] = []

  const flushParagraph = () => {
    if (paragraph.length === 0) return
    const joined = paragraph.join(' ').trim()
    if (joined) {
      state.out.push(`<p class="md-paragraph">${renderInline(joined)}</p>`)
    }
    paragraph = []
  }

  for (const raw of lines) {
    const line = raw.replace(/\r$/, '')

    // 代码块 fence
    const fenceMatch = line.match(/^```(\w*)\s*$/)
    if (fenceMatch) {
      if (state.inCodeBlock) {
        // 关闭
        const langClass = state.codeLang
          ? ` class="md-code-lang-${escapeHtml(state.codeLang)}"`
          : ''
        const body = state.codeLines.map(escapeHtml).join('\n')
        state.out.push(
          `<pre class="md-code-block"${langClass}><code>${body}</code></pre>`,
        )
        state.inCodeBlock = false
        state.codeLang = ''
        state.codeLines = []
      } else {
        // 开启
        flushParagraph()
        flushList(state)
        state.inCodeBlock = true
        state.codeLang = fenceMatch[1] || ''
        state.codeLines = []
      }
      continue
    }

    if (state.inCodeBlock) {
      state.codeLines.push(line)
      continue
    }

    // 标题 # ~ ######
    const headingMatch = line.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/)
    if (headingMatch) {
      flushParagraph()
      flushList(state)
      const level = headingMatch[1].length
      const text = headingMatch[2]
      state.out.push(`<h${level} class="md-heading md-h${level}">${renderInline(text)}</h${level}>`)
      continue
    }

    // 水平线
    if (/^[-*_]{3,}\s*$/.test(line)) {
      flushParagraph()
      flushList(state)
      state.out.push('<hr class="md-hr" />')
      continue
    }

    // 无序列表
    const ulMatch = line.match(/^[-*+]\s+(.+)$/)
    if (ulMatch) {
      flushParagraph()
      if (state.listType !== 'ul') {
        flushList(state)
        state.listType = 'ul'
      }
      state.listItems.push(ulMatch[1])
      continue
    }

    // 有序列表
    const olMatch = line.match(/^\d+\.\s+(.+)$/)
    if (olMatch) {
      flushParagraph()
      if (state.listType !== 'ol') {
        flushList(state)
        state.listType = 'ol'
      }
      state.listItems.push(olMatch[1])
      continue
    }

    // 空行：结束当前段落/列表
    if (line.trim() === '') {
      flushParagraph()
      flushList(state)
      continue
    }

    // 普通段落
    flushList(state)
    paragraph.push(line)
  }

  // 收尾
  flushParagraph()
  flushList(state)
  flushCodeBlock(state) // 处理流式中未关闭的代码块

  return state.out.join('')
}
