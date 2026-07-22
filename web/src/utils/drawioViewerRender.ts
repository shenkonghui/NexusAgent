// 高保真 draw.io 本地渲染：直接使用 draw.io 官方只读查看器（viewer-static.min.js，
// 内置完整 shape/stencil 库），在浏览器端把 XML 渲染为与 draw.io 本尊一致的 SVG。
//
// 相比 @maxgraph/core，官方查看器用的是 draw.io 自己的渲染代码与图形库，
// 复杂形状 / 自定义样式几乎不失真。脚本自包含、离线可用（个别冷门 stencil
// 才会按需回源 diagrams.net）。
//
// 用法：renderDrawioXmlToSvg(xml) -> Promise<svgString>。首次调用会懒加载查看器脚本。

// 官方查看器脚本路径（打包在 public/drawio/ 下，随前端一起分发，离线可用）
const VIEWER_SCRIPT_URL = '/drawio/viewer-static.min.js'
const SVG_NS = 'http://www.w3.org/2000/svg'
// 导出 SVG 四周留白（px），避免描边 / 箭头被裁掉
const PADDING = 8

// svg 用户坐标系下的内容包围盒
type DrawioBounds = { x: number; y: number; width: number; height: number }

/* eslint-disable @typescript-eslint/no-explicit-any */
declare global {
  interface Window {
    GraphViewer?: any
    mxLoadResources?: boolean
    mxLoadStylesheets?: boolean
  }
}

/** 是否看起来像一段 draw.io / mxGraph XML（与预览校验保持一致） */
export function isDrawioXml(xml: string): boolean {
  const s = xml.trim()
  if (!s || !s.startsWith('<')) return false
  return /<(mxGraphModel|mxfile|mxGraph)\b/i.test(s)
}

let loadPromise: Promise<void> | null = null

// 懒加载官方查看器脚本（只加载一次）。脚本会在 window 上挂 GraphViewer。
function loadViewer(): Promise<void> {
  if (window.GraphViewer) return Promise.resolve()
  if (loadPromise) return loadPromise
  loadPromise = new Promise<void>((resolve, reject) => {
    // 关闭 mxGraph 的远程资源 / 样式表加载，避免离线环境下的无谓网络请求
    window.mxLoadResources = false
    window.mxLoadStylesheets = false
    const script = document.createElement('script')
    script.src = VIEWER_SCRIPT_URL
    script.async = true
    script.onload = () => {
      if (window.GraphViewer) resolve()
      else reject(new Error('GraphViewer not available after script load'))
    }
    script.onerror = () => {
      loadPromise = null // 允许后续重试
      reject(new Error('failed to load drawio viewer script'))
    }
    document.head.appendChild(script)
  })
  return loadPromise
}

// 创建一个在文档流内、但移出视口的离屏容器（GraphViewer 需真实 DOM 完成布局测量）
function createOffscreenHost(): HTMLDivElement {
  const host = document.createElement('div')
  host.style.position = 'absolute'
  host.style.left = '-99999px'
  host.style.top = '0'
  host.style.width = '1600px'
  host.style.background = '#ffffff'
  document.body.appendChild(host)
  return host
}

