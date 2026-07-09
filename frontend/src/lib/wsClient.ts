import { solvePow } from './pow';

export type Envelope = {
  v: number;
  type: string;
  id?: string;
  payload?: unknown;
  error?: { code: string; message: string };
};

type Pending = {
  resolve: (env: Envelope) => void;
  reject: (err: Error) => void;
  timer: number;
};

function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws`;
}

export class BlogWS {
  private ws: WebSocket | null = null;
  private pending = new Map<string, Pending>();
  private token = '';
  private openPromise: Promise<void> | null = null;

  setToken(token: string) {
    this.token = token;
  }

  getToken() {
    return this.token;
  }

  async connect(): Promise<void> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return;
    }
    if (this.openPromise) {
      return this.openPromise;
    }
    this.openPromise = new Promise((resolve, reject) => {
      const ws = new WebSocket(wsURL());
      this.ws = ws;
      ws.onopen = () => {
        this.openPromise = null;
        resolve();
      };
      ws.onerror = () => {
        this.openPromise = null;
        reject(new Error('WebSocket connection failed'));
      };
      ws.onclose = () => {
        this.ws = null;
        for (const [, p] of this.pending) {
          window.clearTimeout(p.timer);
          p.reject(new Error('connection closed'));
        }
        this.pending.clear();
      };
      ws.onmessage = (ev) => {
        try {
          const env = JSON.parse(String(ev.data)) as Envelope;
          if (env.id && this.pending.has(env.id)) {
            const p = this.pending.get(env.id)!;
            this.pending.delete(env.id);
            window.clearTimeout(p.timer);
            if (env.error) {
              p.reject(new Error(env.error.message || env.error.code));
            } else {
              p.resolve(env);
            }
          }
        } catch {
          // ignore malformed
        }
      };
    });
    return this.openPromise;
  }

  async request<T = unknown>(type: string, payload?: unknown, timeoutMs = 30000): Promise<T> {
    await this.connect();
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('not connected');
    }
    const id = crypto.randomUUID();
    const env: Envelope = { v: 1, type, id, payload };
    return new Promise<T>((resolve, reject) => {
      const timer = window.setTimeout(() => {
        this.pending.delete(id);
        reject(new Error('request timeout'));
      }, timeoutMs);
      this.pending.set(id, {
        resolve: (resp) => resolve((resp.payload ?? null) as T),
        reject,
        timer,
      });
      this.ws!.send(JSON.stringify(env));
    });
  }

  async login(username: string, password: string) {
    const challenge = await this.request<{ id: string; salt: string; difficulty: number }>(
      'auth.pow.challenge',
      {},
    );
    const nonce = await solvePow(challenge.salt, challenge.difficulty);
    const result = await this.request<{
      token: string;
      userId: string;
      username: string;
      role: string;
      canUpload?: boolean;
      expiresAt: number;
    }>('auth.login', {
      username,
      password,
      challengeId: challenge.id,
      nonce,
    });
    this.token = result.token;
    await this.request('auth.session', { token: result.token });
    return result;
  }

  async loginWithToken(token: string) {
    const result = await this.request<{
      token: string;
      userId: string;
      username: string;
      role: string;
      expiresAt: number;
    }>('auth.login', { token });
    this.token = result.token || token;
    return result;
  }

  async ensureSession(token?: string) {
    const t = token || this.token;
    if (!t) {
      throw new Error('no token');
    }
    await this.request('auth.session', { token: t });
    this.token = t;
  }
}

export const blogWS = new BlogWS();
