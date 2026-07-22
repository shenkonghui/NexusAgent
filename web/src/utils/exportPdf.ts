import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

/**
 * 把 Markdown 文档导出为 PDF（点击直接下载 .pdf，不弹浏览器打印框）。
 *
 * 架构（绕开浏览器原生打印）：
 *   1. drawio 块：用 drawio embed viewer 协议把 XML 转成真实 SVG 字符串
 *      （html2canvas 无法截图跨域 iframe，所以必须先把 drawio 转成内联 SVG
 *      再嵌入，否则图表会是空白）。
 *   2. 正文 + 嵌入的 SVG：渲染到一个屏幕外的白底容器（独立 DOM，不依赖当前
 *      是 view/edit 模式，不受主应用布局污染）。
 *   3. html2pdf.js（html2canvas + jsPDF）对该容器截图生成 PDF，直接下载。
 *
 * drawio 转换失败/超时时降级为显示 XML 源码，不阻塞导出。
 */

const REMARK_PLUGINS = [remarkGfm]

const DRAWIO_EMBED_URL = 'https://viewer.diagrams.net?embed=1&proto=json&browser=0&ui=min'

// 匹配 fenced ```drawio / ```draw 代码块，捕获其 XML 源码
const DRAWIO_BLOCK_RE = /```(?:drawio|draw)\s*\n([\s\S]*?)```/g

/** 从 Markdown 中提取所有 drawio 块的 XML（去重，保持出现顺序） */
function extractDrawioBlocks(content: string): string[] {
  const seen = new Set<string>()
  const list: string[] = []
  let m: RegExpExecArray | null
  DRAWIO_BLOCK_RE.lastIndex = 0
  while ((m = DRAWIO_BLOCK_RE.exec(content)) !== null) {
    const xml = m[1].replace(/\n$/, '').trim()
    if (xml && !seen.has(xml)) {
      seen.add(xml)
      list.push(xml)
    }
  }
  return list
}

/**
 * 通过 drawio embed viewer 把一段 mxGraph XML 转换为 SVG 字符串。
 *
 * 协议（官方 embed mode）：
 *   1. iframe 加载 viewer.diagrams.net
 *   2. 收到 {event:'init'} → 发送 {action:'load', xml}
 *   3. 收到 {event:'load'} → 发送 {action:'export', format:'svg'}
 *   4. 收到 {event:'export'} → data 即 SVG 字符串
 *
 * 注意：依赖远程 viewer.diagrams.net，网络不通时通过 timeout 快速失败，
 * 调用方负责兜底（降级为源码）。
 */
function exportDrawioToSvg(xml: string, timeoutMs = 15000): Promise<string> {
  return new Promise((resolve, reject) => {
    const iframe = document.createElement('iframe')
    iframe.setAttribute('aria-hidden', 'true')
    iframe.title = 'drawio-export'
    // drawio viewer 内部 mxGraph 按容器实际尺寸渲染图形，必须给实际尺寸。
    iframe.style.position = 'fixed'
    iframe.style.left = '-99999px'
    iframe.style.top = '0'
    iframe.style.width = '1024px'
    iframe.style.height = '768px'
    iframe.style.border = '0'
    document.body.appendChild(iframe)

    let settled = false
    let exportRequested = false
    let attempts = 0

    const timer = window.setTimeout(() => {
      done(new Error('drawio export timeout'))
    }, timeoutMs)

    const done = (err: Error | null, svg?: string) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      window.removeEventListener('message', onMessage)
      if (iframe.parentNode) iframe.parentNode.removeChild(iframe)
      if (err) reject(err)
      else resolve(svg || '')
    }

    // SVG 是否包含实际绘图内容（path/rect/ellipse/image/text 等），而非空壳
    const hasContent = (svg: string): boolean => {
      return /<(path|rect|ellipse|image|text|g\s|foreignObject|line|polygon)\b/i.test(svg)
    }

    const requestExport = (format: 'svg' | 'png') => {
      const win = iframe.contentWindow
      if (!win) return
      win.postMessage(
        JSON.stringify({ action: 'export', format, xml, spin: 'Exporting', scale: 2 }),
        '*',
      )
    }

    const onMessage = (e: MessageEvent) => {
      if (e.source !== iframe.contentWindow) return
      let data: any
      try {
        data = typeof e.data === 'string' ? JSON.parse(e.data) : e.data
      } catch {
        return
      }
      const win = iframe.contentWindow
      if (!win) return

      if (data?.event === 'init') {
        win.postMessage(JSON.stringify({ action: 'load', xml, autosave: 0 }), '*')
      } else if (data?.event === 'load' && !exportRequested) {
        exportRequested = true
        // 关键：load 只表示 XML 已解析，图形尚未渲染完。延迟后再 export，
        // 给 mxGraph 时间完成布局与绘制。延迟梯度递增，逐步重试拿非空结果。
        const tryExport = () => {
          attempts += 1
          requestExport('svg')
        }
        // 首次延迟 600ms，之后若拿不到内容会逐步延长重试
        window.setTimeout(tryExport, 600)
      } else if (data?.event === 'export') {
        const svg = typeof data.data === 'string' ? data.data : ''
        if (svg && svg.includes('<svg') && hasContent(svg)) {
          done(null, svg)
        } else if (attempts < 5) {
          // 空结果：图形尚未渲染完，延长等待后重试（600→1200→1800→2400→3000ms）
          window.setTimeout(() => {
            attempts += 1
            requestExport('svg')
          }, 600 * attempts)
        } else {
          done(new Error('drawio export empty after retries'))
        }
      }
    }

    window.addEventListener('message', onMessage)
    iframe.src = DRAWIO_EMBED_URL
  })
}

