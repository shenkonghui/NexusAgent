import { useState, type FormEvent } from 'react'
import styles from './PromptInput.module.css'

interface PromptInputProps {
  onSend: (prompt: string) => void
  onCancel?: () => void
  disabled?: boolean
  sending?: boolean
  placeholder?: string
}

export default function PromptInput({
  onSend,
  onCancel,
  disabled = false,
  sending = false,
  placeholder = '输入 prompt...',
}: PromptInputProps) {
  const [text, setText] = useState('')

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const trimmed = text.trim()
    if (!trimmed || disabled || sending) return
    onSend(trimmed)
    setText('')
  }

  return (
    <form className={styles.container} onSubmit={handleSubmit}>
      <textarea
        className={styles.input}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={placeholder}
        disabled={disabled || sending}
        rows={1}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            handleSubmit(e)
          }
        }}
      />
      {sending && onCancel ? (
        <button className={styles.cancelBtn} type="button" onClick={onCancel}>
          取消
        </button>
      ) : (
        <button
          className={styles.sendBtn}
          type="submit"
          disabled={disabled || sending || !text.trim()}
        >
          发送
        </button>
      )}
    </form>
  )
}
