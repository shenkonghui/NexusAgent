import { useState } from 'react'
import DirectoryPicker from './DirectoryPicker'
import styles from './CreateWorkspaceDialog.module.css'

interface Props {
  onSubmit: (name: string, cwd: string) => void
  onClose: () => void
}

export default function CreateWorkspaceDialog({ onSubmit, onClose }: Props) {
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState('')
  const [showDirPicker, setShowDirPicker] = useState(false)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim() && cwd.trim()) onSubmit(name.trim(), cwd.trim())
  }

  return (
    <div className={styles.overlay} onClick={onClose}>
      <form className={styles.dialog} onClick={e => e.stopPropagation()} onSubmit={handleSubmit}>
        <h3>新建工作区</h3>
        <input className={styles.input} type="text" value={name}
          onChange={e => setName(e.target.value)}
          placeholder="工作区名称" autoFocus required />
        <div className={styles.dirRow}>
          <input className={styles.input} type="text" value={cwd}
            onChange={e => setCwd(e.target.value)}
            placeholder="选择工作目录" required readOnly />
          <button type="button" className={styles.browseBtn}
            onClick={() => setShowDirPicker(true)}>浏览</button>
        </div>
        <div className={styles.actions}>
          <button type="button" onClick={onClose}>取消</button>
          <button type="submit" disabled={!name.trim() || !cwd.trim()}>创建</button>
        </div>
      </form>
      {showDirPicker && (
        <DirectoryPicker
          initialPath={cwd}
          onSelect={path => { setCwd(path); setShowDirPicker(false) }}
          onClose={() => setShowDirPicker(false)}
        />
      )}
    </div>
  )
}
