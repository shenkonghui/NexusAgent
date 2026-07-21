import { useEffect, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import TerminalPanel from '../components/Terminal'
import ChangesPanel from '../components/ChangesPanel'
import DebugPanel from '../components/DebugPanel'
import WorkspaceFileEditor from '../components/WorkspaceFileEditor'
import { useFileViewer } from '../context/FileViewerContext'
import { Folder, SquareTerminal, Pencil, Bug, MessageSquare } from 'lucide-react'
import type { PanelDef, PanelCtx } from './types'
import ChatPanel from './ChatPanel'

/** 通用空占位（要求会话 / 提示选文档等） */
function EmptyPanel({
  hintKey,
  subKey,
  icon,
}: {
  hintKey: string
  subKey?: string
  icon?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div
      style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        padding: 24,
        color: 'var(--text-muted)',
        textAlign: 'center',
        background: 'var(--bg-base)',
      }}
    >
      {icon && <div style={{ opacity: 0.45 }}>{icon}</div>}
      <div style={{ fontSize: 14 }}>{t(hintKey)}</div>
      {subKey && <div style={{ fontSize: 12, maxWidth: 280 }}>{t(subKey)}</div>}
    </div>
  )
}

/**
 * files 面板：不再自带文件树，仅展示左侧文件浏览器选中的文件内容。
 * 挂载时向 FileViewer 注册为内嵌查看器，使 AppLayout 不再用主区域覆盖层显示文件。
 */
function SelectedFileView() {
  const { openFilePath, closeFile, registerEmbedded } = useFileViewer()
  useEffect(() => registerEmbedded(), [registerEmbedded])

  if (!openFilePath) {
    return <EmptyPanel hintKey="fileBrowser.selectHint" icon={<Folder size={40} />} />
  }
  return <WorkspaceFileEditor key={openFilePath} path={openFilePath} onClose={closeFile} />
}

function renderFiles() {
  return <SelectedFileView />
}

function renderTerminal(ctx: PanelCtx) {
  if (!ctx.sessionId) return <EmptyPanel hintKey="panel.requireSession" />
  return <TerminalPanel sessionId={ctx.sessionId} onClose={() => {}} />
}

function renderChanges(ctx: PanelCtx) {
  if (!ctx.sessionId) return <EmptyPanel hintKey="panel.requireSession" />
  return <ChangesPanel sessionId={ctx.sessionId} onClose={() => {}} refreshKey={ctx.restoreRefreshKey ?? 0} />
}

function renderDebug(ctx: PanelCtx) {
  if (!ctx.sessionId) return <EmptyPanel hintKey="panel.requireSession" />
  return <DebugPanel sessionId={ctx.sessionId} />
}

/**
 * 对话面板：configBar 样式与空态文案由 ChatPage 通过 ctx.__chatConfig 注入。
 * 这样 PanelDef.render 签名不变（仅依赖 ctx），同时允许不同模式定制对话列外观。
 */
function renderChat(ctx: PanelCtx) {
  type ChatConfig = {
    configBar: 'coding' | 'none'
    emptyTitleKey?: string
    emptyHintKey?: string
    placeholderKey?: string
  }
  const cfg = (ctx as PanelCtx & { __chatConfig?: ChatConfig }).__chatConfig
  return (
    <ChatPanel
      ctx={ctx}
      configBar={cfg?.configBar ?? 'none'}
      emptyTitleKey={cfg?.emptyTitleKey}
      emptyHintKey={cfg?.emptyHintKey}
      placeholderKey={cfg?.placeholderKey}
    />
  )
}

/** 面板注册表。新增面板在此加一条；新增模式只需在 MODES 引用面板 id。 */
export const PANELS: PanelDef[] = [
  { id: 'chat', titleKey: 'panel.chat', icon: <MessageSquare size={14} />, render: renderChat },
  { id: 'files', titleKey: 'panel.files', icon: <Folder size={14} />, render: renderFiles },
  { id: 'terminal', titleKey: 'panel.terminal', icon: <SquareTerminal size={14} />, render: renderTerminal },
  { id: 'changes', titleKey: 'panel.changes', icon: <Pencil size={14} />, render: renderChanges },
  { id: 'debug', titleKey: 'panel.debug', icon: <Bug size={14} />, render: renderDebug },
]