/**
 * 批量把 drawio XML 转成 SVG。
 * 并行转换 + 总时间预算：即使 viewer.diagrams.net 完全连不上，最多等 budgetMs
 * 后放弃剩余未完成的图，降级为源码，保证导出流程永不卡死。
 */
async function convertDrawioBlocks(
  xmls: string[],
  onProgress?: (done: number, total: number) => void,
  budgetMs = 20000,
): Promise<Map<string, string>> {
  const map = new Map<string, string>()
  let completed = 0
  const total = xmls.length

  const tasks = xmls.map((xml) =>
    exportDrawioToSvg(xml)
      .then((svg) => {
        if (svg) map.set(xml, svg)
      })
      .catch(() => {
        /* 单张失败：留空，渲染时降级为源码 */
      })
      .finally(() => {
        completed += 1
        onProgress?.(completed, total)
      }),
  )

  const budget = new Promise<void>((resolve) => window.setTimeout(resolve, budgetMs))
  await Promise.race([Promise.allSettled(tasks), budget])
  return map
}

/**
 * 把 SVG 字符串处理成可用于 <img src> 的 data URI。
 *
 * html2canvas 对内联 <svg> 支持很差（常渲染空白），但对 <img> 里加载的
 * SVG 能完整光栅化。因此把 SVG 转成 data URI 用 <img> 嵌入更可靠。
 * 同时确保 SVG 有明确 width/height（drawio 导出的 SVG 有时只有 viewBox），
 * 否则 <img> 可能渲染为 0 尺寸。
 */
