import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  getOrchestration, getOrchStatus, getOrchGitStatus, initOrchGitRepo,
  type OrchestrationDef, type OrchestrationTask,
} from '../api/orchestration'
import { sessionUrl, newTaskUrl } from '../utils/routes'
import type { Agent } from '../types'
import LoadingSpinner from './LoadingSpinner'
import OrchestrationChatPanel from './OrchestrationChatPanel'
import SplitPane from './SplitPane'
import styles from './OrchestrationView.module.css'
import { ChevronRight, ChevronDown, MessagesSquare, GitBranch } from 'lucide-react'

const ACTIVE_STATUSES = new Set(['queued', 'running'])

// 规范化后端返回的 def：确保 tasks 为数组（tasks.json 不存在/为空时后端可能省略 tasks 字段）。
function normalizeDef(d: OrchestrationDef | null | undefined): OrchestrationDef {
  const tasks = d?.tasks ?? []
  return { max_parallel: d?.max_parallel || 3, tasks }
}

interface Props {
  workspaceId: number | undefined
  cwd: string
  agents: Agent[]
  /** 从侧边栏点击编排对话记录进入时，指定需恢复的管理会话 DB 主键。 */
  restoreSessionId?: number
  onError: (message: string) => void
}

/**
 * OrchestrationView：编排模式主体（嵌入 ChatPage 编排模式，不走 LayoutRenderer）。
 * 左栏任务列表（含 git 检测/初始化提示、轮询），右栏 AI 管理对话（OrchestrationChatPanel）。
 * 逻辑与原独立编排页一致：点击任务打开其子会话；未运行任务则打开新建任务页预填详情。
 */
