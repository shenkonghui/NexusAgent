import { useRef, useEffect, useMemo } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { indentOnInput, bracketMatching, foldGutter, foldKeymap } from '@codemirror/language'
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search'
import { autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { lintKeymap } from '@codemirror/lint'
import { oneDark } from '@codemirror/theme-one-dark'
import { languages } from '@codemirror/language-data'

interface CodeEditorProps {
  value: string
  onChange?: (value: string) => void
  /** 文件路径，用于推断语言 */
  filePath?: string
  /** 是否只读 */
  readOnly?: boolean
}

// 根据文件扩展名推断语言（language-data 会自动匹配）
function langFromPath(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  const map: Record<string, string> = {
    go: 'go',
    ts: 'typescript',
    tsx: 'typescript-jsx',
    js: 'javascript',
    jsx: 'javascript',
    mjs: 'javascript',
    py: 'python',
    md: 'markdown',
    json: 'json',
    css: 'css',
    html: 'html',
    yaml: 'yaml',
    yml: 'yaml',
    sh: 'shell',
    bash: 'shell',
    sql: 'sql',
    rs: 'rust',
    java: 'java',
    c: 'c',
    cpp: 'cpp',
    xml: 'xml',
  }
  return map[ext] || ''
}

export default function CodeEditor({ value, onChange, filePath, readOnly }: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  // 语言扩展：使用 language-data 的 StreamLanguage 懒加载
  const langExtension = useMemo(() => {
    if (!filePath) return []
    const langName = langFromPath(filePath)
    if (!langName) return []
    // languages 是 LanguageDescription[]，通过 name/alias 匹配
    const desc = languages.find(
      (d) => d.name.toLowerCase() === langName || (d.alias && d.alias.includes(langName)),
    )
    if (desc && desc.support) return [desc.support]
    return []
  }, [filePath])

  useEffect(() => {
    if (!containerRef.current) return

    const updateListener = EditorView.updateListener.of((update) => {
      if (update.docChanged && onChangeRef.current) {
        onChangeRef.current(update.state.doc.toString())
      }
    })

    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        foldGutter(),
        history(),
        bracketMatching(),
        closeBrackets(),
        autocompletion(),
        indentOnInput(),
        highlightActiveLine(),
        highlightActiveLineGutter(),
        highlightSelectionMatches(),
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...searchKeymap,
          ...historyKeymap,
          ...foldKeymap,
          ...completionKeymap,
          ...lintKeymap,
        ]),
        oneDark,
        EditorView.lineWrapping,
        EditorState.readOnly.of(!!readOnly),
        updateListener,
        ...langExtension,
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // 仅在 filePath/readOnly 变化时重建编辑器
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filePath, readOnly, langExtension])

  // 外部 value 变化时更新编辑器内容（避免循环更新）
  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (current !== value) {
      view.dispatch({
        changes: { from: 0, to: current.length, insert: value },
      })
    }
  }, [value])

  return <div ref={containerRef} className="cm-container" style={{ height: '100%', overflow: 'hidden' }} />
}
