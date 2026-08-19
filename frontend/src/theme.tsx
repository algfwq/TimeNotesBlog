import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ConfigProvider } from '@douyinfe/semi-ui';
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';

export type ThemeMode = 'light' | 'dark';

const COOKIE_KEY = 'tn_blog_theme';
const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;
const THEME_ANIM_MS = 520;

type ThemeContextValue = {
  mode: ThemeMode;
  source: 'system' | 'user';
  setMode: (mode: ThemeMode) => void;
  toggle: (origin?: { x: number; y: number }) => void;
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

function prefersReducedMotion(): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
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

function clearThemeAnimState() {
  const root = document.documentElement;
  root.classList.remove('theme-animating');
  document.body.classList.remove('theme-animating');
  delete root.dataset.themeTransition;
  root.style.removeProperty('--theme-reveal-x');
  root.style.removeProperty('--theme-reveal-y');
}

/**
 * 与 TimeNotes 客户端一致的圆形揭示主题切换：
 * 优先 View Transitions（从新主题画面以点击处为圆心展开），
 * 不支持时降级为短暂的色彩过渡。过程中抑制元素级 transition，避免抖动。
 */
function runThemeChange(next: ThemeMode, commit: () => void, origin?: { x: number; y: number }) {
  const root = document.documentElement;
  const apply = () => {
    applyThemeMode(next);
    commit();
  };

  const doc = document as Document & {
    startViewTransition?: (cb: () => void) => { finished: Promise<void> };
  };
  const canReveal = typeof doc.startViewTransition === 'function' && !prefersReducedMotion();

  if (canReveal) {
    try {
      const x = origin?.x ?? window.innerWidth - 60;
      const y = origin?.y ?? 60;
      root.style.setProperty('--theme-reveal-x', `${x}px`);
      root.style.setProperty('--theme-reveal-y', `${y}px`);
      root.dataset.themeTransition = next === 'dark' ? 'to-dark' : 'to-light';
      root.classList.add('theme-animating');
      const vt = doc.startViewTransition!(() => {
        apply();
      });
      void vt.finished.finally(clearThemeAnimState);
      return;
    } catch {
      clearThemeAnimState();
    }
  }

  if (prefersReducedMotion()) {
    apply();
    return;
  }

  // CSS fallback: brief class enables color/background transitions.
  root.classList.add('theme-animating');
  document.body.classList.add('theme-animating');
  // Force style flush so transitions run from previous theme values.
  void root.offsetWidth;
  apply();
  window.setTimeout(clearThemeAnimState, THEME_ANIM_MS);
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

  const toggle = useCallback((origin?: { x: number; y: number }) => {
    const next: ThemeMode = mode === 'dark' ? 'light' : 'dark';
    writeCookie(COOKIE_KEY, next);
    setSource('user');
    runThemeChange(next, () => setModeState(next), origin);
  }, [mode]);

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
