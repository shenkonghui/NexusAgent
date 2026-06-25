import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'
import LoginPage from './pages/LoginPage'
import SessionsPage from './pages/SessionsPage'
import ChatPage from './pages/ChatPage'
import SettingsPage from './pages/SettingsPage'
import ScheduledTasksPage from './pages/ScheduledTasksPage'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* 首页即会话列表 */}
          <Route path="/" element={<SessionsPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/sessions" element={<Navigate to="/" replace />} />
          <Route path="/sessions/:id" element={<ChatPage />} />
          <Route path="/scheduled-tasks" element={<ScheduledTasksPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          {/* 404 路由：未匹配的路径重定向到登录页 */}
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
