import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useParams } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'
import { ThemeProvider } from './context/ThemeContext'
import { WORKSPACE_STORAGE_KEY } from './hooks/useCurrentWorkspace'
import LoginPage from './pages/LoginPage'
import ChatPage from './pages/ChatPage'
import SettingsPage from './pages/SettingsPage'
import ScheduledTasksPage from './pages/ScheduledTasksPage'
import NotesPage from './pages/NotesPage'
import ProfilePage from './pages/ProfilePage'
import SessionRedirect from './components/SessionRedirect'

function WorkspaceHomeRedirect() {
  const { wid } = useParams<{ wid: string }>()
  useEffect(() => {
    if (wid) localStorage.setItem(WORKSPACE_STORAGE_KEY, wid)
  }, [wid])
  return <Navigate to="/" replace />
}

export default function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter>
        <Routes>
          <Route path="/" element={<ChatPage />} />
          <Route path="/new" element={<ChatPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/workspaces/:wid" element={<WorkspaceHomeRedirect />} />
          <Route path="/workspaces/:wid/sessions/:sid" element={<ChatPage />} />
          <Route path="/sessions/:id" element={<SessionRedirect />} />
          <Route path="/scheduled-tasks" element={<ScheduledTasksPage />} />
          <Route path="/notes" element={<NotesPage />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  )
}