export default function OrchestrationView({ workspaceId, cwd, agents, restoreSessionId, onError }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [def, setDef] = useState<OrchestrationDef>({ max_parallel: 3, tasks: [] })
  const [loading, setLoading] = useState(true)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  // 展开/折叠的任务卡片 id 集合
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())
  // git 仓库状态：null=未知/加载中；true=是仓库；false=需初始化
  const [gitRepo, setGitRepo] = useState<boolean | null>(null)
  const [gitInitializing, setGitInitializing] = useState(false)

  // 检查当前工作目录是否为 git 仓库（编排任务需基于 worktree 隔离）
  useEffect(() => {
    if (!workspaceId) { setGitRepo(null); return }
    let alive = true
    getOrchGitStatus(workspaceId)
      .then((r) => { if (alive) setGitRepo(!!r.data.is_git_repo) })
      .catch(() => { if (alive) setGitRepo(null) })
    return () => { alive = false }
  }, [workspaceId])

  // 初始加载编排定义
  useEffect(() => {
    if (!workspaceId) { setLoading(false); return }
    let alive = true
    setLoading(true)
    getOrchestration(workspaceId)
      .then((r) => { if (alive) setDef(normalizeDef(r.data)) })
      .catch((e) => alive && onError(String((e as Error)?.message || e)))
      .finally(() => alive && setLoading(false))
    return () => { alive = false }
  }, [workspaceId, onError])

  // 有活跃任务时轮询状态（左侧列表实时反映运行状态）
  useEffect(() => {
    const hasActive = def.tasks.some((tk) => ACTIVE_STATUSES.has(tk.status))
    if (!hasActive || !workspaceId) {
      if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null }
      return
    }
    if (pollRef.current) return
    pollRef.current = setInterval(() => {
      getOrchStatus(workspaceId)
        .then((r) => setDef(normalizeDef(r.data)))
        .catch(() => {})
    }, 2000)
    return () => {
      if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null }
    }
  }, [def.tasks, workspaceId])

  async function reloadStatus() {
    if (!workspaceId) return
    try {
      const r = await getOrchStatus(workspaceId)
      setDef(normalizeDef(r.data))
    } catch { /* ignore */ }
  }

  function toggleExpand(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // 点击任务名称：打开任务界面（与普通任务界面一致，默认编程模式）。
  // 已运行的任务打开其关联会话；尚未运行的任务打开新建任务页并用任务详情预填 prompt。
  function openTask(task: OrchestrationTask) {
    if (task.db_session_id) {
      navigate(sessionUrl(task.db_session_id, workspaceId), { state: { taskMode: 'coding' } })
    } else {
      navigate(newTaskUrl(workspaceId), { state: { taskMode: 'coding', draftPrompt: task.detail } })
    }
  }

  // 初始化 git 仓库（含初始提交）并创建 .worktrees 目录，成功后刷新状态。
  async function handleGitInit() {
    if (!workspaceId || gitInitializing) return
    setGitInitializing(true)
    try {
      const r = await initOrchGitRepo(workspaceId)
      setGitRepo(!!r.data.is_git_repo)
    } catch (e) {
      onError(String((e as Error)?.message || e))
    } finally {
      setGitInitializing(false)
    }
  }

  if (loading) return <LoadingSpinner />

  return (
    <SplitPane dir="row" storageKey="orchestration" defaultFlexes={[1, 1]}>
      {/* 左栏：任务列表。点击卡片展开/收起查看详情（无操作按钮，操作走右栏 AI 对话）。 */}
      <div className={styles.taskCol}>
        <div className={styles.taskScroll}>
          {gitRepo === false ? (
            <div className={styles.gitPrompt}>
              <GitBranch size={32} className={styles.gitPromptIcon} />
              <h3 className={styles.gitPromptTitle}>{t('orchestration.gitRequiredTitle')}</h3>
              <p className={styles.gitPromptHint}>{t('orchestration.gitRequiredHint')}</p>
              {cwd && <code className={styles.gitPromptCwd}>{cwd}</code>}
              <button
                type="button"
                className={styles.gitInitBtn}
                onClick={handleGitInit}
                disabled={gitInitializing}
              >
                {gitInitializing ? t('orchestration.gitInitializing') : t('orchestration.gitInit')}
              </button>
            </div>
          ) : def.tasks.length === 0 ? (
            <div className={styles.empty}>{t('orchestration.empty')}</div>
          ) : (
            <div className={styles.taskList}>
              {def.tasks.map((task) => {
                const isOpen = expanded.has(task.id)
                return (
                  <div key={task.id} className={styles.taskCard}>
                    <div className={styles.taskHeader} onClick={() => toggleExpand(task.id)}>
                      <span className={styles.taskHeaderLeft}>
                        {isOpen
                          ? <ChevronDown size={14} className={styles.taskChevron} />
                          : <ChevronRight size={14} className={styles.taskChevron} />}
                        <span
                          className={styles.taskName}
                          role="button"
                          tabIndex={0}
                          title={t('orchestration.openTask')}
                          onClick={(e) => { e.stopPropagation(); openTask(task) }}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); openTask(task) }
                          }}
                        >{task.title}</span>
                      </span>
                      <span className={`${styles.taskStatus} ${styles[`status_${task.status}`] || ''}`}>
                        {t(`orchestration.status_${task.status}`)}
                      </span>
                    </div>
                    <div className={styles.taskMeta}>
                      <span className={styles.metaItem}>{task.id}</span>
                      {task.agent_type && <span className={styles.metaItem}>· {task.agent_type}</span>}
                      {task.branch && <span className={styles.metaItem}>· 🌿 {task.branch}</span>}
                      {task.model_value && <span className={styles.metaItem}>· 🤖 {task.model_value}</span>}
                    </div>
                    {isOpen && (
                      <div className={styles.taskBody}>
                        <div className={styles.taskDetail}>{task.detail}</div>
                        {task.worktree_path && (
                          <div className={styles.taskCwd}>
                            <span className={styles.cwdLabel}>{t('orchestration.cwd')}:</span>
                            <code className={styles.cwdValue}>{task.worktree_path}</code>
                          </div>
                        )}
                        {task.error && <div className={styles.taskError}>{task.error}</div>}
                        {task.started_at && (
                          <div className={styles.taskTime}>
                            {t('orchestration.startedAt')}: {new Date(task.started_at).toLocaleString()}
                            {task.finished_at && ` · ${t('orchestration.finishedAt')}: ${new Date(task.finished_at).toLocaleString()}`}
                          </div>
                        )}
                        {task.db_session_id && (
                          <button
                            type="button"
                            className={styles.openChatLink}
                            onClick={() => openTask(task)}
                            title={t('orchestration.openChat')}
                          >
                            <MessagesSquare size={13} style={{ verticalAlign: '-2px' }} /> {t('orchestration.openChat')}
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

      {/* 右栏：AI 管理对话（常驻，通过工具建/改/删任务、启停、调并发）。单任务在其独立会话页打开。 */}
      <div className={styles.chatCol}>
        {!workspaceId ? (
          <div className={styles.empty}>{t('orchestration.empty')}</div>
        ) : (
          <div className={styles.chatBody}>
            <OrchestrationChatPanel
              agents={agents}
              workspaceId={workspaceId}
              cwd={cwd}
              restoreSessionId={restoreSessionId}
              onTaskChanged={reloadStatus}
            />
          </div>
        )}
      </div>
    </SplitPane>
  )
}
