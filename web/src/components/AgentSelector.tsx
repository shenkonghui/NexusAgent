import type { Agent } from '../types'
import styles from './AgentSelector.module.css'

interface AgentSelectorProps {
  agents: Agent[]
  value: string
  onChange: (type: string) => void
}

export default function AgentSelector({ agents, value, onChange }: AgentSelectorProps) {
  return (
    <div className={styles.container}>
      <label className={styles.label}>Agent 类型</label>
      <select
        className={styles.select}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {agents.length === 0 && <option value="">无可用 Agent</option>}
        {agents.map((agent) => (
          <option key={agent.type} value={agent.type}>
            {agent.display_name}
          </option>
        ))}
      </select>
    </div>
  )
}
