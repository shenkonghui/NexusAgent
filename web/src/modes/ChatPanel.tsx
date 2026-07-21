import { useTranslation } from 'react-i18next'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ConvStatusBar from '../components/ConvStatusBar'
import PermissionDialog from '../components/PermissionDialog'
import ModelSelector from '../components/ModelSelector'
import SessionModeSelector from '../components/SessionModeSelector'
import ContextStats from '../components/ContextStats'
import type { PanelCtx, ConfigBarKind } from './types'
import styles from './ChatPanel.module.css'

interface ChatPanelProps {
  ctx: PanelCtx
  configBar: ConfigBarKind
  emptyTitleKey?: string
  emptyHintKey?: string
  placeholderKey?: string
}

/**
 * 通用对话列：configBar + 消息列表 + ConvStatusBar + PromptInput。
 * 配置栏样式与空态文案由外部通过 props 注入。
 */
export default function ChatPanel({
  ctx,
  configBar,
  emptyTitleKey,
  emptyHintKey,
  placeholderKey,
}: ChatPanelProps) {
  const { t } = useTranslation()
  const isEmpty = ctx.messages.length === 0
  const conv = ctx.convState

  const placeholder = conv !== 'idle'
    ? t(`session.conv_${conv}`)
    : placeholderKey
      ? t(placeholderKey)
      : t('session.promptPlaceholder')

  const configBar_node = configBar !== 'none' ? (
    <div className={styles.configBar}>
      <div className={styles.configOptions}>
        <SessionModeSelector
          modes={ctx.modes}
          currentModeId={ctx.currentModeId}
          onChange={ctx.onSetMode}
          disabled={ctx.sending}
        />
        <ModelSelector
          options={ctx.configOptions}
          onApply={ctx.onSetConfigOption}
          disabled={ctx.sending}
        />
      </div>
      <div className={styles.statsArea}>
        <ContextStats messages={ctx.messages} sessionId={ctx.sessionId} onCleared={ctx.onContextCleared} />
      </div>
    </div>
  ) : null

  return (
    <div className={styles.chat}>
      {isEmpty && emptyTitleKey ? (
        <div className={styles.empty}>
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
          <PromptInput
            onSend={ctx.onSend}
            onCancel={ctx.onCancel}
            sending={conv !== 'idle'}
            value={ctx.restoreInput}
            onValueChange={ctx.onRestoreInputChange}
            commands={ctx.commands}
            modes={ctx.modes}
            skills={ctx.skills}
            cwd={ctx.cwd}
            workspaceId={ctx.workspaceId}
            placeholder={placeholder}
          />
        )}
        {configBar_node}
      </div>
    </div>
  )
}
