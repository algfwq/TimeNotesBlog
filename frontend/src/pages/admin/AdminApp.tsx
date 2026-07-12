import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  Button, Checkbox, ColorPicker, Empty, Input, Modal, Progress, Select, Slider, Switch, Tag,
  Table, Toast, Typography, Upload,
} from '@douyinfe/semi-ui';
import {
  IconBook, IconCloud, IconComment, IconDelete, IconDownload, IconEyeOpened,
  IconHistogram, IconLikeThumb, IconLock, IconExit, IconSetting, IconUser, IconHome,
} from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { blogWS } from '../../lib/wsClient';
import type { PowProgress } from '../../lib/pow';
import { useTheme } from '../../theme';
import { ThemeToggle } from '../../components/ThemeToggle';
import { AdminCredentialMigrationModal } from '../../components/AdminCredentialMigrationModal';
import { WorldMapChart } from '../../components/WorldMapChart';
import { defaultSiteSettings, isHeroVideo, resolveHeroBackground, type SiteSettings } from '../../types/site';
import logoUrl from '../../assets/timenotes-logo.png';
import './admin.css';

type User = {
  id: string;
  username: string;
  role: string;
  canUpload: boolean;
  mustChangeCredentials?: boolean;
};

type Note = {
  id: string;
  title: string;
  filename: string;
  ownerName?: string;
  visible: boolean;
  publicDownload?: boolean;
  likeCount: number;
  commentCount: number;
  sizeBytes: number;
  updatedAt: string;
  coverUrl?: string;
};

type Stats = {
  todayCount: number;
  recentCount: number;
  daily: Array<{ date: string; count: number }>;
  locations: Array<{ country: string; region: string; city: string; lat: number; lng: number; count: number }>;
  countries?: Array<{ country: string; count: number }>;
  noteStats: Array<{ noteId: string; title: string; likeCount: number; commentCount: number; visible: boolean }>;
};

const TOKEN_KEY = 'tn_blog_admin_token';

const NAV_ITEMS: Array<{ key: string; label: string; desc: string; icon: ReactNode }> = [
  { key: 'dash', label: '仪表盘', desc: '访问趋势、地图与互动概览', icon: <IconHistogram /> },
  { key: 'notes', label: '手账管理', desc: '上传、可见性、公开下载与客户端编辑', icon: <IconCloud /> },
  { key: 'users', label: '用户管理', desc: '角色、上传权限与所有权转移', icon: <IconUser /> },
  { key: 'site', label: '站点外观', desc: 'Hero 标题、背景与遮罩实时发布', icon: <IconHome /> },
  { key: 'account', label: '账号安全', desc: '修改当前管理员用户名与密码', icon: <IconSetting /> },
];

function chartColors(isDark: boolean) {
  return {
    background: 'transparent',
    color: isDark ? ['#7dd3fc', '#a78bfa', '#34d399', '#fbbf24'] : ['#2563eb', '#7c3aed', '#059669', '#d97706'],
    titleFill: isDark ? '#f4f1ea' : '#1f2430',
    labelFill: isDark ? 'rgba(244,241,234,0.7)' : 'rgba(31,36,48,0.65)',
    grid: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.08)',
    domain: isDark ? 'rgba(255,255,255,0.15)' : 'rgba(15,23,42,0.15)',
  };
}

