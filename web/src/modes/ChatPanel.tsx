import { useTranslation } from 'react-i18next'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ConvStatusBar from '../components/ConvStatusBar'
import PermissionDialog from '../components/PermissionDialog'
import ModelSelector from '../components/ModelSelector'
import ModelPicker from '../components/ModelPicker'
import SessionModeSelector from '../components/SessionModeSelector'
import ContextStats from '../components/ContextStats'
import { BookOpenText } from 'lucide-react'
import type { PanelCtx, ConfigBarKind } from './types'
import styles from './ChatPanel.module.css'

interface ChatPanelProps {
  ctx: PanelCtx
  configBar: ConfigBarKind
  emptyTitleKey?: string
  emptyHintKey?: string
  placeholderKey?: string
  selectDocFirstKey?: string
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
}: ChatPanelProps) {
  const { t } = useTranslation()
  const isEmpty = ctx.messages.length === 0
  const conv = ctx.convState
  const disabled = ctx.sessionKind === 'docs' && !ctx.docTarget

  const placeholder = disabled && selectDocFirstKey
    ? t(selectDocFirstKey)
    : conv !== 'idle'
      ? t(`session.conv_${conv}`)
      : placeholderKey
        ? t(placeholderKey)
        : t('session.promptPlaceholder')

  const configBarNode = configBar !== 'none' ? (
    <div className={styles.configBar}>
      {configBar === 'coding' ? (
        <>
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
            <ContextStats messages={ctx.messages} />
          </div>
        </>
      ) : (
        // 文档模式：agent + 模型下拉
        <div className={styles.docConfigRow}>
          <div className={styles.configItem}>
            <label className={styles.configLabel}>Agent</label>
            <select
              className={styles.configSelect}
              value={ctx.selectedAgent}
              onChange={(e) => ctx.onSelectAgent(e.target.value)}
              disabled={conv !== 'idle'}
            >
              {ctx.agents.length === 0 && <option value="">{t('docMode.noAgent')}</option>}
              {ctx.agents.map((agent) => (
                <option key={agent.type} value={agent.type}>
                  {agent.display_name}
                </option>
              ))}
            </select>
          </div>
          {ctx.probeConfigs
            .filter((o) => o.type === 'select' && o.category === 'model' && o.options.length > 0)
            .map((opt) => (
              <div key={opt.id} className={styles.configItem}>
                <label className={styles.configLabel}>{t('docMode.model')}</label>
                <ModelPicker
                  value={ctx.selectedModel}
                  options={opt.options}
                  onChange={ctx.onSelectModel}
                  disabled={ctx.probing || conv !== 'idle'}
                  placeholder={t('session.selectModel')}
                />
              </div>
            ))}
        </div>
      )}
    </div>
  ) : null

  return (
    <div className={styles.chat}>
      {isEmpty && emptyTitleKey ? (
        <div className={styles.empty}>
          <BookOpenText size={36} className={styles.emptyIcon} />
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
        )}
        {configBarNode}
      </div>
    </div>
  )
}
