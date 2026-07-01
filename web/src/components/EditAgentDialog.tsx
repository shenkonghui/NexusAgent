import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { AgentConfig } from '../types'
import styles from './EditAgentDialog.module.css'

export interface AgentFormPayload {
  type: string
  display_name: string
  description: string
  command: string
  args: string[]
  api_key_env: string
  timeout: string
  enabled: boolean
}

interface Props {
  config: AgentConfig
  saving?: boolean
  onSave: (payload: AgentFormPayload) => void
  onDelete: () => void
  onClose: () => void
}

function toForm(config: AgentConfig) {
  return {
    type: config.type,
    display_name: config.display_name,
    description: config.description,
    command: config.command,
    args: (config.args || []).join('\n'),
    api_key_env: config.api_key_env,
    timeout: config.timeout,
    enabled: config.enabled,
  }
}

export default function EditAgentDialog({ config, saving = false, onSave, onDelete, onClose }: Props) {
  const { t } = useTranslation()
  const [form, setForm] = useState(() => toForm(config))

  useEffect(() => {
    setForm(toForm(config))
  }, [config])

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && !saving) onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose, saving])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.display_name.trim() || !form.command.trim()) return
    onSave({
      type: form.type.trim(),
      display_name: form.display_name.trim(),
      description: form.description.trim(),
      command: form.command.trim(),
      args: form.args.split('\n').map((s) => s.trim()).filter(Boolean),
      api_key_env: form.api_key_env.trim(),
      timeout: form.timeout.trim(),
      enabled: form.enabled,
    })
  }

  return (
    <div className={styles.overlay} onClick={() => { if (!saving) onClose() }}>
      <form className={styles.dialog} onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <div className={styles.header}>
          <h2 className={styles.title}>{t('settings.editTitle')}</h2>
          <button type="button" className={styles.closeBtn} onClick={onClose} disabled={saving} aria-label={t('common.close')}>×</button>
        </div>

        <div className={styles.grid}>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.type')} *</label>
            <input className={styles.input} value={form.type} disabled />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.displayName')} *</label>
            <input className={styles.input} value={form.display_name} autoFocus
              onChange={(e) => setForm({ ...form, display_name: e.target.value })}
              placeholder="Claude Code"
            />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.command')} *</label>
            <input className={styles.input} value={form.command}
              onChange={(e) => setForm({ ...form, command: e.target.value })}
              placeholder="npx / codebuddy"
            />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.apiKeyEnv')}</label>
            <input className={styles.input} value={form.api_key_env}
              onChange={(e) => setForm({ ...form, api_key_env: e.target.value })}
              placeholder="ANTHROPIC_API_KEY"
            />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.timeout')}</label>
            <input className={styles.input} value={form.timeout}
              onChange={(e) => setForm({ ...form, timeout: e.target.value })}
              placeholder="300s"
            />
          </div>
          <div className={styles.field}>
            <label className={styles.label}>{t('settings.description')}</label>
            <input className={styles.input} value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder="Anthropic Claude Code agent"
            />
          </div>
        </div>

        <div className={styles.field}>
          <label className={styles.label}>{t('settings.args')}</label>
          <textarea className={styles.textarea} rows={3} value={form.args}
            onChange={(e) => setForm({ ...form, args: e.target.value })}
            placeholder={'-y\n@agentclientprotocol/claude-agent-acp@latest'}
          />
        </div>

        <label className={styles.checkbox}>
          <input type="checkbox" checked={form.enabled}
            onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
          /> {t('settings.enabled')}
        </label>

        <div className={styles.actions}>
          <button type="button" className={styles.deleteBtn} disabled={saving} onClick={onDelete}>
            {t('common.delete')}
          </button>
          <button type="button" className={styles.cancelBtn} disabled={saving} onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button className={styles.submitBtn} type="submit" disabled={saving}>
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </form>
    </div>
  )
}
