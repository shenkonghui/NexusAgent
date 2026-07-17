import type { AgentPrefs, AgentPrefsPatch } from '../types'
import { apiFetch } from './client'

export function getAgentPrefs(): Promise<{ data: AgentPrefs }> {
  return apiFetch('/agent-prefs')
}

export function patchAgentPrefs(payload: AgentPrefsPatch): Promise<{ data: AgentPrefs }> {
  return apiFetch('/agent-prefs', {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}
