import { useEffect, useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { getSession } from '../api/sessions'
import LoadingSpinner from './LoadingSpinner'

export default function SessionRedirect() {
  const { id } = useParams<{ id: string }>()
  const [target, setTarget] = useState<string | null>(null)

  useEffect(() => {
    getSession(Number(id)).then(r => {
      const wid = r.data.workspace_id
      if (wid) setTarget(`/workspaces/${wid}/sessions/${id}`)
      else setTarget('/')
    }).catch(() => setTarget('/'))
  }, [id])

  if (!target) return <LoadingSpinner />
  return <Navigate to={target} replace />
}
