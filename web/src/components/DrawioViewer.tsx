import { useState, useMemo, useCallback, useEffect, useRef, memo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight, AlertCircle, Loader2 } from 'lucide-react'
import styles from './DrawioViewer.module.css'

interface DrawioViewerProps {
  /** draw.io (mxGraph) XML 源码 */
  xml: string
}

// 简单校验：必须像一段 mxGraphModel / mxfile / mxGraph 的 XML。
// 空字符串也视为"占位等待生成"，不报错只显示等待提示。
function looksLikeDrawioXml(xml: string): boolean {
  const s = xml.trim()
  if (!s) return false
  if (!s.startsWith('<')) return false
  return /<(mxGraphModel|mxfile|mxGraph)\b/i.test(s)
}

// draw.io embed 协议（官方推荐）：
//   1. iframe 加载 https://viewer.diagrams.net（不带 hash，避免被当作压缩数据报错）
//   2. iframe load 后通过 postMessage({event:'init'}) 握手
//   3. 收到 {event:'init'} 后 postMessage({action:'load', xml}) 推图
//   4. xml 变化时再次 'load'，图原地刷新（无需重建 iframe）
// 参考：https://www.drawio.com/doc/faq/embed-mode
const DRAWIO_EMBED_URL = 'https://viewer.diagrams.net?embed=1&proto=json&browser=0&ui=min'

function DrawioViewer({ xml }: DrawioViewerProps) {
  const { t } = useTranslation()
  const [showSource, setShowSource] = useState(false)
  const [ready, setReady] = useState(false) // iframe 已握手
  const [failed, setFailed] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const xmlRef = useRef(xml)
  xmlRef.current = xml

  const valid = useMemo(() => looksLikeDrawioXml(xml), [xml])
  // 空 XML：可能是"活动块占位"，等待 AI 生成中，显示等待提示而不报错
  const isPlaceholder = !xml.trim()

  // iframe load 后未在 8s 内收到 init 握手则视为失败（网络不通等）
  const loadTimeoutRef = useRef<number | undefined>(undefined)

  const postLoad = useCallback((content: string) => {
    const win = iframeRef.current?.contentWindow
    if (!win || !content.trim()) return
    win.postMessage(JSON.stringify({ action: 'load', xml: content, autosave: 0 }), '*')
  }, [])

  const handleMessage = useCallback(
    (e: MessageEvent) => {
      if (e.source !== iframeRef.current?.contentWindow) return
      let data: any
      try {
        data = typeof e.data === 'string' ? JSON.parse(e.data) : e.data
      } catch {
        return
      }
      if (data?.event === 'init') {
        window.clearTimeout(loadTimeoutRef.current)
        setReady(true)
        setFailed(false)
        // 握手成功立即推一次当前 XML
        postLoad(xmlRef.current)
      }
    },
    [postLoad],
  )

  useEffect(() => {
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [handleMessage])

  // xml 变化 & 已握手 → 推送新图（活动块原地刷新的关键）
  useEffect(() => {
    if (ready && valid) postLoad(xml)
  }, [ready, valid, xml, postLoad])

  const handleIframeLoad = useCallback(() => {
    // iframe DOM load 完成，等待 init 握手；超时兜底报错
    window.clearTimeout(loadTimeoutRef.current)
    loadTimeoutRef.current = window.setTimeout(() => {
      setReady((r) => {
        if (!r) setFailed(true)
        return r
      })
    }, 8000)
  }, [])

  // 占位块：等待 AI 生成
  if (isPlaceholder) {
    return (
      <div className={styles.wrap}>
        <div className={styles.placeholder}>
          <Loader2 size={14} className={styles.spinner} />
          <span>{t('drawio.waitingGenerate')}</span>
        </div>
      </div>
    )
  }

  if (!valid) {
    return (
      <div className={`${styles.wrap} ${styles.errorBox}`}>
        <AlertCircle size={14} />
        <span>{t('drawio.renderFailed')}</span>
        <button
          type="button"
          className={styles.toggleBtn}
          onClick={() => setShowSource((v) => !v)}
        >
          {showSource ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {t('drawio.viewSource')}
        </button>
        {showSource && (
          <pre className={styles.source}>
            <code>{xml}</code>
          </pre>
        )}
      </div>
    )
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.toolbar}>
        <button
          type="button"
          className={styles.toggleBtn}
          onClick={() => setShowSource((v) => !v)}
          title={t('drawio.viewSource')}
        >
          {showSource ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {t('drawio.viewSource')}
        </button>
      </div>

      <div className={styles.frameBox}>
        {!ready && !failed && (
          <div className={styles.loading}>
            <Loader2 size={16} className={styles.spinner} />
            <span>{t('common.loading')}</span>
          </div>
        )}
        {failed ? (
          <div className={styles.errorOverlay}>
            <AlertCircle size={14} />
            <span>{t('drawio.renderFailed')}</span>
          </div>
        ) : (
          <iframe
            ref={iframeRef}
            src={DRAWIO_EMBED_URL}
            className={styles.frame}
            onLoad={handleIframeLoad}
            title="draw.io"
            sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
          />
        )}
      </div>

      {showSource && (
        <pre className={styles.source}>
          <code>{xml}</code>
        </pre>
      )}
    </div>
  )
}

// memo：xml 不变时跳过重渲染。父级（MarkdownContent / ChatPage）高频重渲染时，
// 避免 iframe 及其内部 draw.io 视图被无谓地重新处理，消除图表卡顿。
export default memo(DrawioViewer)
