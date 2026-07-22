import { useState, useMemo, useEffect, memo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight, AlertCircle, Loader2 } from 'lucide-react'
import { renderDrawioXmlToSvg, isDrawioXml } from '../utils/drawioViewerRender'
import styles from './DrawioViewer.module.css'

interface DrawioViewerProps {
  /** draw.io (mxGraph) XML 源码 */
  xml: string
}

// 本地高保真渲染：用 draw.io 官方查看器（viewer-static.min.js，内置完整 shape 库）
// 在浏览器端把 XML 渲染为 SVG，离线可用、几乎不失真。渲染是异步的（首次懒加载
// 查看器脚本），因此有 loading / failed 两态。参考实现见 utils/drawioViewerRender.ts。
function DrawioViewer({ xml }: DrawioViewerProps) {
  const { t } = useTranslation()
  const [showSource, setShowSource] = useState(false)
  const [svg, setSvg] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  // 空 XML：可能是"活动块占位"，等待 AI 生成中，显示等待提示而不报错
  const isPlaceholder = !xml.trim()
  const valid = useMemo(() => isDrawioXml(xml), [xml])

  useEffect(() => {
    if (isPlaceholder || !valid) {
      setSvg(null)
      setFailed(false)
      return
    }
    let cancelled = false
    setSvg(null)
    setFailed(false)
    renderDrawioXmlToSvg(xml)
      .then((result) => {
        if (!cancelled) setSvg(result)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [xml, valid, isPlaceholder])

  // 占位块：等待 AI 生成
  if (isPlaceholder) {
    return (
      <div className={styles.wrap}>
        <div className={styles.placeholder}>
          <Loader2 size={14} className={styles.spinner} />
          <span>{t('docAI.waitingGenerate')}</span>
        </div>
      </div>
    )
  }

  // 无效 XML 或渲染失败：报错并提供查看源码
  if (!valid || failed) {
    return (
      <div className={`${styles.wrap} ${styles.errorBox}`}>
        <AlertCircle size={14} />
        <span>{t('docAI.renderFailed')}</span>
        <button
          type="button"
          className={styles.toggleBtn}
          onClick={() => setShowSource((v) => !v)}
        >
          {showSource ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {t('docAI.viewSource')}
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
          title={t('docAI.viewSource')}
        >
          {showSource ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {t('docAI.viewSource')}
        </button>
      </div>

      {svg ? (
        // 本地渲染出的 SVG 直接内联展示
        <div className={styles.svgBox} dangerouslySetInnerHTML={{ __html: svg }} />
      ) : (
        // 渲染中（懒加载查看器脚本 + 出图）
        <div className={styles.svgBox}>
          <div className={styles.placeholder}>
            <Loader2 size={14} className={styles.spinner} />
            <span>{t('common.loading')}</span>
          </div>
        </div>
      )}

      {showSource && (
        <pre className={styles.source}>
          <code>{xml}</code>
        </pre>
      )}
    </div>
  )
}

// memo：xml 不变时跳过重渲染。父级（MarkdownContent / ChatPage）高频重渲染时，
// 避免昂贵的本地渲染被无谓地重复触发，消除图表卡顿。
export default memo(DrawioViewer)
