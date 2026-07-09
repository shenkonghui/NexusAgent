import { useEffect, useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { getSession } from '../api/sessions'
import { sessionUrl } from '../utils/routes'
import LoadingSpinner from './LoadingSpinner'

export default function SessionRedirect() {
  const { id } = useParams<{ id: string }>()
  const [target, setTarget] = useState<string | null>(null)

  useEffect(() => {
    const sid = Number(id)
    getSession(sid).then(r => {
      setTarget(sessionUrl(sid, r.data.workspace_id))
    }).catch(() => setTarget('/'))
  }, [id])

  if (!target) return <LoadingSpinner />
  return <Navigate to={target} replace />
}
