import { useEffect, useMemo, useState } from 'react';
import { Button, Checkbox, Input, Modal, Select, Switch, Table, TabPane, Tabs, Toast, Typography, Upload } from '@douyinfe/semi-ui';
import { IconCloud, IconDelete, IconDownload, IconEyeOpened, IconUser, IconHistogram } from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { blogWS } from '../../lib/wsClient';
import { applyDarkTheme } from '../../theme';
import { AdminCredentialMigrationModal } from '../../components/AdminCredentialMigrationModal';

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
  noteStats: Array<{ noteId: string; title: string; likeCount: number; commentCount: number; visible: boolean }>;
};

const TOKEN_KEY = 'tn_blog_admin_token';

const darkChartCommon = {
  background: 'transparent',
  color: ['#7dd3fc', '#a78bfa', '#34d399', '#fbbf24'],
  title: {
    textStyle: { fill: '#f4f1ea', fontSize: 14, fontWeight: 600 },
  },
  axes: [
    {
      orient: 'left',
      label: { style: { fill: 'rgba(244,241,234,0.7)' } },
      grid: { style: { stroke: 'rgba(255,255,255,0.08)' } },
      domainLine: { style: { stroke: 'rgba(255,255,255,0.15)' } },
    },
    {
      orient: 'bottom',
      label: { style: { fill: 'rgba(244,241,234,0.7)' } },
      grid: { visible: false },
      domainLine: { style: { stroke: 'rgba(255,255,255,0.15)' } },
    },
  ],
};

