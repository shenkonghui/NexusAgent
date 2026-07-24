import { useRef, useEffect, useMemo } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { indentOnInput, bracketMatching, foldGutter, foldKeymap } from '@codemirror/language'
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search'
import { autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { lintKeymap } from '@codemirror/lint'
import { oneDark } from '@codemirror/theme-one-dark'
import type { Extension } from '@codemirror/state'
import { go } from '@codemirror/lang-go'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { markdown } from '@codemirror/lang-markdown'
import { python } from '@codemirror/lang-python'
import { yaml } from '@codemirror/lang-yaml'
import { sql } from '@codemirror/lang-sql'
import { rust } from '@codemirror/lang-rust'
import { java } from '@codemirror/lang-java'
import { cpp } from '@codemirror/lang-cpp'
import { xml } from '@codemirror/lang-xml'

interface CodeEditorProps {
  value: string
  onChange?: (value: string) => void
  /** 文件路径，用于推断语言 */
  filePath?: string
  /** 是否只读 */
  readOnly?: boolean
}

// 根据文件扩展名同步返回对应的语言扩展（直接使用 @codemirror/lang-* 包，无需异步加载）
function langFromPath(path: string): Extension | null {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  switch (ext) {
    case 'go':
      return go()
    case 'ts':
      return javascript({ typescript: true })
    case 'tsx':
      return javascript({ typescript: true, jsx: true })
    case 'js':
    case 'mjs':
      return javascript()
    case 'jsx':
      return javascript({ jsx: true })
    case 'py':
      return python()
    case 'md':
      return markdown()
    case 'json':
      return json()
    case 'css':
      return css()
    case 'html':
      return html()
    case 'yaml':
    case 'yml':
      return yaml()
    case 'sql':
      return sql()
    case 'rs':
      return rust()
    case 'java':
      return java()
    case 'c':
    case 'cpp':
    case 'cc':
    case 'h':
    case 'hpp':
      return cpp()
    case 'xml':
      return xml()
    default:
      return null
  }
}

export default function CodeEditor({ value, onChange, filePath, readOnly }: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  // 语言扩展：按扩展名同步匹配对应的 LanguageSupport
  const langExtension = useMemo<Extension[]>(() => {
    if (!filePath) return []
    const lang = langFromPath(filePath)
    return lang ? [lang] : []
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
