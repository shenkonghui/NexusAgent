import { useState } from 'react'
import DirectoryPicker from './DirectoryPicker'
import { X, Plus } from 'lucide-react'
import styles from './CreateWorkspaceDialog.module.css'

interface Props {
  onSubmit: (name: string, cwd: string, directories: string[]) => void
  onClose: () => void
  /** 编辑模式初始值（可选） */
  initialName?: string
  initialCwd?: string
  initialDirectories?: string[]
}

export default function CreateWorkspaceDialog({ onSubmit, onClose, initialName, initialCwd, initialDirectories }: Props) {
  const [name, setName] = useState(initialName || '')
  const [cwd, setCwd] = useState(initialCwd || '')
  const [directories, setDirectories] = useState<string[]>(initialDirectories || [])
  // showDirPicker: 'primary' 选择主目录, 'additional' 选择附加目录, null 关闭
  const [dirPickerMode, setDirPickerMode] = useState<'primary' | 'additional' | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim() && cwd.trim()) onSubmit(name.trim(), cwd.trim(), directories)
  }

  function addDirectory(path: string) {
    if (!directories.includes(path)) {
      setDirectories([...directories, path])
    }
  }

  function removeDirectory(path: string) {
    setDirectories(directories.filter(d => d !== path))
  }

  return (
    <div className={styles.overlay} onClick={onClose}>
      <form className={styles.dialog} onClick={e => e.stopPropagation()} onSubmit={handleSubmit}>
        <h3>{initialCwd ? '编辑工作区' : '新建工作区'}</h3>
        <input className={styles.input} type="text" value={name}
          onChange={e => setName(e.target.value)}
          placeholder="工作区名称" autoFocus required />

        {/* 主目录 */}
        <div className={styles.sectionLabel}>主目录（Primary）</div>
        <div className={styles.dirRow}>
          <input className={styles.input} type="text" value={cwd}
            onChange={e => setCwd(e.target.value)}
            placeholder="选择主工作目录" required readOnly style={{ marginBottom: 0 }} />
          <button type="button" className={styles.browseBtn}
            onClick={() => setDirPickerMode('primary')}>浏览</button>
        </div>

        {/* 附加目录 */}
        <div className={styles.sectionLabel}>附加目录（Secondary）</div>
        {directories.length > 0 && (
          <div className={styles.dirList}>
            {directories.map(d => (
              <div key={d} className={styles.dirItem}>
                <span className={styles.dirItemPath} title={d}>{d}</span>
                <button type="button" className={styles.dirItemRemove}
                  onClick={() => removeDirectory(d)}><X size={14} /></button>
              </div>
            ))}
          </div>
        )}
        <button type="button" className={styles.addDirBtn}
          onClick={() => setDirPickerMode('additional')}><Plus size={14} style={{ verticalAlign: '-2px' }} /> 添加附加目录</button>

        <div className={styles.actions}>
          <button type="button" onClick={onClose}>取消</button>
          <button type="submit" disabled={!name.trim() || !cwd.trim()}>
            {initialCwd ? '保存' : '创建'}
          </button>
        </div>
      </form>
      {dirPickerMode && (
        <DirectoryPicker
          initialPath={dirPickerMode === 'primary' ? cwd : undefined}
          onSelect={path => {
            if (dirPickerMode === 'primary') {
              setCwd(path)
            } else {
              addDirectory(path)
            }
            setDirPickerMode(null)
          }}
          onClose={() => setDirPickerMode(null)}
        />
      )}
    </div>
  )
}
