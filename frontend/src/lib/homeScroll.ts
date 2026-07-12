const SCROLL_KEY = 'tn_blog_home_scroll_y';
const NOTE_KEY = 'tn_blog_home_scroll_note';

/** Remember homepage scroll before entering a reader, so Back can restore it. */
export function rememberHomeScroll(noteId?: string) {
  try {
    sessionStorage.setItem(SCROLL_KEY, String(Math.max(0, Math.round(window.scrollY))));
    if (noteId) {
      sessionStorage.setItem(NOTE_KEY, noteId);
    } else {
      sessionStorage.removeItem(NOTE_KEY);
    }
  } catch {
    // private mode / quota
  }
}

export function peekHomeScroll(): { y: number; noteId: string } | null {
  try {
    const raw = sessionStorage.getItem(SCROLL_KEY);
    if (raw == null || raw === '') return null;
    const y = Number(raw);
    if (!Number.isFinite(y) || y < 0) return null;
    return { y, noteId: sessionStorage.getItem(NOTE_KEY) || '' };
  } catch {
    return null;
  }
}

export function clearHomeScroll() {
  try {
    sessionStorage.removeItem(SCROLL_KEY);
    sessionStorage.removeItem(NOTE_KEY);
  } catch {
    // ignore
  }
}

/** Restore once after home content is ready. Returns whether a restore ran. */
export function restoreHomeScroll(): boolean {
  const saved = peekHomeScroll();
  if (!saved) return false;
  clearHomeScroll();
  const y = saved.y;
  // Double rAF: wait for layout after notes grid paint.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      window.scrollTo(0, y);
    });
  });
  return true;
}
