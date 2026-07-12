import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ConfigProvider } from '@douyinfe/semi-ui';
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';

export type ThemeMode = 'light' | 'dark';

const COOKIE_KEY = 'tn_blog_theme';
const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;
const THEME_ANIM_MS = 420;

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

function clearThemeAnimClass() {
  document.documentElement.classList.remove('theme-animating');
  document.body.classList.remove('theme-animating');
}

/** Cross-fade / view-transition between light and dark. */
function runThemeChange(next: ThemeMode, commit: () => void) {
  const root = document.documentElement;
  const apply = () => {
    applyThemeMode(next);
    commit();
  };

  // Prefer native View Transitions when available (Chrome/Edge).
  const doc = document as Document & {
    startViewTransition?: (cb: () => void) => { finished: Promise<void> };
  };
  if (typeof doc.startViewTransition === 'function') {
    try {
      root.classList.add('theme-animating');
      const vt = doc.startViewTransition(() => {
        apply();
      });
      void vt.finished.finally(() => {
        clearThemeAnimClass();
      });
      return;
    } catch {
      // fall through
    }
  }

  // CSS fallback: brief class enables color/background transitions.
  root.classList.add('theme-animating');
  document.body.classList.add('theme-animating');
  // Force style flush so transitions run from previous theme values.
  void root.offsetWidth;
  apply();
  window.setTimeout(clearThemeAnimClass, THEME_ANIM_MS);
}

/** @deprecated Use ThemeProvider + useTheme. Kept for transitional callers. */
export function applyDarkTheme() {
  applyThemeMode('dark');
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const initial = useMemo(() => resolveInitial(), []);
  const [mode, setModeState] = useState<ThemeMode>(initial.mode);
  const [source, setSource] = useState<'system' | 'user'>(initial.source);

  // Initial mount: apply without animation.
  useEffect(() => {
    applyThemeMode(mode);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (source !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => {
      const next: ThemeMode = mq.matches ? 'dark' : 'light';
      runThemeChange(next, () => setModeState(next));
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [source]);

  const setMode = useCallback((next: ThemeMode) => {
    writeCookie(COOKIE_KEY, next);
    setSource('user');
    if (next === mode) {
      applyThemeMode(next);
      return;
    }
    runThemeChange(next, () => setModeState(next));
  }, [mode]);

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
