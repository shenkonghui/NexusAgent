import i18n from '../i18n'

// 内置默认标签的中英对照表。用户自定义的标签不在表内则原样返回。
// key 为中文，value 为英文，双向映射。
const DEFAULT_TAG_MAP: Record<string, string> = {
  后端: 'Backend',
  前端: 'Frontend',
  部署: 'Deploy',
  数据库: 'Database',
  文档: 'Docs',
  测试: 'Test',
  调研: 'Research',
  运维: 'DevOps',
  设计: 'Design',
  修复: 'Bugfix',
  重构: 'Refactor',
  优化: 'Optimize',
  安全: 'Security',
}

// 构建反向映射（英文 -> 中文），便于两种语言间互查
const REVERSE_MAP: Record<string, string> = Object.fromEntries(
  Object.entries(DEFAULT_TAG_MAP).map(([zh, en]) => [en, zh]),
)

// 根据当前界面语言翻译标签：中文环境下若存的是英文则翻成中文，反之亦然。
// 不在对照表内的标签原样返回。
export function translateTag(tag: string): string {
  const lang = i18n.language
  if (lang.startsWith('en')) {
    // 当前英文：把中文标签翻成英文
    return DEFAULT_TAG_MAP[tag] || tag
  }
  // 当前中文：把英文标签翻成中文
  return REVERSE_MAP[tag] || tag
}