export function AdminApp() {
  const [token, setToken] = useState(localStorage.getItem(TOKEN_KEY) || '');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [authed, setAuthed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [users, setUsers] = useState<User[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
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

  useEffect(() => {
    applyDarkTheme();
  }, []);

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
    setMustChange(Boolean(res.mustChangeCredentials));
    setAuthed(true);
  };

  const login = async () => {
    setBusy(true);
    try {
      await blogWS.connect();
      const res = await blogWS.login(username, password);
      if (res.role !== 'admin') {
        throw new Error('需要管理员账号');
      }
      applySession(res);
      Toast.success('登录成功');
      if (!res.mustChangeCredentials) {
        await refreshAll();
      }
    } catch (e) {
      Toast.error(mapError(e));
    } finally {
      setBusy(false);
    }
  };

  const ensureAuth = async () => {
    if (!token) return;
    try {
      await blogWS.connect();
      const res = await blogWS.loginWithToken(token);
      if (res.role !== 'admin') throw new Error('需要管理员账号');
      applySession({ ...res, token });
      if (!res.mustChangeCredentials) {
        await refreshAll();
      }
    } catch {
      localStorage.removeItem(TOKEN_KEY);
      setToken('');
      setAuthed(false);
      setMustChange(false);
      setSessionUser(null);
    }
  };

  const refreshAll = async () => {
    const [u, n, s] = await Promise.all([
      blogWS.request<{ users: User[] }>('admin.users.list', {}),
      blogWS.request<{ notes: Note[] }>('admin.notes.list', {}),
      blogWS.request<Stats>('admin.stats', {}),
    ]);
    setUsers(u.users || []);
    setNotes(n.notes || []);
    setStats(s);
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
      title: { text: '近 14 日访问量', ...darkChartCommon.title },
      background: 'transparent',
      color: darkChartCommon.color,
      area: { style: { fillOpacity: 0.25 } },
      line: { style: { stroke: '#7dd3fc', lineWidth: 2 } },
      point: { style: { fill: '#7dd3fc', stroke: '#0b0d12', lineWidth: 1 } },
      axes: darkChartCommon.axes,
    };
  }, [stats]);

  const mapSpec = useMemo(() => {
    const values = (stats?.locations || []).map((l) => ({
      lat: l.lat,
      lng: l.lng,
      size: l.count,
      name: [l.city, l.region, l.country].filter(Boolean).join(', '),
    }));
    return {
      type: 'common',
      background: 'transparent',
      color: darkChartCommon.color,
      data: [{ id: 'loc', values: values.length ? values : [{ lat: 0, lng: 0, size: 1, name: '暂无' }] }],
      series: [
        {
          type: 'scatter',
          xField: 'lng',
          yField: 'lat',
          sizeField: 'size',
          size: { type: 'linear', range: [6, 24] },
          point: { style: { fill: '#60a5fa', fillOpacity: 0.8, stroke: '#e0f2fe' } },
        },
      ],
      title: { text: '访客地理分布（散点近似）', ...darkChartCommon.title },
      axes: [
        {
          orient: 'left',
          title: { text: 'Lat', style: { fill: 'rgba(244,241,234,0.7)' } },
          min: -90,
          max: 90,
          label: { style: { fill: 'rgba(244,241,234,0.7)' } },
          grid: { style: { stroke: 'rgba(255,255,255,0.08)' } },
        },
        {
          orient: 'bottom',
          title: { text: 'Lng', style: { fill: 'rgba(244,241,234,0.7)' } },
          min: -180,
          max: 180,
          label: { style: { fill: 'rgba(244,241,234,0.7)' } },
          grid: { style: { stroke: 'rgba(255,255,255,0.08)' } },
        },
      ],
    };
  }, [stats]);

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
    if (!name) {
      Toast.warning('请填写用户名');
      return;
    }
    if (!pass) {
      Toast.warning('请填写密码');
      return;
    }
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

  if (!authed) {
    return (
      <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 20 }}>
        <div className="glass-strong" style={{ width: 'min(400px, 100%)', borderRadius: 18, padding: 24 }}>
          <Typography.Title heading={3} style={{ color: '#f4f1ea', marginTop: 0 }}>后台登录</Typography.Title>
          <p className="muted">使用管理员账号登录（含 PoW 验证）</p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 16 }}>
            <div>
              <label className="field-label">用户名</label>
              <Input prefix={<IconUser />} value={username} onChange={setUsername} placeholder="admin" />
            </div>
            <div>
              <label className="field-label">密码</label>
              <Input mode="password" value={password} onChange={setPassword} placeholder="密码" onEnterPress={login} />
            </div>
            <Button theme="solid" type="primary" loading={busy} onClick={login}>登录</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="admin-layout">
      <aside className="admin-side glass-strong">
        <Typography.Title heading={4} style={{ color: '#f4f1ea', marginTop: 0 }}>TimeNotes Admin</Typography.Title>
        <p className="muted" style={{ fontSize: 12 }}>安全后台 · {sessionUser?.username || '管理员'}</p>
        <Button block theme="borderless" type="danger" style={{ marginTop: 16 }} onClick={() => {
          localStorage.removeItem(TOKEN_KEY);
          setAuthed(false);
          setToken('');
        }}>
          退出登录
        </Button>
      </aside>
      <main className="admin-main">
        <Tabs type="line" keepDOM={false}>
          <TabPane tab={<span><IconHistogram /> 仪表盘</span>} itemKey="dash">
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(180px,1fr))', gap: 12, marginBottom: 16 }}>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">今日访问</div>
                <div style={{ fontSize: 28, fontWeight: 700, color: '#f4f1ea' }}>{stats?.todayCount ?? 0}</div>
              </div>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">近 14 日访问</div>
                <div style={{ fontSize: 28, fontWeight: 700, color: '#f4f1ea' }}>{stats?.recentCount ?? 0}</div>
              </div>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">手账数</div>
                <div style={{ fontSize: 28, fontWeight: 700, color: '#f4f1ea' }}>{notes.length}</div>
              </div>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 12 }}>
              <div className="chart-panel">
                {stats ? <VChart spec={visitSpec as never} /> : null}
              </div>
              <div className="chart-panel">
                {stats ? <VChart spec={mapSpec as never} /> : null}
              </div>
            </div>
            <div className="glass" style={{ borderRadius: 14, padding: 12, marginTop: 12 }}>
              <Typography.Title heading={5} style={{ color: '#f4f1ea' }}>手账互动</Typography.Title>
              <Table
                dataSource={stats?.noteStats || []}
                rowKey="noteId"
                pagination={false}
                empty={<span className="muted">暂无数据</span>}
                columns={[
                  { title: '标题', dataIndex: 'title' },
                  { title: '点赞', dataIndex: 'likeCount', width: 90 },
                  { title: '评论', dataIndex: 'commentCount', width: 90 },
                  { title: '可见', dataIndex: 'visible', width: 90, render: (v) => (v ? '是' : '否') },
                ]}
              />
            </div>
          </TabPane>

          <TabPane tab={<span><IconCloud /> 手账管理</span>} itemKey="notes">
            <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
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
                <Button theme="solid" type="primary">上传 .tnote</Button>
              </Upload>
              <Button theme="light" onClick={() => void refreshAll()}>刷新</Button>
            </div>
            <div className="glass" style={{ borderRadius: 14, padding: 8 }}>
              <Table
                dataSource={notes}
                rowKey="id"
                empty={<span className="muted">暂无手账</span>}
                columns={[
                  { title: '标题', dataIndex: 'title' },
                  { title: '文件名', dataIndex: 'filename' },
                  { title: '上传者', dataIndex: 'ownerName' },
                  { title: '点赞', dataIndex: 'likeCount', width: 80 },
                  { title: '评论', dataIndex: 'commentCount', width: 80 },
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
                    width: 280,
                    render: (_: unknown, r: Note) => (
                      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
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
          </TabPane>

          <TabPane tab={<span><IconUser /> 用户管理</span>} itemKey="users">
            <div style={{ marginBottom: 12 }}>
              <Button theme="solid" type="primary" onClick={() => setCreateOpen(true)}>添加用户</Button>
            </div>
            <div className="glass" style={{ borderRadius: 14, padding: 8 }}>
              <Table
                dataSource={users}
                rowKey="id"
                empty={<span className="muted">暂无用户</span>}
                columns={[
                  { title: '用户名', dataIndex: 'username' },
                  { title: '角色', dataIndex: 'role', width: 100 },
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
                          await blogWS.request('admin.users.update', {
                            id: r.id,
                            role: checked ? 'admin' : 'user',
                            canUpload: checked ? true : r.canUpload,
                          });
                          await refreshAll();
                        }}
                      />
                    ),
                  },
                  {
                    title: '操作',
                    width: 180,
                    render: (_: unknown, r: User) => (
                      <div style={{ display: 'flex', gap: 6 }}>
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
          </TabPane>
        </Tabs>
      </main>


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
            <Input
              placeholder="例如 editor01"
              value={newUser.username}
              onChange={(v) => setNewUser((s) => ({ ...s, username: v }))}
            />
          </div>
          <div>
            <label className="field-label">密码</label>
            <Input
              mode="password"
              placeholder="登录密码"
              value={newUser.password}
              onChange={(v) => setNewUser((s) => ({ ...s, password: v }))}
            />
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
                setNewUser((s) => ({
                  ...s,
                  role,
                  canUpload: role === 'admin' ? true : s.canUpload,
                }));
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
