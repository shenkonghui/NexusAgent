import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'

interface FileViewerValue {
  /** 当前选中的文件绝对路径（来自左侧文件浏览器） */
  openFilePath: string | null
  /** 打开文件 */
  openFile: (path: string) => void
  /** 关闭当前文件 */
  closeFile: () => void
  /**
   * 注册一个「内嵌文件查看器」（如编码模式布局中的 files 面板）。
   * 调用后返回注销函数。存在内嵌查看器时，AppLayout 不再用主区域覆盖层显示文件，
   * 而是交由内嵌面板渲染——从而保留终端 / 对话等其他面板。
   */
  registerEmbedded: () => () => void
  /** 是否存在内嵌查看器 */
  hasEmbedded: boolean
}

const FileViewerContext = createContext<FileViewerValue>({
  openFilePath: null,
  openFile: () => {},
  closeFile: () => {},
  registerEmbedded: () => () => {},
  hasEmbedded: false,
})

export function useFileViewer() {
  return useContext(FileViewerContext)
}

/**
 * 全局文件查看状态。必须置于 AppLayout（各页面渲染）之上，
 * 使「左侧文件浏览器（AppLayout 内）」与「files 面板（页面内容内）」共享同一份选中态。
 */
export function FileViewerProvider({ children }: { children: ReactNode }) {
  const [openFilePath, setOpenFilePath] = useState<string | null>(null)
  const [embeddedCount, setEmbeddedCount] = useState(0)

  const openFile = useCallback((path: string) => setOpenFilePath(path), [])
  const closeFile = useCallback(() => setOpenFilePath(null), [])

  const registerEmbedded = useCallback(() => {
    setEmbeddedCount((c) => c + 1)
    return () => setEmbeddedCount((c) => c - 1)
  }, [])

  return (
    <FileViewerContext.Provider
      value={{ openFilePath, openFile, closeFile, registerEmbedded, hasEmbedded: embeddedCount > 0 }}
    >
      {children}
    </FileViewerContext.Provider>
  )
}