function svgToImgDataUri(svg: string): string {
  let s = svg.trim()
  // 若无 width/height 但有 viewBox，从 viewBox 提取尺寸写回
  const hasWH = /\swidth=/.test(s) && /\sheight=/.test(s)
  if (!hasWH) {
    const vb = /viewBox=["']([^"']+)["']/.exec(s)
    if (vb) {
      const parts = vb[1].trim().split(/[\s,]+/)
      const w = parts[2] ? Math.ceil(parseFloat(parts[2])) : 800
      const h = parts[3] ? Math.ceil(parseFloat(parts[3])) : 600
      s = s.replace(/<svg/, `<svg width="${w}" height="${h}"`)
    } else {
      // 兜底默认尺寸
      s = s.replace(/<svg/, '<svg width="800" height="600"')
    }
  }
  // 用 encodeURIComponent 处理 SVG（支持 Unicode/特殊字符），避免 btoa 报错
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(s)}`
}

/** 渲染阶段用的 components：drawio 块嵌入真实 SVG（用 <img> data URI，
 *  html2canvas 对此支持远好于内联 <svg>），无 SVG 时降级显示源码 */
function makePrintComponents(svgMap: Map<string, string>): Components {
  return {
    code({ className: langClass, children, ...props }) {
      const match = /language-(\w+)/.exec(langClass || '')
      const lang = match?.[1]
      if (lang === 'drawio' || lang === 'draw') {
        const xml = String(children).replace(/\n$/, '').trim()
        const svg = svgMap.get(xml)
        if (svg) {
          return createElement('div', { className: 'drawio-diagram' }, createElement('img', {
            src: svgToImgDataUri(svg),
            alt: 'draw.io diagram',
            style: { maxWidth: '100%', height: 'auto' },
          }))
        }
        return createElement(
          'div',
          null,
          createElement(
            'p',
            { style: { color: '#8a8a98', fontSize: '12px', margin: '0 0 4px' } },
            'draw.io diagram (source):',
          ),
          createElement('pre', null, xml),
        )
      }
      return createElement('code', { className: langClass, ...props }, children)
    },
  }
}

/** PDF 专用白底浅色样式（独立于 App 主题） */
const PDF_STYLE = `
  .pdf-export-root {
    background: #ffffff;
    color: #1a1d23;
    font-family: 'Outfit', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto,
      'Helvetica Neue', Arial, 'PingFang SC', 'Hiragino Sans GB',
      'Microsoft YaHei', sans-serif;
    padding: 0;
  }
  .pdf-export-title {
    font-size: 20px;
    font-weight: 700;
    color: #1a1d23;
    margin-bottom: 16px;
    padding-bottom: 8px;
    border-bottom: 2px solid #e0e0e6;
  }
  .markdown-body {
    font-size: 14px;
    line-height: 1.7;
    color: #1a1d23;
  }
  .markdown-body h1 {
    font-size: 24px; font-weight: 700; margin: 20px 0 12px;
    padding-bottom: 6px; border-bottom: 1px solid #e0e0e6;
  }
  .markdown-body h2 {
    font-size: 20px; font-weight: 600; margin: 18px 0 10px;
    padding-bottom: 4px; border-bottom: 1px solid #eeeeee;
  }
  .markdown-body h3 { font-size: 17px; font-weight: 600; margin: 14px 0 8px; }
  .markdown-body h4 { font-size: 15px; font-weight: 600; margin: 12px 0 6px; }
  .markdown-body p { margin: 8px 0; }
  .markdown-body ul, .markdown-body ol { margin: 8px 0; padding-left: 24px; }
  .markdown-body li { margin: 3px 0; }
  .markdown-body code {
    background: #f3f4f6; padding: 2px 6px; border-radius: 4px;
    font-size: 13px; color: #1f2937;
    font-family: 'IBM Plex Mono', 'Monaco', 'Menlo', monospace;
  }
  .markdown-body pre {
    background: #f6f8fa; border: 1px solid #e0e0e6; border-radius: 6px;
    padding: 12px; overflow-x: auto; margin: 12px 0;
  }
  .markdown-body pre code { background: none; padding: 0; }
  .markdown-body blockquote {
    border-left: 3px solid #d4a718; padding: 4px 14px; margin: 12px 0;
    color: #4b5563; background: #f8f8fa;
  }
  .markdown-body table {
    border-collapse: collapse; width: 100%; margin: 12px 0;
  }
  .markdown-body th, .markdown-body td {
    border: 1px solid #e0e0e6; padding: 6px 10px; text-align: left; font-size: 13px;
  }
  .markdown-body th { background: #f8f8fa; font-weight: 600; }
  .markdown-body hr { border: none; border-top: 1px solid #e0e0e6; margin: 20px 0; }
  .markdown-body img { max-width: 100%; border-radius: 4px; }
  .markdown-body a { color: #b8941a; text-decoration: none; }
  .drawio-diagram { margin: 12px 0; text-align: center; }
  .drawio-diagram svg { max-width: 100%; height: auto; }
`

export interface ExportOptions {
  /** drawio 图表转换进度回调（done / total） */
  onDrawioProgress?: (done: number, total: number) => void
}

/**
 * 把 Markdown 内容导出为 PDF 并下载。
 *
 * 流程：转换 drawio → 渲染到屏幕外白底容器 → html2pdf 截图生成 PDF → 清理容器。
 *
 * @param title 文档标题（用于 PDF 文件名和顶部标题）
 * @param content Markdown 原文
 */
export async function exportMarkdownToPdf(
  title: string,
  content: string,
  options: ExportOptions = {},
): Promise<void> {
  // 1. 提取并转换 drawio 块为 SVG（若有）
  const drawioXmls = extractDrawioBlocks(content)
  const svgMap =
    drawioXmls.length > 0
      ? await convertDrawioBlocks(drawioXmls, options.onDrawioProgress)
      : new Map<string, string>()

  // 2. 创建屏幕外容器（白底，固定宽度让 html2canvas 拿到正常排版）
  const host = document.createElement('div')
  host.style.position = 'fixed'
  host.style.left = '-99999px'
  host.style.top = '0'
  host.style.width = '794px' // A4 @ 96dpi 宽度
  host.style.background = '#ffffff'
  host.style.zIndex = '-1'
  document.body.appendChild(host)

  // 注入样式
  const styleEl = document.createElement('style')
  styleEl.setAttribute('data-pdf-export', 'true')
  styleEl.textContent = PDF_STYLE
  document.head.appendChild(styleEl)

  const root = createRoot(host)
  try {
    // 3. 渲染 React（MarkdownContent + 嵌入的 drawio SVG as <img>）
    await new Promise<void>((resolve) => {
      root.render(
        createElement(
          'div',
          { className: 'pdf-export-root' },
          title ? createElement('div', { className: 'pdf-export-title' }, title) : null,
          createElement(
            'div',
            { className: 'markdown-body' },
            createElement(
              ReactMarkdown,
              { remarkPlugins: REMARK_PLUGINS, components: makePrintComponents(svgMap) },
              content,
            ),
          ),
        ),
      )
      // 等 React 渲染完成 + 一帧布局
      requestAnimationFrame(() =>
        requestAnimationFrame(() => resolve()),
      )
    })

    // 3.5 等待容器内所有 <img>（drawio SVG）加载完成，否则 html2canvas 会截到空白
    await waitForImages(host)

    // 4. html2pdf 生成 PDF 并下载（动态导入，避免拖慢首屏）
    const filename = `${sanitizeFilename(title) || 'document'}.pdf`
    const { default: html2pdf } = await import('html2pdf.js')
    await html2pdf()
      .set({
        margin: [10, 10, 10, 10],
        filename,
        image: { type: 'jpeg', quality: 0.98 },
        html2canvas: {
          scale: 2,
          useCORS: true,
          backgroundColor: '#ffffff',
          logging: false,
        } as unknown as object,
        jsPDF: { unit: 'mm', format: 'a4', orientation: 'portrait' as const },
      })
      .from(host)
      .save()
  } finally {
    // 5. 清理
    root.unmount()
    if (host.parentNode) host.parentNode.removeChild(host)
    styleEl.remove()
  }
}

function sanitizeFilename(name: string): string {
  return name.replace(/[\\/:*?"<>|]/g, '_').trim()
}

/** 等待容器内所有 <img> 加载完成（drawio SVG data URI），超时后也放行 */
function waitForImages(container: HTMLElement, timeoutMs = 5000): Promise<void> {
  const imgs = Array.from(container.querySelectorAll('img'))
  if (imgs.length === 0) return Promise.resolve()
  return new Promise<void>((resolve) => {
    let remaining = imgs.length
    let done = false
    const finish = () => {
      if (!done) {
        done = true
        resolve()
      }
    }
    const timer = window.setTimeout(finish, timeoutMs)
    imgs.forEach((img) => {
      if (img.complete) {
        remaining -= 1
        if (remaining === 0) {
          window.clearTimeout(timer)
          finish()
        }
      } else {
        const onDone = () => {
          remaining -= 1
          if (remaining === 0) {
            window.clearTimeout(timer)
            finish()
          }
        }
        img.addEventListener('load', onDone, { once: true })
        img.addEventListener('error', onDone, { once: true })
      }
    })
    // 若全部已 complete
    if (remaining === 0) finish()
  })
}
