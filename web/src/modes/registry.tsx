import { Code2 } from 'lucide-react'
import type { ModeDef, PanelDef, LayoutNode } from './types'
import { leaf, split, tabs } from './types'
import { PANELS } from './panels'

export type { LayoutNode }

/** 面板注册表访问器（供 LayoutRenderer 查询） */
export function getPANELS(): PanelDef[] {
  return PANELS
}

/**
 * 模式注册表。新增一个模式只需 push 一条 + 对应 i18n key。
 * ChatPage 不需要改动——TaskModeSwitch 也从 MODES 自动读取选项。
 *
 * 示例：未来加"调试模式"
 *   MODES.push({
 *     id: 'debug', titleKey: 'taskMode.debug', icon: <Bug/>,
 *     sessionKind: 'primary', configBar: 'coding',
 *     layout: split('row', [
 *       leaf('chat', 1),
 *       split('col', [leaf('terminal',1), leaf('debug',1)]),
 *     ]),
 *   })
 */
export const MODES: ModeDef[] = [
  {
    id: 'coding',
    titleKey: 'taskMode.coding',
    icon: <Code2 size={14} />,
    sessionKind: 'primary',
    configBar: 'coding',
    layout: split('row', [
      // 中间列：上文件、下标签组（终端/改动/调试，默认终端）
      split(
        'col',
        [
          leaf('files', 1.2),
          tabs(['terminal', 'changes', 'debug'], 1, 'terminal'),
        ],
        1.3,
      ),
      // 右：AI 对话
      leaf('chat', 1),
    ]),
  },
]

/** 按 id 取模式定义；未注册时回退到首个模式（避免白屏） */
export function getMode(id: string | null | undefined): ModeDef {
  return MODES.find((m) => m.id === id) || MODES[0]
}

export const DEFAULT_MODE_ID = MODES[0].id
