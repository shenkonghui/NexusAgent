import type { ReactNode } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ConvStatusBar from '../components/ConvStatusBar'
import PermissionDialog from '../components/PermissionDialog'
import ModelSelector from '../components/ModelSelector'
import SessionModeSelector from '../components/SessionModeSelector'
import ContextStats from '../components/ContextStats'
import WorktreePicker from '../components/WorktreePicker'
import { BookOpenText, Code2, FolderGit2 } from 'lucide-react'
import type { PanelCtx, ConfigBarKind } from './types'
import styles from './ChatPanel.module.css'

interface ChatPanelProps {
  ctx: PanelCtx
  configBar: ConfigBarKind
  emptyTitleKey?: string
  emptyHintKey?: string
  placeholderKey?: string
  selectDocFirstKey?: string
  /** 外部注入的自定义配置栏节点，渲染在 PromptInput 下方（优先于内置 configBar） */
  configBarNode?: ReactNode
}

/**
 * 取路径最后一段作为紧凑显示（如 /a/b/.worktrees/task-1 -> task-1）。
 * 空路径返回空字符串。
 */
function cwdBaseName(path?: string): string {
  if (!path) return ''
  const parts = path.replace(/\/+$/, '').split('/')
  return parts[parts.length - 1] || path
}

/**
 * 通用对话列：configBar + 消息列表 + ConvStatusBar + PromptInput。
 * 编码/文档两种模式共用此组件，差异仅在 configBar 样式与 onSend 处理器（由 ctx 传入）。
 */
