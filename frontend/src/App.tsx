import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { Toast } from '@douyinfe/semi-ui';
import { GlassBackground } from './components/GlassBackground';
import { HomePage } from './pages/HomePage';
import { ReaderPage } from './pages/ReaderPage';
import { AdminApp } from './pages/admin/AdminApp';

Toast.config({ duration: 2 });

function isAdminPath() {
  return location.pathname.includes('/admin/');
}

export default function App() {
  // Admin SPA is served under /admin/{token}/ ; use relative routing via basename detection.
  if (isAdminPath()) {
    const parts = location.pathname.split('/').filter(Boolean);
    // admin / {token} / ...
    const basenames = parts.length >= 2 ? `/${parts[0]}/${parts[1]}` : '/admin';
    return (
      <>
        <GlassBackground />
        <BrowserRouter basename={basenames}>
          <Routes>
            <Route path="/*" element={<AdminApp />} />
          </Routes>
        </BrowserRouter>
      </>
    );
  }

  return (
    <>
      <GlassBackground />
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/note/:id" element={<ReaderPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </>
  );
}
