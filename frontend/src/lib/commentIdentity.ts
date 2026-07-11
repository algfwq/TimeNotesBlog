export type CommentIdentity = {
  nickname: string;
  email: string;
  githubUrl: string;
};

const COOKIE_NAME = 'tn_blog_comment_identity';
const ONE_YEAR_SECONDS = 365 * 24 * 60 * 60;

function parseCookie(name: string): string | null {
  const parts = document.cookie.split(';');
  for (const part of parts) {
    const [k, ...rest] = part.trim().split('=');
    if (k === name) {
      return decodeURIComponent(rest.join('='));
    }
  }
  return null;
}

export function readCommentIdentity(): CommentIdentity | null {
  try {
    const raw = parseCookie(COOKIE_NAME);
    if (!raw) return null;
    const data = JSON.parse(raw) as Partial<CommentIdentity>;
    const email = String(data.email || '').trim();
    const githubUrl = String(data.githubUrl || '').trim();
    let nickname = String(data.nickname || '').trim();
    if (githubUrl) {
      try {
        const u = new URL(githubUrl);
        if (!['github.com', 'www.github.com'].includes(u.hostname.toLowerCase())) {
          return null;
        }
        const user = u.pathname.split('/').filter(Boolean)[0] || '';
        if (!user) return null;
        if (!nickname) nickname = user;
        return { nickname, email: '', githubUrl: `https://github.com/${user}` };
      } catch {
        return null;
      }
    }
    if (!nickname || !email || !email.includes('@')) return null;
    return { nickname, email, githubUrl: '' };
  } catch {
    return null;
  }
}

export function writeCommentIdentity(identity: CommentIdentity) {
  const payload = encodeURIComponent(JSON.stringify({
    nickname: identity.nickname.trim(),
    email: identity.email.trim(),
    githubUrl: identity.githubUrl.trim(),
  }));
  document.cookie = `${COOKIE_NAME}=${payload}; Path=/; Max-Age=${ONE_YEAR_SECONDS}; SameSite=Lax`;
}

export function clearCommentIdentity() {
  document.cookie = `${COOKIE_NAME}=; Path=/; Max-Age=0; SameSite=Lax`;
}