export default function ChatPanel({
  ctx,
  configBar,
  emptyTitleKey,
  emptyHintKey,
  placeholderKey,
  selectDocFirstKey,
  configBarNode,
}: ChatPanelProps) {
  const { t } = useTranslation()
  const isEmpty = ctx.messages.length === 0
  const conv = ctx.convState
  const disabled = ctx.sessionKind === 'docs' && !ctx.docTarget
  // 新建任务页的工作目录选择器弹窗开关
  const [showDirPicker, setShowDirPicker] = useState(false)

  const placeholder = disabled && selectDocFirstKey
    ? t(selectDocFirstKey)
    : conv !== 'idle'
      ? t(`session.conv_${conv}`)
      : placeholderKey
        ? t(placeholderKey)
        : t('session.promptPlaceholder')

  // 统一配置栏：Agent + 模式 + 模型，所有模式复用同一套控件。
  // 数据源优先用会话级 configOptions（会话详情页，可切换运行时配置），
  // 回退到 probeConfigs（新建任务页，探测出的配置）。
  const builtInConfigBar = configBar !== 'none' ? (() => {
    // 配置选项：会话级 configOptions（会话详情页）或 probeConfigs（新建任务页，已映射为 configOptions）
    const cfgOpts = ctx.configOptions.length > 0 ? ctx.configOptions : ctx.probeConfigs
    const onApplyCfg = ctx.onSetConfigOption
    // Agent：新建任务页可选（有 agents 列表且 onSelectAgent 非空），会话详情页只读显示
    const hasAgentSelect = ctx.agents.length > 0 && ctx.session === null
    const agentValue = hasAgentSelect ? ctx.selectedAgent : (ctx.session?.agent_type || '')
    const agentDisplay = hasAgentSelect
      ? ctx.agents.find((a) => a.type === ctx.selectedAgent)?.display_name || ''
      : (ctx.session?.agent_type || '')

    return (
      <div className={styles.configBar}>
        <div className={styles.configOptions}>
          {/* Agent */}
          {hasAgentSelect ? (
            <select
              className={styles.configSelect}
              value={agentValue}
              onChange={(e) => ctx.onSelectAgent(e.target.value)}
              disabled={ctx.sending || ctx.probing}
            >
              {ctx.agents.length === 0 && <option value="">{t('docMode.noAgent')}</option>}
              {ctx.agents.map((agent) => (
                <option key={agent.type} value={agent.type}>{agent.display_name}</option>
              ))}
            </select>
          ) : agentDisplay ? (
            <span className={styles.configReadonly}>{agentDisplay}</span>
          ) : null}

          {/* 模式：统一用 SessionModeSelector */}
          <SessionModeSelector
            modes={ctx.modes}
            currentModeId={ctx.currentModeId}
            onChange={ctx.onSetMode}
            disabled={ctx.sending}
          />

          {/* 模型（及其它配置项）：用统一的 ModelSelector 渲染 */}
          <ModelSelector
            options={cfgOpts}
            onApply={onApplyCfg}
            disabled={ctx.sending || ctx.probing}
          />

          {/* 工作目录：仅新建任务页（无会话）可选，可选择已存在的 worktree/目录作为本次任务 cwd */}
          {ctx.session === null && ctx.onSelectCwd && (
            <button
              type="button"
              className={styles.cwdBtn}
              onClick={() => setShowDirPicker(true)}
              disabled={ctx.sending || ctx.probing}
              title={ctx.selectedCwd || ctx.cwd || t('session.selectWorktree')}
            >
              <FolderGit2 size={13} />
              <span className={styles.cwdBtnLabel}>
                {cwdBaseName(ctx.selectedCwd || ctx.cwd) || t('session.selectWorktree')}
              </span>
            </button>
          )}
        </div>
        {ctx.session && (
          <div className={styles.statsArea}>
            <ContextStats messages={ctx.messages} sessionId={ctx.sessionId} onCleared={ctx.onContextCleared} />
          </div>
        )}
      </div>
    )
  })() : null

  return (
    <div className={styles.chat}>
      {showDirPicker && ctx.onSelectCwd && (
        <WorktreePicker
          repoPath={ctx.selectedCwd || ctx.cwd || ''}
          selectedPath={ctx.selectedCwd || ctx.cwd || undefined}
          onSelect={(path) => {
            ctx.onSelectCwd?.(path)
            setShowDirPicker(false)
          }}
          onClose={() => setShowDirPicker(false)}
        />
      )}
      {isEmpty && emptyTitleKey ? (
        <div className={styles.empty}>
          {emptyTitleKey.startsWith('codingMode.')
            ? <Code2 size={36} className={styles.emptyIcon} />
            : <BookOpenText size={36} className={styles.emptyIcon} />}
          <h3 className={styles.emptyTitle}>{t(emptyTitleKey)}</h3>
          {emptyHintKey && <p className={styles.emptyHint}>{t(emptyHintKey)}</p>}
        </div>
      ) : (
        <MessageList
          messages={ctx.messages}
          loading={conv === 'streaming' || conv === 'connecting'}
          scheduled={ctx.session?.source === 'scheduled' || ctx.session?.source === 'classify'}
          executions={ctx.executions}
          sessionId={ctx.sessionId}
          cwd={ctx.cwd}
          onRestored={ctx.onRestored}
        />
      )}

      <div className={styles.bottomArea}>
        <ConvStatusBar state={conv}>
          {ctx.pendingPermission && (
            <PermissionDialog
              request={ctx.pendingPermission}
              responding={ctx.permissionResponding}
              onRespond={ctx.onPermissionRespond}
              onCancel={ctx.onPermissionCancel}
            />
          )}
        </ConvStatusBar>
        {ctx.source === 'classify' ? (
          <p className={styles.classifyHint}>{t('notes.classifyTaskHint')}</p>
        ) : (
          // 统一 composer：输入框 + 配置栏合并为一个圆角卡片（参考 Cursor 输入区）
          <div className={styles.composer}>
            <PromptInput
              onSend={ctx.onSend}
              onCancel={ctx.onCancel}
              sending={conv !== 'idle'}
              disabled={disabled}
              value={ctx.restoreInput}
              onValueChange={ctx.onRestoreInputChange}
              commands={ctx.commands}
              modes={ctx.modes}
              skills={ctx.skills}
              cwd={ctx.cwd}
              workspaceId={ctx.workspaceId}
              placeholder={placeholder}
            />
            {configBarNode ?? builtInConfigBar}
          </div>
        )}
      </div>
    </div>
  )
}
