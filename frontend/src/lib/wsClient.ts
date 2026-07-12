import { solvePow, type PowProgress } from './pow';

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

type EventListener = (payload: unknown, env: Envelope) => void;

function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws`;
}

export class BlogWS {
  private ws: WebSocket | null = null;
  private pending = new Map<string, Pending>();
  private token = '';
  private openPromise: Promise<void> | null = null;
  private listeners = new Map<string, Set<EventListener>>();
  private reconnectTimer: number | null = null;
  private shouldReconnect = true;
  private snapshotHandlers = new Set<() => void | Promise<void>>();

  setToken(token: string) {
    this.token = token;
  }

  getToken() {
    return this.token;
  }

  on(type: string, listener: EventListener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type)!.add(listener);
    return () => this.off(type, listener);
  }

  off(type: string, listener: EventListener) {
    this.listeners.get(type)?.delete(listener);
  }

  onSnapshot(handler: () => void | Promise<void>) {
    this.snapshotHandlers.add(handler);
    return () => this.snapshotHandlers.delete(handler);
  }

  async connect(): Promise<void> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return;
    }
    if (this.openPromise) {
      return this.openPromise;
    }
    this.shouldReconnect = true;
    this.openPromise = new Promise((resolve, reject) => {
      const ws = new WebSocket(wsURL());
      this.ws = ws;
      ws.onopen = () => {
        this.openPromise = null;
        resolve();
        void this.afterOpen();
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
        this.scheduleReconnect();
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
            return;
          }
          const set = this.listeners.get(env.type);
          if (set) {
            for (const listener of set) {
              listener(env.payload, env);
            }
          }
          const all = this.listeners.get('*');
          if (all) {
            for (const listener of all) {
              listener(env.payload, env);
            }
          }
        } catch {
          // ignore malformed
        }
      };
    });
    return this.openPromise;
  }

  private scheduleReconnect() {
    if (!this.shouldReconnect || this.reconnectTimer != null) {
      return;
    }
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      void this.connect().catch(() => undefined);
    }, 1500);
  }

  private async afterOpen() {
    try {
      if (this.token) {
        await this.ensureSession(this.token);
      }
      await this.request('events.subscribe', {}).catch(() => undefined);
      for (const handler of this.snapshotHandlers) {
        await handler();
      }
    } catch {
      // reconnect path should not crash the socket
    }
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

  async login(
    username: string,
    password: string,
    options?: { onPowProgress?: (p: PowProgress) => void },
  ) {
    const challenge = await this.request<{ id: string; salt: string; difficulty: number }>(
      'auth.pow.challenge',
      {},
    );
    // Bound PoW is enforced server-side against this websocket session/IP.
    const nonce = await solvePow(challenge.salt, challenge.difficulty, {
      onProgress: options?.onPowProgress,
    });
    const result = await this.request<{
      token: string;
      userId: string;
      username: string;
      role: string;
      canUpload?: boolean;
      mustChangeCredentials?: boolean;
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
      canUpload?: boolean;
      mustChangeCredentials?: boolean;
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
    const result = await this.request<{
      userId: string;
      username: string;
      role: string;
      canUpload?: boolean;
      mustChangeCredentials?: boolean;
      expiresAt: number;
    }>('auth.session', { token: t });
    this.token = t;
    return result;
  }

  close() {
    this.shouldReconnect = false;
    if (this.reconnectTimer != null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.ws?.close();
    this.ws = null;
  }
}

export const blogWS = new BlogWS();
