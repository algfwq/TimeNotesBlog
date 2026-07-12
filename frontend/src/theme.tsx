import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ConfigProvider } from '@douyinfe/semi-ui';
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';

export type ThemeMode = 'light' | 'dark';

const COOKIE_KEY = 'tn_blog_theme';
const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

type ThemeContextValue = {
  mode: ThemeMode;
  source: 'system' | 'user';
  setMode: (mode: ThemeMode) => void;
  toggle: () => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function readCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

function writeCookie(name: string, value: string) {
  document.cookie = `${name}=${encodeURIComponent(value)}; path=/; max-age=${COOKIE_MAX_AGE}; SameSite=Lax`;
}

function systemPrefersDark(): boolean {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true;
}

function resolveInitial(): { mode: ThemeMode; source: 'system' | 'user' } {
  const saved = readCookie(COOKIE_KEY);
  if (saved === 'light' || saved === 'dark') {
    return { mode: saved, source: 'user' };
  }
  return { mode: systemPrefersDark() ? 'dark' : 'light', source: 'system' };
}

export function applyThemeMode(mode: ThemeMode) {
  document.body.setAttribute('theme-mode', mode);
  document.documentElement.setAttribute('theme-mode', mode);
  document.documentElement.style.colorScheme = mode;
  document.body.dataset.theme = mode;
}

/** @deprecated Use ThemeProvider + useTheme. Kept for transitional callers. */
export function applyDarkTheme() {
  applyThemeMode('dark');
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const initial = useMemo(() => resolveInitial(), []);
  const [mode, setModeState] = useState<ThemeMode>(initial.mode);
  const [source, setSource] = useState<'system' | 'user'>(initial.source);

  useEffect(() => {
    applyThemeMode(mode);
  }, [mode]);

  useEffect(() => {
    if (source !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => setModeState(mq.matches ? 'dark' : 'light');
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [source]);

  const setMode = useCallback((next: ThemeMode) => {
    writeCookie(COOKIE_KEY, next);
    setSource('user');
    setModeState(next);
  }, []);

  const toggle = useCallback(() => {
    setMode(mode === 'dark' ? 'light' : 'dark');
  }, [mode, setMode]);

  const value = useMemo(() => ({ mode, source, setMode, toggle }), [mode, source, setMode, toggle]);

  return (
    <ThemeContext.Provider value={value}>
      <ConfigProvider locale={zh_CN}>{children}</ConfigProvider>
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error('useTheme must be used within ThemeProvider');
  }
  return ctx;
}
