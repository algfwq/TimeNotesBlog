import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { Toast } from '@douyinfe/semi-ui';
import { GlassBackground } from './components/GlassBackground';
import { HomePage } from './pages/HomePage';
import { ReaderPage } from './pages/ReaderPage';
import { AdminApp } from './pages/admin/AdminApp';

Toast.config({ duration: 2 });

function adminBasename(): string | null {
  // /admin/{token} or /admin/{token}/...
  const m = location.pathname.match(/^\/admin\/([^/]+)/);
  if (!m) {
    return null;
  }
  return `/admin/${m[1]}`;
}

export default function App() {
  const adminBase = adminBasename();
  if (adminBase) {
    return (
      <>
        <GlassBackground />
        <BrowserRouter basename={adminBase}>
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