// 把 GraphViewer 渲染出的 svg 处理成独立、尺寸自适应内容、带白底的 SVG 字符串。
// 关键：查看器为了居中 / 适配容器，会把 svg 尺寸设成容器宽度（内联 style width/height:100%）
// 并给内容加平移，直接沿用会导致图形偏移、被裁切或整体缩得很小。
// bounds 传入查看器 graph.getGraphBounds()（svg 用户坐标系下内容的权威包围盒），
// 据此重写 viewBox / width / height，并清掉百分比内联尺寸，让 SVG 紧贴图形内容。
function finalizeSvg(svg: SVGSVGElement, bounds?: DrawioBounds | null): string {
  const clone = svg.cloneNode(true) as SVGSVGElement
  clone.setAttribute('xmlns', SVG_NS)
  clone.setAttribute('xmlns:xlink', 'http://www.w3.org/1999/xlink')
  // 清掉查看器注入的百分比 / 定位 / 最小尺寸等内联样式，避免覆盖 width/height 属性
  clone.style.removeProperty('width')
  clone.style.removeProperty('height')
  clone.style.removeProperty('position')
  clone.style.removeProperty('left')
  clone.style.removeProperty('top')
  clone.style.removeProperty('min-width')
  clone.style.removeProperty('min-height')
  clone.style.removeProperty('max-width')
  clone.style.removeProperty('background-image')
  clone.style.removeProperty('background-color')
  clone.style.cursor = 'default'

  // 计算最终 viewBox / 尺寸：优先用 graph 权威包围盒，其次沿用查看器 viewBox
  let vx: number, vy: number, vw: number, vh: number
  if (bounds && bounds.width > 0 && bounds.height > 0) {
    vx = Math.floor(bounds.x) - PADDING
    vy = Math.floor(bounds.y) - PADDING
    vw = Math.ceil(bounds.width) + PADDING * 2
    vh = Math.ceil(bounds.height) + PADDING * 2
  } else {
    const vb = clone.getAttribute('viewBox')
    if (vb) {
      const p = vb.trim().split(/[\s,]+/).map(Number)
      vx = p[0] || 0
      vy = p[1] || 0
      vw = p[2] || 800
      vh = p[3] || 600
    } else {
      vx = 0
      vy = 0
      vw = parseFloat(clone.getAttribute('width') || '800') || 800
      vh = parseFloat(clone.getAttribute('height') || '600') || 600
    }
  }
  clone.setAttribute('viewBox', `${vx} ${vy} ${vw} ${vh}`)
  clone.setAttribute('width', String(vw))
  clone.setAttribute('height', String(vh))
  clone.setAttribute('preserveAspectRatio', 'xMidYMid meet')

  // 白色背景（PDF / 图片导出更干净），插到最前作为底层
  const bg = document.createElementNS(SVG_NS, 'rect')
  bg.setAttribute('x', String(vx))
  bg.setAttribute('y', String(vy))
  bg.setAttribute('width', String(vw))
  bg.setAttribute('height', String(vh))
  bg.setAttribute('fill', '#ffffff')
  clone.insertBefore(bg, clone.firstChild)

  return new XMLSerializer().serializeToString(clone)
}

/**
 * 把 draw.io / mxGraph XML 用官方查看器本地渲染为独立 SVG 字符串（高保真）。
 * 失败 / 超时时 reject，调用方可回退到显示源码。
 */
export function renderDrawioXmlToSvg(xml: string, timeoutMs = 15000): Promise<string> {
  if (!isDrawioXml(xml)) {
    return Promise.reject(new Error('not a draw.io/mxGraph xml'))
  }
  return loadViewer().then(
    () =>
      new Promise<string>((resolve, reject) => {
        const host = createOffscreenHost()
        let settled = false
        const cleanup = () => host.remove()
        const fail = (err: Error) => {
          if (settled) return
          settled = true
          cleanup()
          reject(err)
        }
        const succeed = (svg: string) => {
          if (settled) return
          settled = true
          cleanup()
          resolve(svg)
        }

        const timer = window.setTimeout(() => fail(new Error('drawio render timeout')), timeoutMs)

        try {
          // 配置见官方文档 data-mxgraph：resize 自适应尺寸、无导航/工具栏、只读、居中
          const el = document.createElement('div')
          el.className = 'mxgraph'
          el.setAttribute(
            'data-mxgraph',
            JSON.stringify({
              xml,
              resize: true,
              border: 8,
              nav: false,
              toolbar: null,
              'auto-fit': true,
              center: false,
              lightbox: false,
              editable: false,
              tooltips: false,
            }),
          )
          host.appendChild(el)

          window.GraphViewer.createViewerForElement(el, (viewer: any) => {
            // 渲染完成回调后再等一帧，确保 svg 已插入并完成布局
            requestAnimationFrame(() => {
              window.clearTimeout(timer)
              const svg = el.querySelector('svg') as SVGSVGElement | null
              if (!svg) {
                fail(new Error('drawio viewer produced no svg'))
                return
              }
              // 取查看器 graph 的内容权威包围盒（svg 用户坐标系），用于精确裁剪
              let bounds: DrawioBounds | null = null
              try {
                const gb = viewer?.graph?.getGraphBounds?.()
                if (gb && gb.width > 0 && gb.height > 0) {
                  bounds = { x: gb.x, y: gb.y, width: gb.width, height: gb.height }
                }
              } catch {
                /* 拿不到就落到 finalizeSvg 内的 viewBox 兜底 */
              }
              try {
                succeed(finalizeSvg(svg, bounds))
              } catch (e) {
                fail(e as Error)
              }
            })
          })
        } catch (e) {
          window.clearTimeout(timer)
          fail(e as Error)
        }
      }),
  )
}