export function AdminApp() {
  const { mode } = useTheme();
  const isDark = mode === 'dark';
  const colors = chartColors(isDark);

  const [token, setToken] = useState(localStorage.getItem(TOKEN_KEY) || '');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [authed, setAuthed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [users, setUsers] = useState<User[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [settings, setSettings] = useState<SiteSettings>(defaultSiteSettings());
  const [settingsDraft, setSettingsDraft] = useState<SiteSettings>(defaultSiteSettings());
  const [savingSettings, setSavingSettings] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'user', canUpload: true });
  const [mustChange, setMustChange] = useState(false);
  const [migrating, setMigrating] = useState(false);
  const [sessionUser, setSessionUser] = useState<User | null>(null);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [editForm, setEditForm] = useState({ username: '', password: '', role: 'user', canUpload: true });
  const [deleteUser, setDeleteUser] = useState<User | null>(null);
  const [transferTo, setTransferTo] = useState('');
  const [navKey, setNavKey] = useState('dash');
  const [selfForm, setSelfForm] = useState({ username: '', password: '', password2: '' });
  const [selfSaving, setSelfSaving] = useState(false);
  const [powProgress, setPowProgress] = useState<PowProgress | null>(null);
  const [uploadingHero, setUploadingHero] = useState(false);

  const mapError = (e: unknown) => {
    const msg = String(e);
    if (msg.includes('thumbnail_required')) return '请先用 TimeNotes 客户端打开该手账生成封面后再上传';
    if (msg.includes('last_admin')) return '不能删除/降级最后一名管理员';
    if (msg.includes('transfer_target_required')) return '该用户仍有手账，请先选择接收管理员';
    if (msg.includes('credentials_required')) return '请先完成默认管理员凭据修改';
    if (msg.includes('bridge_unavailable')) return '未检测到本机 TimeNotes 客户端';
    if (msg.includes('capability_expired')) return '编辑授权已过期，请重试';
    if (msg.includes('rate_limited')) return '操作过于频繁，请稍后再试';
    return msg;
  };

  const applySession = (res: { token?: string; username: string; role: string; canUpload?: boolean; mustChangeCredentials?: boolean; userId?: string }) => {
    if (res.token) {
      localStorage.setItem(TOKEN_KEY, res.token);
      setToken(res.token);
      blogWS.setToken(res.token);
    }
    setSessionUser({
      id: res.userId || 'self',
      username: res.username,
      role: res.role,
      canUpload: Boolean(res.canUpload),
      mustChangeCredentials: Boolean(res.mustChangeCredentials),
    });
    setSelfForm((s) => ({ ...s, username: res.username }));
    setMustChange(Boolean(res.mustChangeCredentials));
    setAuthed(true);
  };

  const login = async () => {
    setBusy(true);
    const startedAt = Date.now();
    setPowProgress({ attempts: 0, difficulty: 0, percent: 2, status: 'solving' });
    try {
      await blogWS.connect();
      const res = await blogWS.login(username, password, {
        onPowProgress: (p) => setPowProgress(p),
      });
      if (res.role !== 'admin') throw new Error('需要管理员账号');
      setPowProgress((p) => ({
        attempts: p?.attempts ?? 0,
        difficulty: p?.difficulty ?? 0,
        percent: 100,
        status: 'done',
      }));
      // Keep the progress panel visible long enough for users to notice (easy challenges finish in ms).
      const remain = Math.max(0, 700 - (Date.now() - startedAt));
      if (remain > 0) {
        await new Promise((r) => window.setTimeout(r, remain));
      }
      applySession(res);
      Toast.success('登录成功');
      if (!res.mustChangeCredentials) await refreshAll();
    } catch (e) {
      setPowProgress((p) => (p ? { ...p, status: 'error' } : { attempts: 0, difficulty: 0, percent: 0, status: 'error' }));
      Toast.error(mapError(e));
    } finally {
      setBusy(false);
      window.setTimeout(() => setPowProgress(null), 400);
    }
  };

  const ensureAuth = async () => {
    if (!token) return;
    try {
      await blogWS.connect();
      const res = await blogWS.loginWithToken(token);
      if (res.role !== 'admin') throw new Error('需要管理员账号');
      applySession({ ...res, token });
      if (!res.mustChangeCredentials) await refreshAll();
    } catch {
      localStorage.removeItem(TOKEN_KEY);
      setToken('');
      setAuthed(false);
      setMustChange(false);
      setSessionUser(null);
    }
  };

  const refreshAll = async () => {
    const [u, n, s, site] = await Promise.all([
      blogWS.request<{ users: User[] }>('admin.users.list', {}),
      blogWS.request<{ notes: Note[] }>('admin.notes.list', {}),
      blogWS.request<Stats>('admin.stats', {}),
      blogWS.request<{ settings: SiteSettings }>('admin.site.get', {}).catch(() => ({ settings: defaultSiteSettings() })),
    ]);
    setUsers(u.users || []);
    setNotes(n.notes || []);
    setStats(s);
    const next = { ...defaultSiteSettings(), ...(site.settings || {}) };
    setSettings(next);
    setSettingsDraft(next);
  };

  useEffect(() => {
    void ensureAuth();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!authed || mustChange) return;
    const unsubs = [
      blogWS.on('event.note.changed', () => { void refreshAll(); }),
      blogWS.on('event.note.deleted', () => { void refreshAll(); }),
      blogWS.on('event.user.changed', () => { void refreshAll(); }),
      blogWS.on('event.stats.changed', () => { void refreshAll(); }),
      blogWS.on('event.like.changed', () => { void refreshAll(); }),
      blogWS.on('event.comment.created', () => { void refreshAll(); }),
      blogWS.on('event.site-settings.changed', (payload) => {
        const p = payload as { settings?: SiteSettings };
        if (p.settings) {
          const next = { ...defaultSiteSettings(), ...p.settings };
          setSettings(next);
          setSettingsDraft(next);
        }
      }),
      blogWS.onSnapshot(async () => { if (authed && !mustChange) await refreshAll(); }),
    ];
    return () => unsubs.forEach((u) => u());
  }, [authed, mustChange]);

  const migrateCredentials = async (name: string, pass: string) => {
    setMigrating(true);
    try {
      const res = await blogWS.request<{ ok: boolean; username: string; mustChangeCredentials?: boolean }>('admin.self.update', {
        username: name,
        password: pass,
      });
      setMustChange(Boolean(res.mustChangeCredentials));
      setSessionUser((u) => (u ? { ...u, username: res.username, mustChangeCredentials: Boolean(res.mustChangeCredentials) } : u));
      Toast.success('凭据已更新');
      await refreshAll();
    } catch (e) {
      Toast.error(mapError(e));
      throw e;
    } finally {
      setMigrating(false);
    }
  };

  const downloadNote = async (note: Note) => {
    try {
      const res = await blogWS.request<{ downloadUrl: string; filename: string }>('admin.notes.download', { id: note.id });
      const url = res.downloadUrl.startsWith('http') ? res.downloadUrl : `${location.origin}${res.downloadUrl}`;
      window.location.href = url;
    } catch (e) {
      Toast.error(mapError(e));
    }
  };

  const visitSpec = useMemo(() => {
    const values = (stats?.daily || []).map((d) => ({ date: d.date, count: d.count }));
    return {
      type: 'area',
      data: [{ id: 'daily', values: values.length ? values : [{ date: '-', count: 0 }] }],
      xField: 'date',
      yField: 'count',
      title: { text: '近 14 日访问量', textStyle: { fill: colors.titleFill, fontSize: 14, fontWeight: 600 } },
      background: 'transparent',
      color: colors.color,
      area: { style: { fillOpacity: 0.25 } },
      line: { style: { stroke: colors.color[0], lineWidth: 2 } },
      point: { style: { fill: colors.color[0], lineWidth: 1 } },
      axes: [
        {
          orient: 'left',
          label: { style: { fill: colors.labelFill } },
          grid: { style: { stroke: colors.grid } },
          domainLine: { style: { stroke: colors.domain } },
        },
        {
          orient: 'bottom',
          label: { style: { fill: colors.labelFill } },
          grid: { visible: false },
          domainLine: { style: { stroke: colors.domain } },
        },
      ],
    };
  }, [stats, colors]);

  const uploadFile = async (file: File) => {
    const buf = await file.arrayBuffer();
    const bytes = new Uint8Array(buf);
    const hashBuf = await crypto.subtle.digest('SHA-256', bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer);
    const sha256 = [...new Uint8Array(hashBuf)].map((b) => b.toString(16).padStart(2, '0')).join('');
    const start = await blogWS.request<{ uploadId: string; chunkSize: number }>('admin.notes.upload.start', {
      filename: file.name,
      title: file.name.replace(/\.tnote$/i, ''),
      size: bytes.length,
    });
    const chunkSize = start.chunkSize || 256 * 1024;
    for (let offset = 0, index = 0; offset < bytes.length; offset += chunkSize, index++) {
      const slice = bytes.subarray(offset, Math.min(offset + chunkSize, bytes.length));
      let binary = '';
      for (let i = 0; i < slice.length; i++) binary += String.fromCharCode(slice[i]);
      const data = btoa(binary);
      await blogWS.request('admin.notes.upload.chunk', {
        uploadId: start.uploadId,
        index,
        data,
      }, 120000);
    }
    await blogWS.request('admin.notes.upload.finish', {
      uploadId: start.uploadId,
      sha256,
    }, 120000);
    Toast.success('上传成功');
    await refreshAll();
  };

  const openInClient = async (note: Note) => {
    try {
      const res = await blogWS.request<{ downloadUrl?: string; filename?: string }>('admin.notes.download', { id: note.id });
      const url = res.downloadUrl || '';
      if (!url) throw new Error('download unavailable');
      const absolute = url.startsWith('http') ? url : `${location.origin}${url}`;

      const pairResp = await fetch('http://127.0.0.1:54088/api/blog-bridge/pair-token');
      if (!pairResp.ok) throw new Error('bridge_unavailable');
      const pair = await pairResp.json() as { token?: string };
      if (!pair.token) throw new Error('bridge_unavailable');

      const capResp = await fetch('http://127.0.0.1:54088/api/blog-bridge/capabilities', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-TimeNotes-Pair-Token': pair.token,
        },
        body: JSON.stringify({
          noteId: note.id,
          downloadUrl: absolute,
          filename: res.filename || note.filename,
        }),
      });
      if (!capResp.ok) throw new Error((await capResp.text()) || 'bridge_unavailable');
      const cap = await capResp.json() as { capabilityId?: string };
      if (!cap.capabilityId) throw new Error('capability_expired');

      const openResp = await fetch('http://127.0.0.1:54088/api/blog-bridge/open', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-TimeNotes-Pair-Token': pair.token,
        },
        body: JSON.stringify({ capabilityId: cap.capabilityId }),
      });
      if (!openResp.ok) throw new Error((await openResp.text()) || 'bridge_unavailable');
      Toast.success('已请求客户端打开');
    } catch (e) {
      Toast.error(`打开失败，请确认 TimeNotes 客户端已启动：${mapError(e)}`);
    }
  };

  const handleCreateUser = async () => {
    const name = newUser.username.trim();
    const pass = newUser.password;
    const role = newUser.role === 'admin' ? 'admin' : 'user';
    if (!name) { Toast.warning('请填写用户名'); return; }
    if (!pass) { Toast.warning('请填写密码'); return; }
    setCreating(true);
    try {
      await blogWS.request('admin.users.create', {
        username: name,
        password: pass,
        role,
        canUpload: role === 'admin' ? true : newUser.canUpload,
      });
      Toast.success(role === 'admin' ? '管理员已创建' : '用户已创建');
      setCreateOpen(false);
      setNewUser({ username: '', password: '', role: 'user', canUpload: true });
      await refreshAll();
    } catch (e) {
      Toast.error(`创建失败：${mapError(e)}`);
    } finally {
      setCreating(false);
    }
  };

  const saveSiteSettings = async () => {
    setSavingSettings(true);
    try {
      const res = await blogWS.request<{ settings: SiteSettings }>('admin.site.update', {
        heroTitle: settingsDraft.heroTitle,
        heroSubtitle: settingsDraft.heroSubtitle,
        navTitle: settingsDraft.navTitle,
        backgroundMode: settingsDraft.backgroundMode,
        backgroundUrl: settingsDraft.backgroundUrl || '',
        focusX: settingsDraft.focusX,
        focusY: settingsDraft.focusY,
        overlayColor: settingsDraft.overlayColor,
        overlayOpacity: settingsDraft.overlayOpacity,
      });
      const next = { ...defaultSiteSettings(), ...(res.settings || {}) };
      setSettings(next);
      setSettingsDraft(next);
      Toast.success('站点外观已发布');
    } catch (e) {
      Toast.error(mapError(e));
    } finally {
      setSavingSettings(false);
    }
  };

  const uploadHero = async (file: File) => {
    setUploadingHero(true);
    try {
      const isVideo = /^video\//i.test(file.type) || /\.(mp4|webm|mov)$/i.test(file.name);
      // Prefer HTTP for all hero media (supports large video wallpapers).
      const form = new FormData();
      form.append('file', file);
      const resp = await fetch(`${location.origin}/api/site/background/upload`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${blogWS.getToken() || token}`,
        },
        body: form,
      });
      if (!resp.ok) {
        const text = await resp.text();
        let msg = text;
        try {
          const j = JSON.parse(text) as { error?: string };
          if (j.error) msg = j.error;
        } catch {
          // keep text
        }
        // Fallback: small images via WS base64 path.
        if (!isVideo && file.size <= 4 * 1024 * 1024) {
          const buf = await file.arrayBuffer();
          const bytes = new Uint8Array(buf);
          let binary = '';
          for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
          const data = btoa(binary);
          const res = await blogWS.request<{ settings: SiteSettings }>('admin.site.background.upload', {
            data,
            name: file.name,
          }, 60000);
          const next = { ...defaultSiteSettings(), ...(res.settings || {}) };
          setSettings(next);
          setSettingsDraft(next);
          Toast.success(isVideo ? '背景视频已上传' : '背景图已上传');
          return;
        }
        throw new Error(msg || `upload failed (${resp.status})`);
      }
      const body = await resp.json() as { settings?: SiteSettings };
      const next = { ...defaultSiteSettings(), ...(body.settings || {}) };
      setSettings(next);
      setSettingsDraft(next);
      Toast.success(isVideo ? '背景视频已上传（静默循环播放）' : '背景图已上传');
    } finally {
      setUploadingHero(false);
    }
  };

  const saveSelf = async () => {
    if (selfForm.password && selfForm.password !== selfForm.password2) {
      Toast.warning('两次密码不一致');
      return;
    }
    setSelfSaving(true);
    try {
      const res = await blogWS.request<{ ok: boolean; username: string }>('admin.self.update', {
        username: selfForm.username.trim() || undefined,
        password: selfForm.password || undefined,
      });
      setSessionUser((u) => (u ? { ...u, username: res.username } : u));
      setSelfForm((s) => ({ ...s, username: res.username, password: '', password2: '' }));
      Toast.success('账号已更新');
    } catch (e) {
      Toast.error(mapError(e));
    } finally {
      setSelfSaving(false);
    }
  };

  const coverOf = (note: Note) => {
    if (!note.coverUrl) return '';
    return note.coverUrl.startsWith('http') ? note.coverUrl : `${location.origin}${note.coverUrl}`;
  };

  const heroPreview = useMemo(() => resolveHeroBackground(settingsDraft), [settingsDraft]);
  const heroOverlay = useMemo(() => {
    const h = (settingsDraft.overlayColor || '#0b0d12').replace('#', '');
    if (h.length !== 6) return `rgba(11,13,18,${settingsDraft.overlayOpacity})`;
    const r = parseInt(h.slice(0, 2), 16);
    const g = parseInt(h.slice(2, 4), 16);
    const b = parseInt(h.slice(4, 6), 16);
    return `rgba(${r},${g},${b},${settingsDraft.overlayOpacity})`;
  }, [settingsDraft]);

  const overlayColorValue = useMemo(() => {
    try {
      return ColorPicker.colorStringToValue(settingsDraft.overlayColor || '#0b0d12');
    } catch {
      return ColorPicker.colorStringToValue('#0b0d12');
    }
  }, [settingsDraft.overlayColor]);

  if (!authed) {
    return (
      <div className="admin-app admin-login">
        <div className="admin-login-shell">
          <div className="admin-login-brand">
            <div>
              <div className="admin-login-mark admin-login-mark--logo">
                <img src={logoUrl} alt="TimeNotes" draggable={false} />
              </div>
              <h1>TimeNotes Blog</h1>
              <p>TimeNotes 手账本的公开展示与协作上传服务。管理员在此管理手账、用户与站点外观。</p>
              <div className="admin-login-pills">
                <span className="admin-login-pill">公开浏览 · 点赞评论</span>
                <span className="admin-login-pill">客户端上传</span>
                <span className="admin-login-pill">用户与权限</span>
                <span className="admin-login-pill">PoW + JWT 登录</span>
              </div>
            </div>
            <div className="admin-login-foot">仅授权管理员 · 首次请尽快修改默认密码</div>
          </div>
          <div className="admin-login-form">
            <div className="admin-login-form-top">
              <div>
                <h2>欢迎回来</h2>
                <p className="hint">管理员登录（含 PoW 工作量验证）</p>
              </div>
              <ThemeToggle size="small" />
            </div>
            <div className="admin-login-fields">
              <div>
                <label className="field-label">用户名</label>
                <Input prefix={<IconUser />} value={username} onChange={setUsername} placeholder="管理员用户名" size="large" />
              </div>
              <div>
                <label className="field-label">密码</label>
                <Input
                  prefix={<IconLock />}
                  mode="password"
                  value={password}
                  onChange={setPassword}
                  placeholder="登录密码"
                  size="large"
                  onEnterPress={login}
                />
              </div>
              {powProgress ? (
                <div className={`admin-pow-panel ${powProgress.status}`}>
                  <div className="admin-pow-head">
                    <span className="admin-pow-spinner" aria-hidden />
                    <div>
                      <div className="admin-pow-title">
                        {powProgress.status === 'done'
                          ? 'PoW 验证完成'
                          : powProgress.status === 'error'
                            ? 'PoW 验证失败'
                            : '正在计算 PoW 验证码…'}
                      </div>
                      <div className="admin-pow-sub">
                        难度 {powProgress.difficulty || '…'} bit · 尝试 {powProgress.attempts.toLocaleString()} 次
                      </div>
                    </div>
                  </div>
                  <Progress
                    percent={powProgress.percent}
                    showInfo
                    stroke={powProgress.status === 'error' ? 'var(--semi-color-danger)' : undefined}
                    aria-label="PoW progress"
                  />
                </div>
              ) : null}
              <Button className="admin-login-submit" theme="solid" type="primary" loading={busy} onClick={login} block>
                {busy ? '验证中…' : '登录后台'}
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  const activeNav = NAV_ITEMS.find((n) => n.key === navKey) || NAV_ITEMS[0];
  const totalLikes = (stats?.noteStats || []).reduce((s, n) => s + (n.likeCount || 0), 0);
  const totalComments = (stats?.noteStats || []).reduce((s, n) => s + (n.commentCount || 0), 0);
  const userInitial = (sessionUser?.username || 'A').trim().charAt(0).toUpperCase();

  return (
    <div className="admin-app">
      <div className="admin-shell">
        <aside className="admin-sidebar">
          <div className="admin-brand">
            <div className="admin-brand-mark admin-brand-mark--logo">
              <img src={logoUrl} alt="TimeNotes" draggable={false} />
            </div>
            <div className="admin-brand-text">
              <div className="admin-brand-title">TimeNotes</div>
              <div className="admin-brand-sub">Admin Console</div>
            </div>
          </div>

          <nav className="admin-nav">
            {NAV_ITEMS.map((item) => (
              <button
                key={item.key}
                type="button"
                className={`admin-nav-item ${navKey === item.key ? 'is-active' : ''}`}
                onClick={() => setNavKey(item.key)}
              >
                <span className="icon">{item.icon}</span>
                <span>{item.label}</span>
              </button>
            ))}
          </nav>

          <div className="admin-sidebar-foot">
            <div className="admin-user-chip">
              <div className="admin-user-avatar">{userInitial}</div>
              <div className="admin-user-meta">
                <div className="admin-user-name">{sessionUser?.username || '管理员'}</div>
                <div className="admin-user-role">Administrator</div>
              </div>
            </div>
            <Button
              block
              type="danger"
              theme="borderless"
              icon={<IconExit />}
              onClick={() => {
                localStorage.removeItem(TOKEN_KEY);
                setAuthed(false);
                setToken('');
                blogWS.setToken('');
              }}
            >
              退出登录
            </Button>
          </div>
        </aside>

        <div className="admin-main">
          <header className="admin-topbar">
            <div className="admin-topbar-left">
              <h1 className="admin-topbar-title">{activeNav.label}</h1>
              <p className="admin-topbar-desc">{activeNav.desc}</p>
            </div>
            <div className="admin-topbar-actions">
              <ThemeToggle />
            </div>
          </header>

          <main className="admin-content">
            {navKey === 'dash' ? (
              <>
                <div className="admin-stat-grid">
                  <div className="admin-stat-card tone-blue">
                    <div className="row">
                      <div className="label">今日访问</div>
                      <div className="icon-badge"><IconEyeOpened /></div>
                    </div>
                    <div className="value">{stats?.todayCount ?? 0}</div>
                    <div className="hint">含首页与阅读页</div>
                  </div>
                  <div className="admin-stat-card tone-violet">
                    <div className="row">
                      <div className="label">近 14 日访问</div>
                      <div className="icon-badge"><IconHistogram /></div>
                    </div>
                    <div className="value">{stats?.recentCount ?? 0}</div>
                    <div className="hint">滚动窗口统计</div>
                  </div>
                  <div className="admin-stat-card tone-emerald">
                    <div className="row">
                      <div className="label">手账本</div>
                      <div className="icon-badge"><IconBook /></div>
                    </div>
                    <div className="value">{notes.length}</div>
                    <div className="hint">含隐藏手账</div>
                  </div>
                  <div className="admin-stat-card tone-amber">
                    <div className="row">
                      <div className="label">互动合计</div>
                      <div className="icon-badge"><IconLikeThumb /></div>
                    </div>
                    <div className="value">{totalLikes + totalComments}</div>
                    <div className="hint">点赞 {totalLikes} · 评论 {totalComments}</div>
                  </div>
                </div>

                <div className="admin-chart-grid">
                  <section className="admin-panel">
                    <div className="admin-panel-head">
                      <h3>访问趋势</h3>
                      <Tag size="small" color="blue">14 日</Tag>
                    </div>
                    <div className="admin-panel-body">
                      <div className="admin-chart-box">
                        {stats ? <VChart spec={visitSpec as never} style={{ width: '100%', height: '100%' }} /> : null}
                      </div>
                    </div>
                  </section>
                  <section className="admin-panel">
                    <div className="admin-panel-head">
                      <h3>访客地理分布</h3>
                      <Tag size="small" color="violet">World Map</Tag>
                    </div>
                    <div className="admin-panel-body">
                      <div className="admin-chart-box">
                        {stats ? (
                          <WorldMapChart countries={stats.countries || []} locations={stats.locations || []} />
                        ) : null}
                      </div>
                    </div>
                  </section>
                </div>

                <section className="admin-panel">
                  <div className="admin-panel-head">
                    <h3>手账互动</h3>
                    <span className="muted" style={{ fontSize: 12 }}>{(stats?.noteStats || []).length} 本</span>
                  </div>
                  <div className="admin-panel-body tight">
                    <Table
                      dataSource={stats?.noteStats || []}
                      rowKey="noteId"
                      pagination={false}
                      empty={<Empty description="暂无数据" />}
                      columns={[
                        { title: '标题', dataIndex: 'title' },
                        {
                          title: '点赞',
                          dataIndex: 'likeCount',
                          width: 100,
                          render: (v: number) => (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                              <IconLikeThumb size="small" /> {v}
                            </span>
                          ),
                        },
                        {
                          title: '评论',
                          dataIndex: 'commentCount',
                          width: 100,
                          render: (v: number) => (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                              <IconComment size="small" /> {v}
                            </span>
                          ),
                        },
                        {
                          title: '状态',
                          dataIndex: 'visible',
                          width: 100,
                          render: (v: boolean) => (
                            <Tag size="small" color={v ? 'green' : 'grey'}>{v ? '公开' : '隐藏'}</Tag>
                          ),
                        },
                      ]}
                    />
                  </div>
                </section>
              </>
            ) : null}

            {navKey === 'notes' ? (
              <section className="admin-panel">
                <div className="admin-panel-head">
                  <div>
                    <h3>手账列表</h3>
                  </div>
                  <Upload
                    action=""
                    accept=".tnote"
                    showUploadList={false}
                    customRequest={async ({ file, onSuccess, onError }) => {
                      try {
                        const raw = (file as { fileInstance?: File }).fileInstance || (file as unknown as File);
                        await uploadFile(raw);
                        onSuccess?.({});
                      } catch (e) {
                        Toast.error(mapError(e));
                        onError?.({ status: 500 });
                      }
                    }}
                  >
                    <Button theme="solid" type="primary" icon={<IconCloud />}>上传 .tnote</Button>
                  </Upload>
                </div>
                <div className="admin-panel-body tight">
                  <Table
                    dataSource={notes}
                    rowKey="id"
                    empty={<Empty description="暂无手账" />}
                    columns={[
                      {
                        title: '手账',
                        dataIndex: 'title',
                        render: (_: unknown, r: Note) => {
                          const src = coverOf(r);
                          return (
                            <div className="admin-note-cell">
                              {src ? <img className="note-thumb" src={src} alt="" /> : <div className="note-thumb" />}
                              <div className="meta">
                                <div className="title">{r.title || '未命名'}</div>
                                <div className="sub">{r.filename}</div>
                              </div>
                            </div>
                          );
                        },
                      },
                      { title: '上传者', dataIndex: 'ownerName', width: 110, render: (v: string) => `@${v || 'unknown'}` },
                      {
                        title: '互动',
                        width: 120,
                        render: (_: unknown, r: Note) => (
                          <span className="muted" style={{ fontSize: 12 }}>
                            <IconLikeThumb size="small" /> {r.likeCount} · <IconComment size="small" /> {r.commentCount}
                          </span>
                        ),
                      },
                      {
                        title: '可见',
                        dataIndex: 'visible',
                        width: 90,
                        render: (v: boolean, r: Note) => (
                          <Switch
                            checked={v}
                            onChange={async (checked) => {
                              try {
                                await blogWS.request('admin.notes.set_visible', { id: r.id, visible: checked });
                              } catch (e) {
                                Toast.error(mapError(e));
                              }
                            }}
                          />
                        ),
                      },
                      {
                        title: '公开下载',
                        dataIndex: 'publicDownload',
                        width: 100,
                        render: (v: boolean, r: Note) => (
                          <Switch
                            checked={Boolean(v)}
                            onChange={async (checked) => {
                              try {
                                await blogWS.request('admin.notes.set_public_download', { id: r.id, enabled: checked });
                              } catch (e) {
                                Toast.error(mapError(e));
                              }
                            }}
                          />
                        ),
                      },
                      {
                        title: '操作',
                        width: 230,
                        render: (_: unknown, r: Note) => (
                          <div className="admin-actions">
                            <Button size="small" theme="light" icon={<IconDownload />} onClick={() => void downloadNote(r)}>下载</Button>
                            <Button size="small" theme="light" icon={<IconEyeOpened />} onClick={() => void openInClient(r)}>编辑</Button>
                            <Button
                              size="small"
                              type="danger"
                              theme="light"
                              icon={<IconDelete />}
                              onClick={async () => {
                                try {
                                  await blogWS.request('admin.notes.delete', { id: r.id });
                                  Toast.success('已删除');
                                } catch (e) {
                                  Toast.error(mapError(e));
                                }
                              }}
                            >
                              删除
                            </Button>
                          </div>
                        ),
                      },
                    ]}
                  />
                </div>
              </section>
            ) : null}

            {navKey === 'users' ? (
              <section className="admin-panel">
                <div className="admin-panel-head">
                  <h3>用户列表</h3>
                  <Button theme="solid" type="primary" icon={<IconUser />} onClick={() => setCreateOpen(true)}>添加用户</Button>
                </div>
                <div className="admin-panel-body">
                  <div className="admin-alert">
                    上传用户视为可信：手账内原始 HTML 会按原样渲染。删除用户前须将其手账转移给其他管理员；系统始终保留至少一名管理员。
                  </div>
                  <Table
                    dataSource={users}
                    rowKey="id"
                    empty={<Empty description="暂无用户" />}
                    columns={[
                      {
                        title: '用户',
                        dataIndex: 'username',
                        render: (v: string) => (
                          <div className="admin-note-cell">
                            <div className="admin-user-avatar" style={{ width: 32, height: 32, borderRadius: 10, fontSize: 13 }}>
                              {(v || '?').charAt(0).toUpperCase()}
                            </div>
                            <div className="meta">
                              <div className="title">{v}</div>
                            </div>
                          </div>
                        ),
                      },
                      {
                        title: '角色',
                        dataIndex: 'role',
                        width: 110,
                        render: (v: string) => (
                          <Tag size="small" color={v === 'admin' ? 'violet' : 'white'}>
                            {v === 'admin' ? '管理员' : '普通用户'}
                          </Tag>
                        ),
                      },
                      {
                        title: '可上传',
                        dataIndex: 'canUpload',
                        width: 100,
                        render: (v: boolean, r: User) => (
                          <Switch
                            checked={v}
                            onChange={async (checked) => {
                              await blogWS.request('admin.users.update', { id: r.id, canUpload: checked });
                              await refreshAll();
                            }}
                          />
                        ),
                      },
                      {
                        title: '设为管理员',
                        width: 120,
                        render: (_: unknown, r: User) => (
                          <Switch
                            checked={r.role === 'admin'}
                            onChange={async (checked) => {
                              try {
                                await blogWS.request('admin.users.update', {
                                  id: r.id,
                                  role: checked ? 'admin' : 'user',
                                  canUpload: checked ? true : r.canUpload,
                                });
                                await refreshAll();
                              } catch (e) {
                                Toast.error(mapError(e));
                              }
                            }}
                          />
                        ),
                      },
                      {
                        title: '操作',
                        width: 180,
                        render: (_: unknown, r: User) => (
                          <div className="admin-actions">
                            <Button
                              size="small"
                              theme="light"
                              onClick={() => {
                                setEditUser(r);
                                setEditForm({ username: r.username, password: '', role: r.role, canUpload: r.canUpload });
                              }}
                            >
                              编辑
                            </Button>
                            <Button
                              size="small"
                              type="danger"
                              theme="light"
                              onClick={() => {
                                setDeleteUser(r);
                                const admins = users.filter((u) => u.role === 'admin' && u.id !== r.id);
                                setTransferTo(admins[0]?.id || '');
                              }}
                            >
                              删除
                            </Button>
                          </div>
                        ),
                      },
                    ]}
                  />
                </div>
              </section>
            ) : null}

            {navKey === 'site' ? (
              <div className="admin-site-grid">
                <section className="admin-panel">
                  <div className="admin-panel-head">
                    <h3>站点外观设置</h3>
                    <Tag size="small" color="cyan">实时广播</Tag>
                  </div>
                  <div className="admin-panel-body">
                    <div className="admin-form-stack">
                      <div>
                        <label className="field-label">导航栏标题</label>
                        <Input
                          value={settingsDraft.navTitle}
                          onChange={(v) => setSettingsDraft((s) => ({ ...s, navTitle: v }))}
                          placeholder="TimeNotes Blog"
                        />
                      </div>
                      <div>
                        <label className="field-label">Hero 标题</label>
                        <Input value={settingsDraft.heroTitle} onChange={(v) => setSettingsDraft((s) => ({ ...s, heroTitle: v }))} />
                      </div>
                      <div>
                        <label className="field-label">副标题</label>
                        <Input value={settingsDraft.heroSubtitle} onChange={(v) => setSettingsDraft((s) => ({ ...s, heroSubtitle: v }))} />
                      </div>
                      <div>
                        <label className="field-label">背景模式</label>
                        <Select
                          style={{ width: '100%' }}
                          value={settingsDraft.backgroundMode}
                          optionList={[
                            { value: 'none', label: '默认渐变' },
                            { value: 'url', label: '图片 / 视频 URL' },
                            { value: 'upload', label: '上传图片或视频' },
                          ]}
                          onChange={(v) => setSettingsDraft((s) => ({ ...s, backgroundMode: String(v) as SiteSettings['backgroundMode'] }))}
                        />
                      </div>
                      {settingsDraft.backgroundMode === 'url' ? (
                        <div>
                          <label className="field-label">背景 URL（https，支持图片或 mp4/webm）</label>
                          <Input value={settingsDraft.backgroundUrl || ''} onChange={(v) => setSettingsDraft((s) => ({ ...s, backgroundUrl: v }))} placeholder="https://.../hero.mp4" />
                        </div>
                      ) : null}
                      {settingsDraft.backgroundMode === 'upload' ? (
                        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                          <Upload
                            action=""
                            accept="image/png,image/jpeg,image/webp,image/gif,video/mp4,video/webm"
                            showUploadList={false}
                            customRequest={async ({ file, onSuccess, onError }) => {
                              try {
                                const raw = (file as { fileInstance?: File }).fileInstance || (file as unknown as File);
                                await uploadHero(raw);
                                onSuccess?.({});
                              } catch (e) {
                                Toast.error(mapError(e));
                                onError?.({ status: 500 });
                              }
                            }}
                          >
                            <Button theme="solid" type="primary" loading={uploadingHero}>上传背景媒体</Button>
                          </Upload>
                          <Button
                            type="danger"
                            theme="light"
                            onClick={async () => {
                              try {
                                const res = await blogWS.request<{ settings: SiteSettings }>('admin.site.update', { clearUpload: true, backgroundMode: 'none' });
                                const next = { ...defaultSiteSettings(), ...(res.settings || {}) };
                                setSettings(next);
                                setSettingsDraft(next);
                              } catch (e) {
                                Toast.error(mapError(e));
                              }
                            }}
                          >
                            清除上传
                          </Button>
                          <Typography.Text type="tertiary" style={{ fontSize: 12 }}>
                            视频将作为动态壁纸静默循环播放（≤80MB，mp4/webm）
                          </Typography.Text>
                        </div>
                      ) : null}
                      <div>
                        <label className="field-label">焦点 X ({Math.round(settingsDraft.focusX)}%)</label>
                        <Slider value={settingsDraft.focusX} min={0} max={100} onChange={(v) => setSettingsDraft((s) => ({ ...s, focusX: Number(v) }))} />
                      </div>
                      <div>
                        <label className="field-label">焦点 Y ({Math.round(settingsDraft.focusY)}%)</label>
                        <Slider value={settingsDraft.focusY} min={0} max={100} onChange={(v) => setSettingsDraft((s) => ({ ...s, focusY: Number(v) }))} />
                      </div>
                      <div>
                        <label className="field-label">遮罩颜色</label>
                        <div className="admin-color-row">
                          <ColorPicker
                            usePopover
                            alpha={false}
                            value={overlayColorValue}
                            onChange={(v) => {
                              const hex = (v.hex || '#0b0d12').slice(0, 7);
                              setSettingsDraft((s) => ({ ...s, overlayColor: hex }));
                            }}
                          />
                          <Input
                            style={{ maxWidth: 140 }}
                            value={settingsDraft.overlayColor}
                            onChange={(v) => setSettingsDraft((s) => ({ ...s, overlayColor: v }))}
                            placeholder="#0b0d12"
                          />
                        </div>
                      </div>
                      <div>
                        <label className="field-label">遮罩透明度 ({settingsDraft.overlayOpacity.toFixed(2)})</label>
                        <Slider
                          value={Math.round(settingsDraft.overlayOpacity * 100)}
                          min={0}
                          max={90}
                          onChange={(v) => setSettingsDraft((s) => ({ ...s, overlayOpacity: Number(v) / 100 }))}
                        />
                      </div>
                      <Button theme="solid" type="primary" loading={savingSettings} onClick={() => void saveSiteSettings()}>
                        发布到公开首页
                      </Button>
                    </div>
                  </div>
                </section>
                <section className="admin-panel">
                  <div className="admin-panel-head">
                    <h3>实时预览</h3>
                    <Tag size="small" color="grey">
                      {settings.backgroundMode}
                      {isHeroVideo(settingsDraft) ? ' · video' : ''}
                    </Tag>
                  </div>
                  <div className="admin-panel-body">
                    <div
                      className="site-preview"
                      style={{
                        backgroundImage: heroPreview && !isHeroVideo(settingsDraft)
                          ? `url(${heroPreview})`
                          : undefined,
                        backgroundPosition: `${settingsDraft.focusX}% ${settingsDraft.focusY}%`,
                      }}
                    >
                      {heroPreview && isHeroVideo(settingsDraft) ? (
                        <video
                          className="site-preview-video"
                          src={heroPreview}
                          autoPlay
                          muted
                          loop
                          playsInline
                          controls={false}
                          style={{ objectPosition: `${settingsDraft.focusX}% ${settingsDraft.focusY}%` }}
                        />
                      ) : null}
                      <div className="site-preview-overlay" style={{ background: heroOverlay }} />
                      <div className="site-preview-copy">
                        <div className="site-preview-title">{settingsDraft.heroTitle}</div>
                        <div className="site-preview-sub">{settingsDraft.heroSubtitle}</div>
                      </div>
                    </div>
                    <Typography.Text type="tertiary" style={{ display: 'block', marginTop: 12, fontSize: 12 }}>
                      线上：{settings.heroTitle} · 模式 {settings.backgroundMode}
                      {settings.backgroundMediaType ? ` · ${settings.backgroundMediaType}` : ''}
                    </Typography.Text>
                  </div>
                </section>
              </div>
            ) : null}

            {navKey === 'account' ? (
              <section className="admin-panel admin-account-panel">
                <div className="admin-panel-head">
                  <h3>当前账号安全</h3>
                </div>
                <div className="admin-panel-body">
                  <div className="admin-form-stack">
                    <div>
                      <label className="field-label">用户名</label>
                      <Input value={selfForm.username} onChange={(v) => setSelfForm((s) => ({ ...s, username: v }))} />
                    </div>
                    <div>
                      <label className="field-label">新密码（留空不改）</label>
                      <Input mode="password" value={selfForm.password} onChange={(v) => setSelfForm((s) => ({ ...s, password: v }))} />
                    </div>
                    <div>
                      <label className="field-label">确认新密码</label>
                      <Input mode="password" value={selfForm.password2} onChange={(v) => setSelfForm((s) => ({ ...s, password2: v }))} />
                    </div>
                    <Button theme="solid" type="primary" loading={selfSaving} onClick={() => void saveSelf()}>保存更改</Button>
                  </div>
                </div>
              </section>
            ) : null}
          </main>
        </div>
      </div>

      <AdminCredentialMigrationModal
        visible={authed && mustChange}
        loading={migrating}
        onSubmit={migrateCredentials}
      />

      <Modal
        title="编辑用户"
        visible={Boolean(editUser)}
        okText="保存"
        cancelText="取消"
        onCancel={() => setEditUser(null)}
        onOk={async () => {
          if (!editUser) return;
          try {
            await blogWS.request('admin.users.update', {
              id: editUser.id,
              username: editForm.username.trim(),
              password: editForm.password || undefined,
              role: editForm.role,
              canUpload: editForm.role === 'admin' ? true : editForm.canUpload,
            });
            Toast.success('用户已更新');
            setEditUser(null);
            await refreshAll();
          } catch (e) {
            Toast.error(mapError(e));
          }
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <Input value={editForm.username} onChange={(v) => setEditForm((s) => ({ ...s, username: v }))} placeholder="用户名" />
          <Input mode="password" value={editForm.password} onChange={(v) => setEditForm((s) => ({ ...s, password: v }))} placeholder="新密码（可选）" />
          <Select
            style={{ width: '100%' }}
            value={editForm.role}
            optionList={[{ value: 'user', label: '普通用户' }, { value: 'admin', label: '管理员' }]}
            onChange={(v) => setEditForm((s) => ({ ...s, role: String(v) === 'admin' ? 'admin' : 'user', canUpload: String(v) === 'admin' ? true : s.canUpload }))}
          />
          <Checkbox
            checked={editForm.role === 'admin' ? true : editForm.canUpload}
            disabled={editForm.role === 'admin'}
            onChange={(e) => setEditForm((s) => ({ ...s, canUpload: Boolean(e.target.checked) }))}
          >
            允许上传手账
          </Checkbox>
        </div>
      </Modal>

      <Modal
        title="删除用户"
        visible={Boolean(deleteUser)}
        okText="确认删除"
        cancelText="取消"
        onCancel={() => setDeleteUser(null)}
        onOk={async () => {
          if (!deleteUser) return;
          try {
            await blogWS.request('admin.users.delete', {
              id: deleteUser.id,
              transferToAdminId: transferTo || undefined,
            });
            Toast.success('用户已删除');
            setDeleteUser(null);
            await refreshAll();
          } catch (e) {
            Toast.error(mapError(e));
          }
        }}
      >
        <Typography.Paragraph>
          删除用户 <b>{deleteUser?.username}</b>。若其拥有手账，必须转移给其他管理员。
        </Typography.Paragraph>
        <Select
          style={{ width: '100%' }}
          placeholder="选择接收手账的管理员"
          value={transferTo}
          optionList={users.filter((u) => u.role === 'admin' && u.id !== deleteUser?.id).map((u) => ({ value: u.id, label: u.username }))}
          onChange={(v) => setTransferTo(String(v))}
        />
      </Modal>

      <Modal
        title="添加用户"
        visible={createOpen}
        okText="创建"
        cancelText="取消"
        confirmLoading={creating}
        onCancel={() => {
          setCreateOpen(false);
          setNewUser({ username: '', password: '', role: 'user', canUpload: true });
        }}
        onOk={handleCreateUser}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div>
            <label className="field-label">用户名（不可重名）</label>
            <Input placeholder="例如 editor01" value={newUser.username} onChange={(v) => setNewUser((s) => ({ ...s, username: v }))} />
          </div>
          <div>
            <label className="field-label">密码</label>
            <Input mode="password" placeholder="登录密码" value={newUser.password} onChange={(v) => setNewUser((s) => ({ ...s, password: v }))} />
          </div>
          <div>
            <label className="field-label">角色</label>
            <Select
              style={{ width: '100%' }}
              value={newUser.role}
              optionList={[
                { value: 'user', label: '普通用户 (user)' },
                { value: 'admin', label: '管理员 (admin)' },
              ]}
              onChange={(v) => {
                const role = String(v) === 'admin' ? 'admin' : 'user';
                setNewUser((s) => ({ ...s, role, canUpload: role === 'admin' ? true : s.canUpload }));
              }}
            />
          </div>
          <Checkbox
            checked={newUser.role === 'admin' ? true : newUser.canUpload}
            disabled={newUser.role === 'admin'}
            onChange={(e) => setNewUser((s) => ({ ...s, canUpload: Boolean(e.target.checked) }))}
          >
            允许上传手账{newUser.role === 'admin' ? '（管理员默认开启）' : ''}
          </Checkbox>
        </div>
      </Modal>
    </div>
  );
}
