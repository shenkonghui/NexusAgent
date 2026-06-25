import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Navigate to="/login" replace />} />
          <Route path="/login" element={<div>登录页（待实现）</div>} />
          <Route path="/sessions" element={<div>会话列表页（待实现）</div>} />
          <Route path="/sessions/:id" element={<div>聊天页（待实现）</div>} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
