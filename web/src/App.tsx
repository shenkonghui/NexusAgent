import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'
import { ThemeProvider } from './context/ThemeContext'
import LoginPage from './pages/LoginPage'
import HomePage from './pages/HomePage'
import WorkspacePage from './pages/WorkspacePage'
import ChatPage from './pages/ChatPage'
import SettingsPage from './pages/SettingsPage'
import ScheduledTasksPage from './pages/ScheduledTasksPage'
import ProfilePage from './pages/ProfilePage'
import SessionRedirect from './components/SessionRedirect'

export default function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/workspaces/:wid" element={<WorkspacePage />} />
          <Route path="/workspaces/:wid/sessions/:sid" element={<ChatPage />} />
          {/* 旧路由兼容重定向 */}
          <Route path="/sessions/:id" element={<SessionRedirect />} />
          <Route path="/scheduled-tasks" element={<ScheduledTasksPage />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  )
}
